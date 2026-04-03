use anyhow::{anyhow, Result};
use k8s_openapi::api::core::v1::Pod;
use kube::api::{Api, ListParams};
use std::net::TcpListener;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
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
    /// Uses the kube crate's portforward API.
    pub async fn start_port_forward(&self, pod_name: &str, remote_port: u16) -> Result<u16> {
        let pods: Api<Pod> = Api::namespaced(self.client.clone(), &self.namespace);

        // Find a free local port
        let local_port = free_port()?;

        let pod_name = pod_name.to_string();
        let pods_clone = pods.clone();

        // Spawn the port-forward proxy in the background
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
                let (mut tcp_stream, _) = match listener.accept().await {
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

                    let mut upstream = match pf.take_stream(remote_port) {
                        Some(s) => s,
                        None => {
                            eprintln!("port-forward: no stream for port {}", remote_port);
                            return;
                        }
                    };

                    let (mut tcp_read, mut tcp_write) = tcp_stream.split();
                    let (mut up_read, mut up_write) = tokio::io::split(&mut upstream);

                    let _ = tokio::io::copy_bidirectional(
                        &mut TwoHalf {
                            r: &mut tcp_read,
                            w: &mut up_write,
                        },
                        &mut TwoHalf {
                            r: &mut up_read,
                            w: &mut tcp_write,
                        },
                    )
                    .await;
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

/// Helper to combine a reader and writer into a single AsyncRead + AsyncWrite.
struct TwoHalf<R, W> {
    r: R,
    w: W,
}

impl<R: AsyncReadExt + Unpin, W: Unpin> tokio::io::AsyncRead for TwoHalf<R, W> {
    fn poll_read(
        self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
        buf: &mut tokio::io::ReadBuf<'_>,
    ) -> std::task::Poll<std::io::Result<()>> {
        let this = self.get_mut();
        std::pin::Pin::new(&mut this.r).poll_read(cx, buf)
    }
}

impl<R: Unpin, W: AsyncWriteExt + Unpin> tokio::io::AsyncWrite for TwoHalf<R, W> {
    fn poll_write(
        self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
        buf: &[u8],
    ) -> std::task::Poll<Result<usize, std::io::Error>> {
        let this = self.get_mut();
        std::pin::Pin::new(&mut this.w).poll_write(cx, buf)
    }

    fn poll_flush(
        self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<Result<(), std::io::Error>> {
        let this = self.get_mut();
        std::pin::Pin::new(&mut this.w).poll_flush(cx)
    }

    fn poll_shutdown(
        self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<Result<(), std::io::Error>> {
        let this = self.get_mut();
        std::pin::Pin::new(&mut this.w).poll_shutdown(cx)
    }
}
