import SwiftUI

struct BanDetailView: View {
    let ban: Ban
    @Bindable var viewModel: MonitorViewModel

    var body: some View {
        List {
            Section("Ban Information") {
                DetailField(label: "IP Address", value: ban.ip, color: IPColor.color(for: ban.ip))

                if let names = viewModel.dnsCache[ban.ip] {
                    DetailField(label: "rDNS", value: names.joined(separator: ", "))
                }

                HStack {
                    Text("Country")
                        .foregroundStyle(.secondary)
                    Spacer()
                    if !ban.country.isEmpty {
                        Text(flagEmoji(for: ban.country))
                        Text(countryName(for: ban.country))
                    }
                }

                DetailField(label: "Service", value: ban.service, valueColor: .teal)
                DetailField(label: "Reason", value: ban.reason, valueColor: .orange)
                DetailField(label: "Ban Scope", value: ban.banScope)
            }

            Section("Timing") {
                DetailField(label: "Banned At", value: ban.timestamp.formatted(.dateTime))

                if ban.permanent {
                    DetailField(label: "Expires", value: "Never (permanent)", valueColor: .red)
                } else {
                    DetailField(label: "Expires In", value: ban.expiresInText)
                    DetailField(label: "Expires At", value: ban.expiresAt.formatted(.dateTime))
                }
            }

            if !ban.events.isEmpty {
                Section("Events (\(ban.events.count) requests led to this ban)") {
                    ForEach(Array(ban.events.enumerated()), id: \.element.id) { index, event in
                        VStack(alignment: .leading, spacing: 4) {
                            HStack {
                                Text("[\(index + 1)]")
                                    .font(.caption.bold())
                                    .foregroundStyle(.yellow)

                                Text(event.timestamp, style: .time)
                                    .font(.caption.monospacedDigit())

                                methodBadge(event.method)

                                Text("-> \(event.status)")
                                    .font(.caption.monospacedDigit())
                                    .foregroundStyle(.secondary)
                            }

                            Text(event.url)
                                .font(.caption2.monospaced())
                                .foregroundStyle(.secondary)
                                .lineLimit(2)
                        }
                        .padding(.vertical, 2)
                    }
                }

                if let first = ban.events.first {
                    Section("Summary") {
                        Text("Ban triggered after \(ban.events.count) requests in \(first.countTime)s (threshold: \(first.threshold))")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
        }
        .navigationTitle("Ban Detail")
        .navigationBarTitleDisplayMode(.inline)
        .onAppear {
            viewModel.lookupDNS(for: ban.ip)
        }
    }

    private func methodBadge(_ method: String) -> some View {
        Text(method)
            .font(.caption2.bold())
            .padding(.horizontal, 5)
            .padding(.vertical, 1)
            .background(methodColor(method).opacity(0.2), in: RoundedRectangle(cornerRadius: 3))
            .foregroundStyle(methodColor(method))
    }

    private func methodColor(_ method: String) -> Color {
        switch method.uppercased() {
        case "GET": return .blue
        case "POST": return .green
        case "PUT", "PATCH": return .orange
        case "DELETE": return .red
        default: return .secondary
        }
    }
}
