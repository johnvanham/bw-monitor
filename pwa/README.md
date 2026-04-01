# BW Monitor - PWA

Progressive Web App version of BW Monitor for monitoring BunkerWeb security events from any device with a browser.

## Quick Start

Serve the `pwa/` directory with any static file server:

```bash
cd pwa
python3 -m http.server 8080
```

Then open `http://localhost:8080` on your phone or desktop browser.

### Other hosting options

- **GitHub Pages**: Push to a `gh-pages` branch
- **Netlify/Vercel**: Point to the `pwa/` directory
- **nginx**: Serve as static files
- **Open directly**: Open `index.html` from the Files app (some PWA features won't work)

## Usage

1. **Import kubeconfig** — Tap the import button and select your kubeconfig file. It's parsed client-side and stored in localStorage (never uploaded).
2. **Set namespace** — Default is `bunkerweb`.
3. **Connect** — The app discovers the Redis pod and starts streaming blocked requests.

### Features

- **Reports tab**: Live-updating list of blocked HTTP requests (3s polling)
- **Bans tab**: Active IP bans with refresh support
- **Detail views**: Full request/ban details with reverse DNS lookup
- **Filter**: Filter by IP, country, server, and date range
- **Exclude IPs**: Long-press any row to exclude an IP
- **Pause/Resume**: Pause the live stream from the menu
- **Color-coded IPs**: Same 20-color FNV-1a palette as the TUI
- **Add to Home Screen**: Works as a standalone app on iOS and Android

## Architecture

```
Browser (PWA)
  ├── kubeconfig parsed in JS (client-side only)
  ├── credentials in localStorage (on device)
  └── WebSocket exec ──────> Kubernetes API Server
                                 └──> redis-cli in BunkerWeb Redis Pod
                                         └──> Redis data
```

All communication goes directly from the browser to your Kubernetes API server. No backend server is needed.

The app uses the Kubernetes exec WebSocket API (`v4.channel.k8s.io` subprotocol) to run `redis-cli` commands inside the Redis pod. Lua EVAL scripts batch multiple Redis commands per call.

## Requirements

- Your K8s API server must be reachable from the device (VPN, public endpoint, or same network)
- Supported auth: Bearer token, basic auth (username/password)
- For clusters with self-signed CAs, you must trust the CA in your browser/OS settings

## Files

```
pwa/
├── index.html      # App shell
├── app.css         # All styles (dark theme, mobile-optimized)
├── services.js     # KubeConfig parser, K8s client, MonitorService
├── app.js          # UI rendering, state management, event handling
├── sw.js           # Service worker for offline/PWA support
├── manifest.json   # PWA manifest
├── icon-192.png    # App icon (192x192)
└── icon-512.png    # App icon (512x512)
```
