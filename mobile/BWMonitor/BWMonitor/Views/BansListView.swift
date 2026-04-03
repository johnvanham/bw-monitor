import SwiftUI

struct BansListView: View {
    @Bindable var viewModel: MonitorViewModel

    var body: some View {
        List {
            ForEach(viewModel.filteredBans) { ban in
                NavigationLink(value: ban.key) {
                    BanRow(ban: ban)
                }
                .swipeActions(edge: .trailing) {
                    Button("Exclude") {
                        viewModel.excludeIP(ban.ip)
                    }
                    .tint(.orange)
                }
            }
        }
        .listStyle(.plain)
        .navigationTitle("Active Bans")
        .navigationBarTitleDisplayMode(.inline)
        .navigationDestination(for: String.self) { banKey in
            if let ban = viewModel.bans.first(where: { $0.key == banKey }) {
                BanDetailView(ban: ban, viewModel: viewModel)
            }
        }
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
                Text("\(viewModel.filteredBans.count)/\(viewModel.bans.count)")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
            }
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button {
                        viewModel.refreshBans()
                    } label: {
                        Label("Refresh", systemImage: "arrow.clockwise")
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
                } label: {
                    Image(systemName: "ellipsis.circle")
                }
            }
        }
        .refreshable {
            viewModel.refreshBans()
        }
        .overlay {
            if viewModel.filteredBans.isEmpty {
                ContentUnavailableView {
                    Label("No Bans", systemImage: "hand.raised.slash")
                } description: {
                    if viewModel.filter.isActive {
                        Text("No bans match the current filter")
                    } else {
                        Text("No active bans found")
                    }
                }
            }
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
        .onAppear {
            viewModel.refreshBans()
        }
    }
}

// MARK: - Ban Row

struct BanRow: View {
    let ban: Ban

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(ban.ip)
                    .font(.subheadline.bold().monospaced())
                    .foregroundStyle(IPColor.color(for: ban.ip))

                if !ban.country.isEmpty {
                    Text(flagEmoji(for: ban.country))
                    Text(ban.country)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                Spacer()

                Text("\(ban.events.count)")
                    .font(.caption.bold().monospacedDigit())
                    .foregroundStyle(.white)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(.red.opacity(0.8), in: Capsule())
            }

            HStack(spacing: 8) {
                Text(ban.reason)
                    .font(.caption)
                    .foregroundStyle(.orange)
                    .lineLimit(1)

                Spacer()

                if ban.permanent {
                    Text("PERMANENT")
                        .font(.caption2.bold())
                        .foregroundStyle(.red)
                } else {
                    Text(ban.expiresInText)
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
            }

            HStack {
                Text(ban.service)
                    .font(.caption2)
                    .foregroundStyle(.teal)
                    .lineLimit(1)

                Spacer()

                Text(ban.timestamp, style: .relative)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 2)
    }
}
