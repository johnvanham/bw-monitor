import Foundation
import SwiftUI

/// Connection state for the app.
enum ConnectionState: Equatable {
    case disconnected
    case connecting
    case connected(podName: String)
    case error(String)

    var isConnected: Bool {
        if case .connected = self { return true }
        return false
    }
}

/// Main view model that manages all app state.
@Observable
final class MonitorViewModel {
    // Connection
    var connectionState: ConnectionState = .disconnected
    var kubeConfigYAML: String?
    var kubeConfigFileName: String?
    var namespace: String = "bunkerweb"
    var maxEntries: Int = 5000

    // Data
    var allReports: [BlockReport] = []
    var bans: [Ban] = []
    var totalReports: Int = 0

    // Stream control
    var isPaused: Bool = false
    var isLive: Bool = false

    // Filter
    var filter = Filter()
    var excludedIPs: Set<String> = []

    // UI state
    var showFilter: Bool = false
    var showExcludes: Bool = false
    var lastError: String?

    // DNS cache
    var dnsCache: [String: [String]] = [:]

    // Private
    private var service: MonitorService?
    private var pollTimer: Timer?

    // MARK: - Computed

    var filteredReports: [BlockReport] {
        allReports.filter { report in
            !excludedIPs.contains(report.ip) &&
            (!filter.isActive || filter.matchesReport(report))
        }
    }

    var filteredBans: [Ban] {
        bans.filter { ban in
            !excludedIPs.contains(ban.ip) &&
            (!filter.isActive || filter.matchesBan(ban))
        }
    }

    // MARK: - Connection

    func connect() {
        guard let yaml = kubeConfigYAML else {
            connectionState = .error("No kubeconfig loaded")
            return
        }

        connectionState = .connecting
        lastError = nil

        Task { @MainActor in
            do {
                let config = try KubeConfig.parse(yaml: yaml)
                let k8sClient = KubernetesClient(config: config)
                let monitor = MonitorService(k8sClient: k8sClient, namespace: namespace)
                let podName = try await monitor.connect()
                self.service = monitor
                self.connectionState = .connected(podName: podName)
                await loadInitialData()
                startPolling()
            } catch {
                self.connectionState = .error(error.localizedDescription)
            }
        }
    }

    func disconnect() {
        stopPolling()
        service = nil
        allReports = []
        bans = []
        totalReports = 0
        isLive = false
        connectionState = .disconnected
    }

    // MARK: - Data Loading

    @MainActor
    private func loadInitialData() async {
        guard let service else { return }
        do {
            let (reports, total) = try await service.loadInitial(maxEntries: maxEntries)
            self.allReports = reports
            self.totalReports = total
            self.isLive = true
            self.lastError = nil
        } catch {
            self.lastError = "Load failed: \(error.localizedDescription)"
        }
    }

    func refreshBans() {
        guard let service else { return }
        Task { @MainActor in
            do {
                self.bans = try await service.loadBans()
                self.lastError = nil
            } catch {
                self.lastError = "Bans: \(error.localizedDescription)"
            }
        }
    }

    // MARK: - Polling

    private func startPolling() {
        stopPolling()
        pollTimer = Timer.scheduledTimer(withTimeInterval: 3.0, repeats: true) { [weak self] _ in
            self?.poll()
        }
    }

    private func stopPolling() {
        pollTimer?.invalidate()
        pollTimer = nil
    }

    private func poll() {
        guard let service, !isPaused else { return }
        Task { @MainActor in
            do {
                let newReports = try await service.pollNew()
                if !newReports.isEmpty {
                    self.allReports.insert(contentsOf: newReports, at: 0)
                    self.totalReports += newReports.count
                }
                self.lastError = nil
            } catch {
                self.lastError = error.localizedDescription
            }
        }
    }

    // MARK: - Pause / Resume

    func togglePause() {
        isPaused.toggle()
    }

    // MARK: - Filter

    func applyFilter(_ newFilter: Filter) {
        filter = newFilter
        showFilter = false
    }

    func clearFilter() {
        filter.clear()
    }

    // MARK: - Excludes

    func excludeIP(_ ip: String) {
        excludedIPs.insert(ip)
    }

    func removeExclusion(_ ip: String) {
        excludedIPs.remove(ip)
    }

    // MARK: - DNS

    func lookupDNS(for ip: String) {
        guard dnsCache[ip] == nil else { return }
        dnsCache[ip] = ["(looking up...)"]

        Task {
            do {
                let host = CFHostCreateWithName(nil, ip as CFString).takeRetainedValue()
                CFHostStartInfoResolution(host, .names, nil)
                var success: DarwinBoolean = false
                if let names = CFHostGetNames(host, &success)?.takeUnretainedValue() as? [String], success.boolValue {
                    await MainActor.run { self.dnsCache[ip] = names }
                } else {
                    await MainActor.run { self.dnsCache[ip] = ["(no rDNS)"] }
                }
            }
        }
    }

    // MARK: - Kubeconfig Import

    func importKubeConfig(from url: URL) {
        let accessed = url.startAccessingSecurityScopedResource()
        defer { if accessed { url.stopAccessingSecurityScopedResource() } }

        do {
            let yaml = try String(contentsOf: url, encoding: .utf8)
            self.kubeConfigYAML = yaml
            self.kubeConfigFileName = url.lastPathComponent

            // Try to extract namespace from kubeconfig
            if let config = try? KubeConfig.parse(yaml: yaml),
               let ns = config.namespace, !ns.isEmpty {
                // Don't override if user already set a non-default namespace
                if namespace == "bunkerweb" {
                    namespace = ns
                }
            }
        } catch {
            connectionState = .error("Failed to read file: \(error.localizedDescription)")
        }
    }
}
