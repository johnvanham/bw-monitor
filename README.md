# BW Monitor

A terminal UI application for streaming and inspecting BunkerWeb block reports from a Kubernetes cluster in real-time.

![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)

## Features

- **Live streaming** of blocked requests from BunkerWeb's Redis store
- **Colour-coded IPs** — each IP gets a consistent colour from a 20-colour palette, making it easy to spot repeat offenders
- **Detail view** with full report data, parsed user agent info, and async reverse DNS lookup
- **Filterable** by IP, country code, and date/time range
- **Scrollable** detail view for reports with extensive bad behavior history
- **Pause/resume** the live stream to inspect entries without the list moving

## Prerequisites

- Go 1.24+
- `kubectl` configured with access to a cluster running BunkerWeb
- BunkerWeb deployed with its built-in Redis (the `redis-bunkerweb` pod)

## Installation

```bash
go install github.com/johnvanham/bw-monitor@latest
```

Or build from source:

```bash
git clone https://github.com/johnvanham/bw-monitor.git
cd bw-monitor
go build -o bw-monitor .
```

## Usage

```bash
# Use current kubectl context, default namespace (bunkerweb)
./bw-monitor

# Custom namespace
./bw-monitor --namespace my-bunkerweb-ns

# Limit initial load (default: 1000)
./bw-monitor --max-entries 5000
```

## Key Bindings

### List View

| Key | Action |
|-----|--------|
| `Space` | Pause / resume live stream |
| `Up` / `k` | Move selection up |
| `Down` / `j` | Move selection down |
| `PgUp` / `PgDn` | Page up / down |
| `Home` / `End` | Jump to top / bottom (End re-enables auto-follow) |
| `Enter` | View report details |
| `f` | Open filter modal |
| `c` | Clear active filters |
| `q` / `Ctrl+C` | Quit |

### Detail View

| Key | Action |
|-----|--------|
| `Up` / `k` | Scroll up |
| `Down` / `j` | Scroll down |
| `PgUp` / `PgDn` | Page up / down |
| `Home` | Jump to top |
| `Esc` / `q` | Back to list |

### Filter Modal

| Key | Action |
|-----|--------|
| `Tab` / `Down` | Next field |
| `Shift+Tab` / `Up` | Previous field |
| `Enter` | Apply filter |
| `Esc` | Cancel |

Filter fields:
- **IP** — substring match (e.g. `192.168` matches any IP containing that)
- **Country** — two-letter country code (e.g. `GB`, `US`)
- **From / To** — date range in `YYYY-MM-DD HH:MM` format

## How It Works

BW Monitor connects to the Kubernetes cluster using your current kubectl context, discovers the BunkerWeb Redis pod, and opens a port-forward to it. It then connects a native Redis client (go-redis) through the tunnel and reads the `requests` list which contains JSON-encoded block reports. On startup it loads existing reports, then polls every 2 seconds for new entries.

The port-forward is managed automatically — no manual `kubectl port-forward` is needed. The persistent connection is significantly more efficient than per-request exec, as it avoids repeated SPDY/TLS handshakes.

## Architecture

```
Kubernetes Cluster
  └─ redis-bunkerweb pod (:6379)
              │
              │ port-forward (client-go)
              │
         localhost:<port>
              │
              │ go-redis (native Redis protocol)
              ▼
         bw-monitor
           ├─ internal/k8s/     Kubernetes client, pod discovery, port-forward
           ├─ internal/redis/   go-redis client, report parsing, polling
           ├─ internal/model/   Bubble Tea model, views, filter logic
           └─ internal/ui/      Styles, IP colour palette, formatting
```

## License

MIT
