import Foundation

/// Kubernetes API client that supports REST calls and WebSocket exec.
final class KubernetesClient: NSObject, URLSessionDelegate, URLSessionWebSocketDelegate {
    private let serverURL: String
    private let config: KubeConfig
    private lazy var session: URLSession = {
        URLSession(configuration: .default, delegate: self, delegateQueue: nil)
    }()

    init(config: KubeConfig) {
        self.serverURL = config.server.hasSuffix("/") ? String(config.server.dropLast()) : config.server
        self.config = config
        super.init()
    }

    // MARK: - REST API

    /// Lists pods matching a label selector in a namespace.
    func listPods(namespace: String, labelSelector: String) async throws -> [PodInfo] {
        var components = URLComponents(string: "\(serverURL)/api/v1/namespaces/\(namespace)/pods")!
        components.queryItems = [URLQueryItem(name: "labelSelector", value: labelSelector)]

        var request = URLRequest(url: components.url!)
        applyAuth(to: &request)

        let (data, response) = try await session.data(for: request)
        try checkHTTPResponse(response, data: data)

        let podList = try JSONDecoder().decode(PodList.self, from: data)
        return podList.items.map { item in
            PodInfo(
                name: item.metadata.name,
                phase: item.status.phase
            )
        }
    }

    /// Finds the first running pod matching the label selector.
    func findRunningPod(namespace: String, labelSelector: String) async throws -> String {
        let pods = try await listPods(namespace: namespace, labelSelector: labelSelector)
        guard let running = pods.first(where: { $0.phase == "Running" }) else {
            if pods.isEmpty {
                throw K8sError.noPods(labelSelector)
            }
            throw K8sError.noRunningPods
        }
        return running.name
    }

    // MARK: - WebSocket Exec

    /// Executes a command in a pod and returns stdout as a string.
    func exec(namespace: String, pod: String, command: [String], timeoutSeconds: TimeInterval = 30) async throws -> String {
        var components = URLComponents(string: "\(serverURL)/api/v1/namespaces/\(namespace)/pods/\(pod)/exec")!
        var queryItems = command.map { URLQueryItem(name: "command", value: $0) }
        queryItems.append(URLQueryItem(name: "stdout", value: "true"))
        queryItems.append(URLQueryItem(name: "stderr", value: "true"))
        components.queryItems = queryItems

        // Convert scheme for WebSocket
        if components.scheme == "https" {
            components.scheme = "wss"
        } else if components.scheme == "http" {
            components.scheme = "ws"
        }

        var request = URLRequest(url: components.url!)
        applyAuth(to: &request)

        let task = session.webSocketTask(with: request, protocols: ["v4.channel.k8s.io"])
        task.resume()

        return try await withThrowingTaskGroup(of: String.self) { group in
            group.addTask {
                var stdoutData = Data()

                while true {
                    let message = try await task.receive()
                    switch message {
                    case .data(let data):
                        guard data.count > 0 else { continue }
                        let channel = data[0]
                        let payload = data.subdata(in: 1..<data.count)
                        switch channel {
                        case 1: // stdout
                            stdoutData.append(payload)
                        case 3: // status - command complete
                            task.cancel(with: .normalClosure, reason: nil)
                            return String(data: stdoutData, encoding: .utf8) ?? ""
                        default:
                            break
                        }
                    case .string(let str):
                        // v4 protocol uses binary, but handle string gracefully
                        if str.contains("\"status\":\"Success\"") || str.contains("\"status\":\"Failure\"") {
                            task.cancel(with: .normalClosure, reason: nil)
                            return String(data: stdoutData, encoding: .utf8) ?? ""
                        }
                    @unknown default:
                        break
                    }
                }
            }

            group.addTask {
                try await Task.sleep(nanoseconds: UInt64(timeoutSeconds * 1_000_000_000))
                task.cancel(with: .normalClosure, reason: nil)
                throw K8sError.execTimeout
            }

            guard let result = try await group.next() else {
                throw K8sError.execTimeout
            }
            group.cancelAll()
            return result
        }
    }

    // MARK: - Auth

    private func applyAuth(to request: inout URLRequest) {
        if let token = config.token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        } else if let username = config.username, let password = config.password {
            let credentials = "\(username):\(password)"
            if let data = credentials.data(using: .utf8) {
                request.setValue("Basic \(data.base64EncodedString())", forHTTPHeaderField: "Authorization")
            }
        }
    }

    private func checkHTTPResponse(_ response: URLResponse, data: Data) throws {
        guard let http = response as? HTTPURLResponse else {
            throw K8sError.invalidResponse
        }
        guard (200...299).contains(http.statusCode) else {
            let body = String(data: data, encoding: .utf8) ?? ""
            throw K8sError.httpError(http.statusCode, body)
        }
    }

    // MARK: - URLSessionDelegate (TLS)

    func urlSession(_ session: URLSession, didReceive challenge: URLAuthenticationChallenge) async -> (URLSession.AuthChallengeDisposition, URLCredential?) {
        guard challenge.protectionSpace.authenticationMethod == NSURLAuthenticationMethodServerTrust,
              let serverTrust = challenge.protectionSpace.serverTrust else {
            return (.performDefaultHandling, nil)
        }

        if config.insecureSkipTLSVerify {
            return (.useCredential, URLCredential(trust: serverTrust))
        }

        if let caData = config.caCertData,
           let caCert = SecCertificateCreateWithData(nil, caData as CFData) {
            SecTrustSetAnchorCertificates(serverTrust, [caCert] as CFArray)
            SecTrustSetAnchorCertificatesOnly(serverTrust, true)

            var error: CFError?
            if SecTrustEvaluateWithError(serverTrust, &error) {
                return (.useCredential, URLCredential(trust: serverTrust))
            }
        }

        return (.performDefaultHandling, nil)
    }

    // WebSocket delegate - accept the negotiated protocol
    func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didOpenWithProtocol protocol: String?) {}

    func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didCloseWith closeCode: URLSessionWebSocketTask.CloseCode, reason: Data?) {}
}

// MARK: - K8s API Types

private struct PodList: Codable {
    var items: [PodItem]
}

private struct PodItem: Codable {
    var metadata: PodMetadata
    var status: PodStatus
}

private struct PodMetadata: Codable {
    var name: String
}

private struct PodStatus: Codable {
    var phase: String
}

struct PodInfo {
    var name: String
    var phase: String
}

enum K8sError: LocalizedError {
    case noPods(String)
    case noRunningPods
    case invalidResponse
    case httpError(Int, String)
    case execTimeout
    case execFailed(String)

    var errorDescription: String? {
        switch self {
        case .noPods(let selector): return "No pods found matching '\(selector)'"
        case .noRunningPods: return "No running pods found"
        case .invalidResponse: return "Invalid HTTP response"
        case .httpError(let code, let body): return "HTTP \(code): \(body.prefix(200))"
        case .execTimeout: return "Command execution timed out"
        case .execFailed(let msg): return "Exec failed: \(msg)"
        }
    }
}
