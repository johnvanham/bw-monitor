import Foundation
import Yams

/// Parsed Kubernetes cluster configuration from a kubeconfig file.
struct KubeConfig {
    var server: String
    var caCertData: Data?
    var insecureSkipTLSVerify: Bool
    var token: String?
    var username: String?
    var password: String?
    var clientCertData: Data?
    var clientKeyData: Data?
    var namespace: String?

    /// Parses a kubeconfig YAML string and resolves the current context.
    static func parse(yaml: String) throws -> KubeConfig {
        guard let doc = try Yams.load(yaml: yaml) as? [String: Any] else {
            throw KubeConfigError.invalidFormat
        }

        guard let currentContext = doc["current-context"] as? String else {
            throw KubeConfigError.noCurrentContext
        }

        // Resolve context
        guard let contexts = doc["contexts"] as? [[String: Any]],
              let ctx = contexts.first(where: { ($0["name"] as? String) == currentContext }),
              let ctxData = ctx["context"] as? [String: Any] else {
            throw KubeConfigError.contextNotFound(currentContext)
        }

        let clusterName = ctxData["cluster"] as? String ?? ""
        let userName = ctxData["user"] as? String ?? ""
        let namespace = ctxData["namespace"] as? String

        // Resolve cluster
        guard let clusters = doc["clusters"] as? [[String: Any]],
              let cluster = clusters.first(where: { ($0["name"] as? String) == clusterName }),
              let clusterData = cluster["cluster"] as? [String: Any],
              let server = clusterData["server"] as? String else {
            throw KubeConfigError.clusterNotFound(clusterName)
        }

        var caCertData: Data?
        if let caB64 = clusterData["certificate-authority-data"] as? String {
            caCertData = Data(base64Encoded: caB64)
        }
        let insecureSkip = clusterData["insecure-skip-tls-verify"] as? Bool ?? false

        // Resolve user
        var token: String?
        var username: String?
        var password: String?
        var clientCertData: Data?
        var clientKeyData: Data?

        if let users = doc["users"] as? [[String: Any]],
           let user = users.first(where: { ($0["name"] as? String) == userName }),
           let userData = user["user"] as? [String: Any] {
            token = userData["token"] as? String
            username = userData["username"] as? String
            password = userData["password"] as? String

            if let certB64 = userData["client-certificate-data"] as? String {
                clientCertData = Data(base64Encoded: certB64)
            }
            if let keyB64 = userData["client-key-data"] as? String {
                clientKeyData = Data(base64Encoded: keyB64)
            }
        }

        return KubeConfig(
            server: server,
            caCertData: caCertData,
            insecureSkipTLSVerify: insecureSkip,
            token: token,
            username: username,
            password: password,
            clientCertData: clientCertData,
            clientKeyData: clientKeyData,
            namespace: namespace
        )
    }
}

enum KubeConfigError: LocalizedError {
    case invalidFormat
    case noCurrentContext
    case contextNotFound(String)
    case clusterNotFound(String)

    var errorDescription: String? {
        switch self {
        case .invalidFormat: return "Invalid kubeconfig format"
        case .noCurrentContext: return "No current-context defined"
        case .contextNotFound(let name): return "Context '\(name)' not found"
        case .clusterNotFound(let name): return "Cluster '\(name)' not found"
        }
    }
}
