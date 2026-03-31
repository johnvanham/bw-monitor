import Foundation

/// A single blocked HTTP request from BunkerWeb's Redis store.
struct BlockReport: Identifiable, Codable {
    var id: String
    var ip: String
    var date: Double
    var country: String
    var reason: String
    var method: String
    var url: String
    var status: Int
    var userAgent: String
    var serverName: String
    var securityMode: String
    var synced: Bool
    var data: [BadBehaviorDetail]?

    var timestamp: Date {
        Date(timeIntervalSince1970: date)
    }

    enum CodingKeys: String, CodingKey {
        case id, ip, date, country, reason, method, url, status, synced, data
        case userAgent = "user_agent"
        case serverName = "server_name"
        case securityMode = "security_mode"
    }
}

/// Details from a bad behavior ban event.
struct BadBehaviorDetail: Codable, Identifiable {
    var id: String
    var ip: String
    var date: Double
    var country: String
    var banTime: Int
    var banScope: String
    var threshold: Int
    var url: String
    var serverName: String
    var method: String
    var countTime: Int
    var status: String
    var securityMode: String

    var timestamp: Date {
        Date(timeIntervalSince1970: date)
    }

    enum CodingKeys: String, CodingKey {
        case id, ip, date, country, url, method, status, threshold
        case banTime = "ban_time"
        case banScope = "ban_scope"
        case serverName = "server_name"
        case countTime = "count_time"
        case securityMode = "security_mode"
    }
}
