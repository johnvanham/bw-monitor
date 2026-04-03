# BW Monitor

A multi-platform application for streaming and inspecting BunkerWeb block reports and active bans from a Kubernetes cluster in real-time.

## Variants

| Variant | Directory | Platform | Stack |
|---------|-----------|----------|-------|
| [Go TUI](tui-go/) | `tui-go/` | Terminal | Go, Bubble Tea v2, Lip Gloss, go-redis |
| [Rust TUI](tui-rust/) | `tui-rust/` | Terminal | Rust, Ratatui, Crossterm |
| [PWA](pwa/) | `pwa/` | Browser / Mobile | Vanilla JS, Service Worker, Node.js proxy |
| [iOS](mobile/BWMonitor/) | `mobile/` | iOS | Swift, SwiftUI |

### Go TUI (`tui-go/`)

The original terminal UI. Connects via kubectl port-forward to the BunkerWeb Redis pod using a native Redis client (go-redis). Full-featured with live streaming, filtering, IP exclusion, detail views, and colour-coded IPs.

```bash
cd tui-go && go build -o bw-monitor . && ./bw-monitor
```

### Rust TUI (`tui-rust/`)

A Rust port of the Go TUI using the Ratatui framework. Mirrors the same UI layout and functionality — reports list, bans list, detail views, filtering, IP exclusion, and live polling. Connects to Redis via Kubernetes exec WebSocket.

```bash
cd tui-rust && cargo build --release && ./target/release/bw-monitor
```

### PWA (`pwa/`)

Progressive Web App that runs in any browser. Includes a Node.js proxy server that handles kubeconfig auth and proxies Kubernetes API requests (solving CORS). Can be added to the home screen for an app-like experience.

```bash
cd pwa && node server.js
```

### iOS (`mobile/`)

Native iOS app built with SwiftUI. Connects to the Kubernetes API directly using kubeconfig imported on device. Designed for iOS 17+ and can be sideloaded without the App Store.

## Features (all variants)

- **Live streaming** of blocked requests from BunkerWeb's Redis store
- **Active bans view** showing currently banned IPs with full event history
- **Colour-coded IPs** — each IP gets a consistent colour from a 20-colour palette
- **Detail views** with full report data, country info, and reverse DNS lookup
- **Filterable** by IP, country code, server name, and date range
- **IP exclusion** — hide noisy IPs from the list, persisted between sessions

## Prerequisites

- Kubernetes cluster running BunkerWeb with its built-in Redis
- `kubectl` / kubeconfig access to the cluster

## How It Works

All variants read BunkerWeb's block reports from the `requests` list in Redis, and active bans from `bans_*` keys. The TUI variants use port-forwarding or exec to reach Redis inside the cluster. The PWA uses a Node.js proxy to handle auth and CORS. The iOS app connects directly to the Kubernetes API.

## Repository Structure

```
bw-monitor/
├── tui-go/      Go terminal UI (Bubble Tea)
├── tui-rust/    Rust terminal UI (Ratatui)
├── pwa/         Progressive Web App + proxy server
└── mobile/      iOS app (SwiftUI)
```

Each directory is self-contained and can be split into its own repository if needed.

## License

MIT
