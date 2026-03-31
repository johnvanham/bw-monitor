import Foundation

/// Filter criteria for reports and bans.
struct Filter: Equatable {
    var ip: String = ""
    var country: String = ""
    var server: String = ""
    var dateFrom: Date?
    var dateTo: Date?

    var isActive: Bool {
        !ip.isEmpty || !country.isEmpty || !server.isEmpty || dateFrom != nil || dateTo != nil
    }

    var summary: String {
        var parts: [String] = []
        if !ip.isEmpty { parts.append("IP: \(ip)") }
        if !country.isEmpty { parts.append("CC: \(country)") }
        if !server.isEmpty { parts.append("Server: \(server)") }
        if let from = dateFrom {
            parts.append("From: \(Self.shortFormatter.string(from: from))")
        }
        if let to = dateTo {
            parts.append("To: \(Self.shortFormatter.string(from: to))")
        }
        return parts.joined(separator: " / ")
    }

    func matchesReport(_ r: BlockReport) -> Bool {
        if !ip.isEmpty && !r.ip.contains(ip) { return false }
        if !country.isEmpty {
            let codes = country.split(separator: ",").map { $0.trimmingCharacters(in: .whitespaces).uppercased() }
            if !codes.contains(r.country.uppercased()) { return false }
        }
        if !server.isEmpty && !r.serverName.localizedCaseInsensitiveContains(server) { return false }
        if let from = dateFrom, r.timestamp < from { return false }
        if let to = dateTo, r.timestamp > to { return false }
        return true
    }

    func matchesBan(_ b: Ban) -> Bool {
        if !ip.isEmpty && !b.ip.contains(ip) { return false }
        if !country.isEmpty {
            let codes = country.split(separator: ",").map { $0.trimmingCharacters(in: .whitespaces).uppercased() }
            if !codes.contains(b.country.uppercased()) { return false }
        }
        if !server.isEmpty && !b.service.localizedCaseInsensitiveContains(server) { return false }
        if let from = dateFrom, b.timestamp < from { return false }
        if let to = dateTo, b.timestamp > to { return false }
        return true
    }

    mutating func clear() {
        ip = ""
        country = ""
        server = ""
        dateFrom = nil
        dateTo = nil
    }

    private static let shortFormatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd HH:mm"
        return f
    }()
}
