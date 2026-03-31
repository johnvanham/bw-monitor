import Foundation

/// Orchestrates Redis data fetching via Kubernetes exec.
actor MonitorService {
    private let k8sClient: KubernetesClient
    private let namespace: String
    private var redisPod: String?
    private var highwater: Int = 0

    init(k8sClient: KubernetesClient, namespace: String) {
        self.k8sClient = k8sClient
        self.namespace = namespace
    }

    /// Discovers the BunkerWeb Redis pod.
    func connect() async throws -> String {
        let pod = try await k8sClient.findRunningPod(
            namespace: namespace,
            labelSelector: "bunkerweb.io/component=redis"
        )
        self.redisPod = pod

        // Verify redis-cli is available
        let pong = try await redisCommand(["PING"])
        guard pong.trimmingCharacters(in: .whitespacesAndNewlines) == "PONG" else {
            throw MonitorError.redisNotReachable
        }
        return pod
    }

    /// Loads the most recent reports from Redis.
    func loadInitial(maxEntries: Int) async throws -> (reports: [BlockReport], total: Int) {
        let script = """
        local len = redis.call('llen', 'requests')
        local max = tonumber(ARGV[1])
        local start = 0
        if max > 0 and len > max then start = len - max end
        if len == 0 then return '0' end
        local items = redis.call('lrange', 'requests', start, -1)
        if #items == 0 then return tostring(len) end
        return tostring(len) .. '\\n' .. table.concat(items, '\\n')
        """
        let output = try await redisEval(script: script, args: [String(maxEntries)])
        return parseReportsOutput(output)
    }

    /// Fetches new reports appended since the last poll.
    func pollNew() async throws -> [BlockReport] {
        let script = """
        local len = redis.call('llen', 'requests')
        if len <= tonumber(ARGV[1]) then return tostring(len) end
        local items = redis.call('lrange', 'requests', tonumber(ARGV[1]), -1)
        if #items == 0 then return tostring(len) end
        return tostring(len) .. '\\n' .. table.concat(items, '\\n')
        """
        let output = try await redisEval(script: script, args: [String(highwater)])
        let (reports, _) = parseReportsOutput(output)
        return reports
    }

    /// Loads all active bans from Redis.
    func loadBans() async throws -> [Ban] {
        let script = """
        local keys = redis.call('keys', 'bans_*')
        if #keys == 0 then return '' end
        local r = {}
        for _, k in ipairs(keys) do
            local v = redis.call('get', k)
            local t = redis.call('ttl', k)
            if v then
                r[#r+1] = k .. '\\t' .. t .. '\\t' .. v
            end
        end
        return table.concat(r, '\\n')
        """
        let output = try await redisEval(script: script, args: [])
        return parseBansOutput(output)
    }

    // MARK: - Private

    private func redisCommand(_ args: [String]) async throws -> String {
        guard let pod = redisPod else { throw MonitorError.notConnected }
        var command = ["redis-cli", "--raw"]
        command.append(contentsOf: args)
        return try await k8sClient.exec(namespace: namespace, pod: pod, command: command)
    }

    private func redisEval(script: String, args: [String]) async throws -> String {
        guard let pod = redisPod else { throw MonitorError.notConnected }
        var command = ["redis-cli", "--raw", "EVAL", script, "0"]
        command.append(contentsOf: args)
        return try await k8sClient.exec(namespace: namespace, pod: pod, command: command, timeoutSeconds: 15)
    }

    private func parseReportsOutput(_ output: String) -> (reports: [BlockReport], total: Int) {
        let trimmed = output.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return ([], 0) }

        let lines = trimmed.components(separatedBy: "\n")
        guard let totalStr = lines.first, let total = Int(totalStr) else {
            return ([], 0)
        }

        highwater = total

        guard lines.count > 1 else { return ([], total) }

        let decoder = JSONDecoder()
        var reports: [BlockReport] = []
        for line in lines.dropFirst() {
            let trimmedLine = line.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !trimmedLine.isEmpty, let data = trimmedLine.data(using: .utf8) else { continue }
            if let report = try? decoder.decode(BlockReport.self, from: data) {
                reports.append(report)
            }
        }

        // Reverse so newest is first
        reports.reverse()
        return (reports, total)
    }

    private func parseBansOutput(_ output: String) -> [Ban] {
        let trimmed = output.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return [] }

        let decoder = JSONDecoder()
        var bans: [Ban] = []

        for line in trimmed.components(separatedBy: "\n") {
            let parts = line.components(separatedBy: "\t")
            guard parts.count == 3 else { continue }
            let key = parts[0]
            let ttl = Int(parts[1]) ?? -1
            let jsonStr = parts[2]

            guard let jsonData = jsonStr.data(using: .utf8),
                  let banData = try? decoder.decode(BanData.self, from: jsonData) else { continue }

            let ban = Ban(
                key: key,
                ip: Ban.parseIP(from: key),
                data: banData,
                ttlSeconds: max(ttl, 0)
            )
            bans.append(ban)
        }

        // Sort newest first
        bans.sort { $0.data.date > $1.data.date }
        return bans
    }
}

enum MonitorError: LocalizedError {
    case notConnected
    case redisNotReachable

    var errorDescription: String? {
        switch self {
        case .notConnected: return "Not connected to a Redis pod"
        case .redisNotReachable: return "Redis is not reachable in the pod"
        }
    }
}
