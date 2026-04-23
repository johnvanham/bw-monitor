# BW Monitor — Rust TUI

Terminal UI for streaming and inspecting BunkerWeb block reports and active bans from a Kubernetes cluster in real-time.

![Rust](https://img.shields.io/badge/Rust-1.75+-DEA584?logo=rust&logoColor=white)

This is a Rust port of the [Go TUI](../tui-go/) using the Ratatui framework, with identical functionality and UI layout.

## Features

- **Live streaming** of blocked requests from BunkerWeb's Redis store
- **Active bans view** showing currently banned IPs with full event history
- **Colour-coded IPs** — each IP gets a consistent colour from a 20-colour palette, making it easy to spot repeat offenders
- **Detail views** with full report data, parsed user agent info, and async reverse DNS lookup
- **Filterable** by IP, country code, server name, and date/time range
- **IP exclusion** — hide noisy IPs from the list, persisted between sessions
- **Pause/resume** the live stream to inspect entries without the list moving
- **Auto-follow** newest entries, with smart cursor tracking when scrolling history

## Prerequisites

- Rust 1.75+
- `kubectl` configured with access to a cluster running BunkerWeb
- BunkerWeb deployed with its built-in Redis (the `redis-bunkerweb` pod)

## Installation

Build from source:

```bash
cd tui-rust
cargo build --release
./target/release/bw-monitor
```

## Usage

```bash
# Use current kubectl context, default namespace (bunkerweb)
./bw-monitor

# Custom namespace
./bw-monitor --namespace my-bunkerweb-ns

# Limit initial load (default: 10000)
./bw-monitor --max-entries 5000
```

## Key Bindings

Same as the Go TUI version — see [Go TUI README](../tui-go/README.md#key-bindings) for the full key binding reference.

## Architecture

```
Kubernetes Cluster
  └─ redis-bunkerweb pod (:6379)
              │
              │ port-forward (kube-rs)
              │
         localhost:<port>
              │
              │ redis-rs (native Redis protocol)
              ▼
         tui-rust/
           ├─ src/main.rs         Entry point, event loop
           ├─ src/app.rs          Application state and key handling
           ├─ src/k8s.rs          Kubernetes client, pod discovery, port-forward
           ├─ src/redis_client.rs Redis client, report/ban loading
           ├─ src/types.rs        Data models, filter, excludes, IP colours
           └─ src/ui.rs           Ratatui rendering (list, detail, modals)
```

### Built with

- [Ratatui](https://github.com/ratatui/ratatui) — TUI framework
- [Crossterm](https://github.com/crossterm-rs/crossterm) — Terminal manipulation
- [kube-rs](https://github.com/kube-rs/kube) — Kubernetes client
- [redis-rs](https://github.com/redis-rs/redis-rs) — Redis client
- [Tokio](https://github.com/tokio-rs/tokio) — Async runtime

## License

MIT
