import SwiftUI

struct ReportDetailView: View {
    let report: BlockReport
    @Bindable var viewModel: MonitorViewModel

    var body: some View {
        List {
            Section("Request") {
                DetailField(label: "Request ID", value: report.id, monospaced: true)
                DetailField(label: "Date/Time", value: report.timestamp.formatted(.dateTime))
            }

            Section("Source") {
                DetailField(label: "IP Address", value: report.ip, color: IPColor.color(for: report.ip))

                if let names = viewModel.dnsCache[report.ip] {
                    DetailField(label: "rDNS", value: names.joined(separator: ", "))
                }

                HStack {
                    Text("Country")
                        .foregroundStyle(.secondary)
                    Spacer()
                    if !report.country.isEmpty {
                        Text(flagEmoji(for: report.country))
                        Text(countryName(for: report.country))
                    }
                }
            }

            Section("Request Details") {
                DetailField(label: "Method", value: report.method)
                DetailField(label: "URL", value: report.url, monospaced: true)
                DetailField(label: "Status", value: "\(report.status)")
                DetailField(label: "Reason", value: report.reason, valueColor: .orange)
                DetailField(label: "Server", value: report.serverName, valueColor: .teal)
                DetailField(label: "Security Mode", value: report.securityMode)
            }

            Section("User Agent") {
                Text(report.userAgent.isEmpty || report.userAgent == "-" ? "(none)" : report.userAgent)
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
            }

            if let details = report.data, !details.isEmpty {
                Section("Bad Behavior History") {
                    ForEach(Array(details.enumerated()), id: \.element.id) { index, detail in
                        VStack(alignment: .leading, spacing: 6) {
                            Text("Event \(index + 1)")
                                .font(.subheadline.bold())
                                .foregroundStyle(.yellow)

                            DetailField(label: "Date", value: detail.timestamp.formatted(.dateTime))
                            DetailField(label: "URL", value: detail.url, monospaced: true)
                            DetailField(label: "Method", value: detail.method)
                            DetailField(label: "Status", value: detail.status)
                            DetailField(label: "Ban Time", value: "\(detail.banTime)s")
                            DetailField(label: "Ban Scope", value: detail.banScope)
                            DetailField(label: "Threshold", value: "\(detail.threshold)")
                            DetailField(label: "Count Time", value: "\(detail.countTime)s")
                        }
                        .padding(.vertical, 4)
                    }
                }
            }
        }
        .navigationTitle("Block Detail")
        .navigationBarTitleDisplayMode(.inline)
        .onAppear {
            viewModel.lookupDNS(for: report.ip)
        }
    }
}

// MARK: - Shared Detail Components

struct DetailField: View {
    let label: String
    let value: String
    var monospaced: Bool = false
    var color: Color?
    var valueColor: Color?

    var body: some View {
        HStack(alignment: .top) {
            Text(label)
                .foregroundStyle(.secondary)
                .frame(minWidth: 80, alignment: .leading)
            Spacer()
            Text(value)
                .font(monospaced ? .caption.monospaced() : .body)
                .foregroundStyle(color ?? valueColor ?? .primary)
                .multilineTextAlignment(.trailing)
                .textSelection(.enabled)
        }
    }
}

/// Returns a full country name from a 2-letter code using system locale.
func countryName(for code: String) -> String {
    Locale.current.localizedString(forRegionCode: code) ?? code
}
