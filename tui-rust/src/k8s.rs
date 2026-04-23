use anyhow::{anyhow, Result};
use k8s_openapi::api::core::v1::Pod;
use kube::api::{Api, ListParams};
use std::net::TcpListener;
use tokio::net::TcpListener as TokioTcpListener;

pub struct Client {
    client: kube::Client,
    namespace: String,
}

impl Client {
    pub async fn new(namespace: &str) -> Result<Self> {
        let client = kube::Client::try_default().await?;
        Ok(Client {
            client,
            namespace: namespace.to_string(),
        })
    }

    /// Find the BunkerWeb Redis pod by label selector.
    pub async fn find_redis_pod(&self) -> Result<String> {
        let pods: Api<Pod> = Api::namespaced(self.client.clone(), &self.namespace);
        let lp = ListParams::default().labels("bunkerweb.io/component=redis");
        let pod_list = pods.list(&lp).await?;

        for pod in pod_list.items {
            if let Some(status) = &pod.status {
                if status.phase.as_deref() == Some("Running") {
                    if let Some(name) = pod.metadata.name {
                        return Ok(name);
                    }
                }
            }
        }

        Err(anyhow!(
            "no running redis-bunkerweb pods found in namespace {}",
            self.namespace
        ))
    }

    /// Start a port-forward to the given pod and return the local port.
    pub async fn start_port_forward(&self, pod_name: &str, remote_port: u16) -> Result<u16> {
        let pods: Api<Pod> = Api::namespaced(self.client.clone(), &self.namespace);

        let local_port = free_port()?;

        let pod_name = pod_name.to_string();
        let pods_clone = pods.clone();

        // Spawn a TCP proxy that forwards connections through the K8s portforward API
        tokio::spawn(async move {
            let listener = match TokioTcpListener::bind(format!("127.0.0.1:{}", local_port)).await
            {
                Ok(l) => l,
                Err(e) => {
                    eprintln!("port-forward bind error: {}", e);
                    return;
                }
            };

            loop {
                let (tcp_stream, _) = match listener.accept().await {
                    Ok(s) => s,
                    Err(_) => continue,
                };

                let pods_inner = pods_clone.clone();
                let pod_inner = pod_name.clone();

                tokio::spawn(async move {
                    let mut pf = match pods_inner.portforward(&pod_inner, &[remote_port]).await {
                        Ok(pf) => pf,
                        Err(e) => {
                            eprintln!("port-forward error: {}", e);
                            return;
                        }
                    };

                    let upstream = match pf.take_stream(remote_port) {
                        Some(s) => s,
                        None => {
                            eprintln!("port-forward: no stream for port {}", remote_port);
                            return;
                        }
                    };

                    // Split both sides and copy bidirectionally
                    let (mut tcp_read, mut tcp_write) = tokio::io::split(tcp_stream);
                    let (mut up_read, mut up_write) = tokio::io::split(upstream);

                    let client_to_server = tokio::io::copy(&mut tcp_read, &mut up_write);
                    let server_to_client = tokio::io::copy(&mut up_read, &mut tcp_write);

                    let _ = tokio::try_join!(client_to_server, server_to_client);

                    // Ensure the portforward is joined to clean up
                    let _ = pf.join().await;
                });
            }
        });

        // Give the listener time to bind
        tokio::time::sleep(std::time::Duration::from_millis(100)).await;

        Ok(local_port)
    }
}

fn free_port() -> Result<u16> {
    let listener = TcpListener::bind("127.0.0.1:0")?;
    Ok(listener.local_addr()?.port())
}
