import SwiftUI

struct ReportsListView: View {
    @Bindable var viewModel: MonitorViewModel

    var body: some View {
        List {
            ForEach(viewModel.filteredReports) { report in
                NavigationLink(value: report) {
                    ReportRow(report: report)
                }
                .swipeActions(edge: .trailing) {
                    Button("Exclude") {
                        viewModel.excludeIP(report.ip)
                    }
                    .tint(.orange)
                }
            }
        }
        .listStyle(.plain)
        .navigationTitle("Live View")
        .navigationBarTitleDisplayMode(.inline)
        .navigationDestination(for: BlockReport.self) { report in
            ReportDetailView(report: report, viewModel: viewModel)
        }
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
                statusBadge
            }
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button {
                        viewModel.togglePause()
                    } label: {
                        Label(
                            viewModel.isPaused ? "Resume" : "Pause",
                            systemImage: viewModel.isPaused ? "play.fill" : "pause.fill"
                        )
                    }
                    Button {
                        viewModel.showFilter = true
                    } label: {
                        Label("Filter", systemImage: "line.3.horizontal.decrease.circle")
                    }
                    if viewModel.filter.isActive {
                        Button(role: .destructive) {
                            viewModel.clearFilter()
                        } label: {
                            Label("Clear Filter", systemImage: "xmark.circle")
                        }
                    }
                    if !viewModel.excludedIPs.isEmpty {
                        Button {
                            viewModel.showExcludes = true
                        } label: {
                            Label("Excluded IPs (\(viewModel.excludedIPs.count))", systemImage: "eye.slash")
                        }
                    }
                    Divider()
                    Button(role: .destructive) {
                        viewModel.disconnect()
                    } label: {
                        Label("Disconnect", systemImage: "bolt.slash")
                    }
                } label: {
                    Image(systemName: "ellipsis.circle")
                }
            }
        }
        .overlay {
            if viewModel.filteredReports.isEmpty {
                ContentUnavailableView {
                    Label("No Reports", systemImage: "shield.slash")
                } description: {
                    if viewModel.filter.isActive {
                        Text("No reports match the current filter")
                    } else {
                        Text("Waiting for blocked requests...")
                    }
                }
            }
        }
        .safeAreaInset(edge: .bottom) {
            statusBar
        }
        .sheet(isPresented: $viewModel.showFilter) {
            FilterSheet(filter: viewModel.filter) { newFilter in
                viewModel.applyFilter(newFilter)
            }
        }
        .sheet(isPresented: $viewModel.showExcludes) {
            ExcludesSheet(excludedIPs: viewModel.excludedIPs) { ip in
                viewModel.removeExclusion(ip)
            }
        }
    }

    @ViewBuilder
    private var statusBadge: some View {
        if viewModel.isPaused {
            Text("PAUSED")
                .font(.caption.bold())
                .foregroundStyle(.white)
                .padding(.horizontal, 8)
                .padding(.vertical, 3)
                .background(.red, in: Capsule())
        } else if viewModel.isLive {
            HStack(spacing: 4) {
                Circle()
                    .fill(.green)
                    .frame(width: 7, height: 7)
                Text("LIVE")
                    .font(.caption.bold())
                    .foregroundStyle(.green)
            }
        }
    }

    @ViewBuilder
    private var statusBar: some View {
        VStack(spacing: 0) {
            Divider()
            HStack(spacing: 12) {
                Text("\(viewModel.filteredReports.count)/\(viewModel.totalReports)")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)

                if viewModel.filter.isActive {
                    Text(viewModel.filter.summary)
                        .font(.caption2)
                        .foregroundStyle(.yellow)
                        .lineLimit(1)
                }

                if !viewModel.excludedIPs.isEmpty {
                    Text("\(viewModel.excludedIPs.count) excluded")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }

                Spacer()

                if let err = viewModel.lastError {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .font(.caption2)
                        .foregroundStyle(.red)
                    Text(err)
                        .font(.caption2)
                        .foregroundStyle(.red)
                        .lineLimit(1)
                }
            }
            .padding(.horizontal)
            .padding(.vertical, 6)
            .background(.ultraThinMaterial)
        }
    }
}

// MARK: - Report Row

struct ReportRow: View {
    let report: BlockReport

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(report.ip)
                    .font(.subheadline.bold().monospaced())
                    .foregroundStyle(IPColor.color(for: report.ip))

                if !report.country.isEmpty {
                    Text(flagEmoji(for: report.country))
                    Text(report.country)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                Spacer()

                Text(report.timestamp, style: .time)
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
            }

            HStack(spacing: 8) {
                methodBadge(report.method)

                Text("\(report.status)")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(report.status >= 400 ? .red : .secondary)

                Text(report.reason)
                    .font(.caption)
                    .foregroundStyle(.orange)
                    .lineLimit(1)
            }

            HStack {
                Text(report.serverName)
                    .font(.caption2)
                    .foregroundStyle(.teal)
                    .lineLimit(1)

                Text(report.url)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
        }
        .padding(.vertical, 2)
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

// MARK: - Excludes Sheet

struct ExcludesSheet: View {
    let excludedIPs: Set<String>
    let onRemove: (String) -> Void
    @Environment(\.dismiss) private var dismiss

    var sortedIPs: [String] {
        excludedIPs.sorted()
    }

    var body: some View {
        NavigationStack {
            List {
                ForEach(sortedIPs, id: \.self) { ip in
                    HStack {
                        Text(ip)
                            .font(.body.monospaced())
                            .foregroundStyle(IPColor.color(for: ip))
                        Spacer()
                        Button {
                            onRemove(ip)
                        } label: {
                            Image(systemName: "trash")
                                .foregroundStyle(.red)
                        }
                    }
                }
            }
            .navigationTitle("Excluded IPs")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Done") { dismiss() }
                }
            }
            .overlay {
                if excludedIPs.isEmpty {
                    ContentUnavailableView("No Excluded IPs", systemImage: "eye")
                }
            }
        }
    }
}

/// Returns a flag emoji for a 2-letter country code.
func flagEmoji(for countryCode: String) -> String {
    let code = countryCode.uppercased()
    guard code.count == 2 else { return "" }
    let base: UInt32 = 127397
    var emoji = ""
    for scalar in code.unicodeScalars {
        if let flag = Unicode.Scalar(base + scalar.value) {
            emoji.append(String(flag))
        }
    }
    return emoji
}
