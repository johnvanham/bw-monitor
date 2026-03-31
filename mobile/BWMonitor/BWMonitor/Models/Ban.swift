import Foundation

/// JSON payload stored in Redis for each ban key.
struct BanData: Codable {
    var service: String
    var reason: String
    var date: Double
    var country: String
    var banScope: String
    var permanent: Bool
    var reasonData: [BanEvent]?

    enum CodingKeys: String, CodingKey {
        case service, reason, date, country, permanent
        case banScope = "ban_scope"
        case reasonData = "reason_data"
    }
}

/// A currently active IP ban.
struct Ban: Identifiable {
    var key: String
    var ip: String
    var data: BanData
    var ttlSeconds: Int

    var id: String { key }
    var service: String { data.service }
    var reason: String { data.reason }
    var country: String { data.country }
    var banScope: String { data.banScope }
    var permanent: Bool { data.permanent }
    var events: [BanEvent] { data.reasonData ?? [] }

    var timestamp: Date {
        Date(timeIntervalSince1970: data.date)
    }

    var ttl: TimeInterval {
        TimeInterval(ttlSeconds)
    }

    var expiresAt: Date {
        Date().addingTimeInterval(ttl)
    }

    var expiresInText: String {
        if permanent { return "Permanent" }
        let hours = Int(ttl) / 3600
        let mins = (Int(ttl) % 3600) / 60
        if hours > 0 { return "\(hours)h \(mins)m" }
        return "\(mins)m"
    }

    /// Extracts IP from a ban key like "bans_service_example.com_ip_1.2.3.4"
    static func parseIP(from key: String) -> String {
        let parts = key.components(separatedBy: "_ip_")
        return parts.count == 2 ? parts[1] : key
    }
}

/// A single event that contributed to a ban.
struct BanEvent: Codable, Identifiable {
    var id: String
    var ip: String
    var date: Double
    var country: String
    var method: String
    var url: String
    var status: String
    var serverName: String
    var securityMode: String
    var banScope: String
    var banTime: Int
    var countTime: Int
    var threshold: Int

    var timestamp: Date {
        Date(timeIntervalSince1970: date)
    }

    enum CodingKeys: String, CodingKey {
        case id, ip, date, country, method, url, status, threshold
        case serverName = "server_name"
        case securityMode = "security_mode"
        case banScope = "ban_scope"
        case banTime = "ban_time"
        case countTime = "count_time"
    }
}
