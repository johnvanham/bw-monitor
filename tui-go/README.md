# BW Monitor — Go TUI

Terminal UI for streaming and inspecting BunkerWeb block reports and active bans from a Kubernetes cluster in real-time.

![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)

## Features

- **Live streaming** of blocked requests from BunkerWeb's Redis store
- **Active bans view** showing currently banned IPs with full event history
- **Colour-coded IPs** — each IP gets a consistent colour from a 20-colour palette, making it easy to spot repeat offenders
- **Detail views** with full report data, parsed user agent info, country names, and async reverse DNS lookup
- **Filterable** by IP, country code, and date/time range
- **IP exclusion** — hide noisy IPs from the list, persisted between sessions
- **Pause/resume** the live stream to inspect entries without the list moving
- **Auto-follow** newest entries, with smart cursor tracking when scrolling history

## Prerequisites

- Go 1.24+
- `kubectl` configured with access to a cluster running BunkerWeb
- BunkerWeb deployed with its built-in Redis (the `redis-bunkerweb` pod)

## Installation

```bash
go install github.com/johnvanham/bw-monitor/tui-go@latest
```

Or build from source:

```bash
git clone https://github.com/johnvanham/bw-monitor.git
cd bw-monitor/tui-go
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

### Reports List (default view)

| Key | Action |
|-----|--------|
| `1` | Switch to reports view |
| `2` | Switch to bans view |
| `Space` | Pause / resume live stream |
| `Up` / `k` | Move selection up |
| `Down` / `j` | Move selection down |
| `PgUp` / `PgDn` | Page up / down |
| `Home` | Jump to newest (re-enables auto-follow) |
| `End` | Jump to oldest loaded entry |
| `Enter` | View report details |
| `f` | Open filter modal |
| `c` | Clear active filters |
| `x` | Exclude selected IP from list |
| `X` | View/manage excluded IPs |
| `q` / `Ctrl+C` | Quit |

### Bans List

| Key | Action |
|-----|--------|
| `1` | Switch to reports view |
| `2` | Switch to bans view |
| `Up` / `k` | Move selection up |
| `Down` / `j` | Move selection down |
| `Enter` | View ban details |
| `r` | Refresh bans list |
| `q` / `Ctrl+C` | Quit |

### Detail View (reports and bans)

| Key | Action |
|-----|--------|
| `Up` / `k` | Scroll up |
| `Down` / `j` | Scroll down |
| `PgUp` / `PgDn` | Page up / down |
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

### Exclude IPs Modal

| Key | Action |
|-----|--------|
| `Up` / `Down` | Navigate list |
| `Delete` / `Backspace` | Remove exclusion |
| `Esc` | Close |

Excluded IPs are saved to `~/.bw-monitor-excludes` and persist between sessions.

## How It Works

BW Monitor connects to the Kubernetes cluster using your current kubectl context, discovers the BunkerWeb Redis pod, and opens a port-forward to it. It then connects a native Redis client (go-redis) through the tunnel and reads the `requests` list which contains JSON-encoded block reports. On startup it loads existing reports, then polls every 2 seconds for new entries.

Active bans are read from `bans_*` keys in Redis, each containing the ban metadata and the full list of requests that triggered the ban.

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
         tui-go/
           ├─ internal/k8s/     Kubernetes client, pod discovery, port-forward
           ├─ internal/redis/   go-redis client, report parsing, ban loading
           ├─ internal/model/   Bubble Tea v2 model, viewport-based views,
           │                    filter/exclude modals (lipgloss.Place)
           └─ internal/ui/      Styles, IP colour palette, formatting
```

### Built with

- [Bubble Tea v2](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Bubbles v2](https://github.com/charmbracelet/bubbles) — viewport, textinput components
- [Lip Gloss v2](https://github.com/charmbracelet/lipgloss) — terminal styling and layout
- [go-redis](https://github.com/redis/go-redis) — Redis client
- [client-go](https://github.com/kubernetes/client-go) — Kubernetes API

## License

MIT
