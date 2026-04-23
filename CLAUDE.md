# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Shape

This repo holds **four independent client variants** of the same product (BW Monitor — streams BunkerWeb block reports and active bans from a Kubernetes cluster). Each variant lives in its own top-level directory and can be developed, built, and shipped on its own:

- `tui-go/` — Go terminal UI (Bubble Tea v2, Lip Gloss, go-redis, client-go)
- `tui-rust/` — Rust terminal UI (Ratatui, Crossterm, kube-rs, redis-rs, Tokio)
- `pwa/` — Progressive Web App (vanilla JS + Node.js proxy in `server.js`)
- `mobile/BWMonitor/` — iOS app (SwiftUI, generated via XcodeGen from `project.yml`)

The git history keeps the variants on separate branches that are periodically merged into `main` (`pwa`, `mobile-app`, `tui-rust`, etc.). Treat each directory as its own project — do not try to share code across variants, and when asked to change behavior, confirm which variant(s) the change should apply to.

## Common Commands

Per-variant build/run (from the repo root):

```bash
# Go TUI
cd tui-go && go build -o bw-monitor . && ./bw-monitor
cd tui-go && go test ./...              # run all Go tests
cd tui-go && go test ./internal/redis   # single package
cd tui-go && go vet ./...

# Rust TUI
cd tui-rust && cargo build --release && ./target/release/bw-monitor
cd tui-rust && cargo test
cd tui-rust && cargo clippy --all-targets

# PWA (proxy mode — reads ~/.kube/config server-side)
cd pwa && node server.js                # defaults: --port 3000 --address 0.0.0.0

# iOS (requires macOS + Xcode 15+ + XcodeGen)
cd mobile/BWMonitor && xcodegen generate && open BWMonitor.xcodeproj
```

All runtime CLI flags (both TUIs): `--namespace <ns>` (default `bunkerweb`), `--max-entries <n>`.

## Shared Domain Model (important)

Every variant reads the **same two Redis data shapes** from the BunkerWeb `redis-bunkerweb` pod:

1. The `requests` list — JSON-encoded block reports, appended live. Variants load a bounded initial slice then poll every ~2–3 seconds for new entries.
2. `bans_*` keys — each holds a ban's metadata plus the full history of requests that triggered it.

When adding or changing fields, keep the four variants' parsers in sync — the JSON wire format is the contract. Filter semantics (IP substring, two-letter country, date range), the 20-colour FNV-1a IP palette, and the persisted IP-exclude list are intentionally identical across variants.

## Transport Differences (do not "unify" these)

Variants differ deliberately in how they reach Redis, because the platform constraints differ:

- **tui-go**: `client-go` port-forward → `go-redis` speaks the native Redis protocol over the tunnel. A `redis.Reconnector` (see `tui-go/internal/redis/reconnect.go`) auto-restores the port-forward + client on failure.
- **tui-rust**: `kube-rs` (ws feature) + `redis-rs` over Tokio. Mirrors the Go variant's UX but is structured as `main.rs` (event loop) / `app.rs` (state + keys) / `ui.rs` (Ratatui render) / `k8s.rs` + `redis_client.rs` / `types.rs`.
- **pwa**: Browser cannot port-forward. Either uses the Node.js proxy in `server.js` (recommended — handles kubeconfig + CORS), or in direct mode connects to the K8s API via the exec WebSocket (`v4.channel.k8s.io`) and runs `redis-cli` inside the pod. Lua `EVAL` scripts batch commands to reduce exec overhead.
- **mobile**: Same exec-WebSocket strategy as PWA direct mode (SPDY port-forward isn't viable on iOS). Kubeconfig is parsed on-device and stored in the Keychain/UserDefaults equivalent.

If you're fixing a Redis-access bug, check which transport is in use before assuming a shared root cause.

## Go TUI Internal Layout

`tui-go/internal/` is split by concern, not by screen:

- `k8s/` — pod discovery, port-forward lifecycle
- `redis/` — client, report list loading, ban loading, reconnect logic
- `model/` — Bubble Tea v2 model (reports list, bans list, detail, filter modal, exclude modal, messages). Views are composed with `lipgloss.Place`.
- `ui/` — styles, IP colour palette, formatting helpers

The Bubble Tea `Model` in `internal/model/model.go` is the single source of UI truth; individual files (`list.go`, `bans.go`, `detail.go`, `filter.go`, `excludes.go`) extend it with methods rather than defining separate models.

## PWA Layout

- `server.js` — Node.js proxy; reads kubeconfig, injects auth, proxies K8s API + WebSocket exec.
- `services.js` — kubeconfig parser, K8s client, `MonitorService` (polling + state).
- `app.js` — UI rendering, state, event handlers.
- `sw.js` / `manifest.json` — PWA install + offline shell.

Kubeconfig in **direct mode** is parsed client-side and kept in `localStorage` — never POSTed. In **proxy mode** the kubeconfig stays on the host running `server.js`.

## Persisted State

- TUIs: excluded IPs are persisted to `~/.bw-monitor-excludes` (plain text, one IP per line).
- PWA: excludes + last-used kubeconfig live in `localStorage`.
- iOS: excludes + kubeconfig on device only.

These files/stores are user data — migrations must be backward-compatible.
