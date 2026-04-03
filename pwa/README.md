# BW Monitor - PWA

Progressive Web App version of BW Monitor for monitoring BunkerWeb security events from any device with a browser.

## Quick Start

### Option 1: Proxy Server (recommended)

The proxy server reads your kubeconfig and proxies Kubernetes API requests, avoiding browser CORS restrictions. Just run:

```bash
cd pwa
node server.js
```

Then open `http://localhost:3000` on your phone or desktop browser. The app auto-detects the proxy and shows your cluster info — just hit Connect.

**Options:**
```
--port PORT          Listen port (default: 3000)
--kubeconfig PATH    Kubeconfig file (default: ~/.kube/config)
--context NAME       Kubeconfig context (default: current-context)
--address ADDR       Listen address (default: 0.0.0.0)
```

### Option 2: Static file server (direct mode)

Serve the `pwa/` directory with any static file server:

```bash
cd pwa
python3 -m http.server 8080
```

Then open `http://localhost:8080` on your phone or desktop browser. In this mode you'll need to import a kubeconfig file, and your K8s API server must have CORS enabled or be accessible without CORS (e.g. via a VPN).

### Other hosting options

- **GitHub Pages**: Push to a `gh-pages` branch
- **Netlify/Vercel**: Point to the `pwa/` directory
- **nginx**: Serve as static files
- **Open directly**: Open `index.html` from the Files app (some PWA features won't work)

## Usage

### Proxy mode (server.js)
1. **Start the server** — `node server.js` (reads kubeconfig automatically)
2. **Open the URL** — on your phone or browser
3. **Connect** — The app shows your cluster info, just set namespace and connect

### Direct mode (static hosting)
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

### Proxy mode (recommended)
```
Browser (PWA)
  └── HTTP/WebSocket ──────> server.js (Node.js proxy)
                                 ├── kubeconfig read server-side
                                 ├── auth headers injected
                                 └── proxied to K8s API Server
                                         └──> redis-cli in BunkerWeb Redis Pod
                                                 └──> Redis data
```

### Direct mode
```
Browser (PWA)
  ├── kubeconfig parsed in JS (client-side only)
  ├── credentials in localStorage (on device)
  └── WebSocket exec ──────> Kubernetes API Server
                                 └──> redis-cli in BunkerWeb Redis Pod
                                         └──> Redis data
```

In proxy mode, the Node.js server handles auth and CORS. In direct mode, all communication goes directly from the browser to your Kubernetes API server.

The app uses the Kubernetes exec WebSocket API (`v4.channel.k8s.io` subprotocol) to run `redis-cli` commands inside the Redis pod. Lua EVAL scripts batch multiple Redis commands per call.

## Requirements

- Your K8s API server must be reachable from the device (VPN, public endpoint, or same network)
- Supported auth: Bearer token, basic auth (username/password)
- For clusters with self-signed CAs, you must trust the CA in your browser/OS settings

## Files

```
pwa/
├── server.js       # Node.js proxy server (optional, solves CORS)
├── index.html      # App shell
├── app.css         # All styles (dark theme, mobile-optimized)
├── services.js     # KubeConfig parser, K8s client, MonitorService
├── app.js          # UI rendering, state management, event handling
├── sw.js           # Service worker for offline/PWA support
├── manifest.json   # PWA manifest
├── icon-192.png    # App icon (192x192)
└── icon-512.png    # App icon (512x512)
```
