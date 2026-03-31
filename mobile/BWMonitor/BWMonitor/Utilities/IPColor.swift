import SwiftUI

/// Deterministic color palette for IP addresses, matching the TUI's 20-color scheme.
enum IPColor {
    private static let palette: [Color] = [
        Color(hex: 0xFF6B6B), // coral red
        Color(hex: 0x4EC9B0), // teal
        Color(hex: 0xDCDCAA), // warm yellow
        Color(hex: 0x569CD6), // soft blue
        Color(hex: 0xC586C0), // purple/magenta
        Color(hex: 0x4FC1FF), // bright cyan
        Color(hex: 0xCE9178), // orange/rust
        Color(hex: 0xB5CEA8), // soft green
        Color(hex: 0xD7BA7D), // gold
        Color(hex: 0x9CDCFE), // ice blue
        Color(hex: 0xF48771), // salmon
        Color(hex: 0x7EC8E3), // sky blue
        Color(hex: 0xC3E88D), // lime green
        Color(hex: 0xF78C6C), // bright orange
        Color(hex: 0xFF79C6), // pink
        Color(hex: 0x8BE9FD), // aqua
        Color(hex: 0x50FA7B), // bright green
        Color(hex: 0xFFB86C), // peach
        Color(hex: 0xBD93F9), // violet
        Color(hex: 0xF1FA8C), // pale yellow
    ]

    /// Returns a deterministic color for a given IP address using FNV-1a hash.
    static func color(for ip: String) -> Color {
        var hash: UInt32 = 2166136261 // FNV offset basis
        for byte in ip.utf8 {
            hash ^= UInt32(byte)
            hash = hash &* 16777619 // FNV prime
        }
        let idx = Int(hash) % palette.count
        return palette[abs(idx)]
    }
}

extension Color {
    init(hex: UInt32) {
        let r = Double((hex >> 16) & 0xFF) / 255.0
        let g = Double((hex >> 8) & 0xFF) / 255.0
        let b = Double(hex & 0xFF) / 255.0
        self.init(red: r, green: g, blue: b)
    }
}
