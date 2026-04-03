# BW Monitor - iOS App

Native iOS version of BW Monitor for monitoring BunkerWeb security events from your iPhone.

## Requirements

- Xcode 15+ with iOS 17 SDK
- [XcodeGen](https://github.com/yonaskolb/XcodeGen) (for project generation)
- An Apple ID for sideloading (no App Store needed)
- A kubeconfig file with access to your BunkerWeb Kubernetes cluster

## Setup

1. Install XcodeGen if you don't have it:
   ```bash
   brew install xcodegen
   ```

2. Generate the Xcode project:
   ```bash
   cd mobile/BWMonitor
   xcodegen generate
   ```

3. Open in Xcode:
   ```bash
   open BWMonitor.xcodeproj
   ```

4. In Xcode, select your Development Team under **Signing & Capabilities**.

5. Connect your iPhone and build/run (Cmd+R).

## Usage

1. Transfer your kubeconfig file to your iPhone (via AirDrop, Files, or iCloud).
2. In the app, tap **Import Kubeconfig File** and select it.
3. Adjust the namespace if needed (default: `bunkerweb`).
4. Tap **Connect**.

The app connects to your Kubernetes cluster, discovers the BunkerWeb Redis pod,
and streams blocked request data using `redis-cli` exec via the Kubernetes API.

### Features

- **Reports tab**: Live view of blocked requests with auto-polling (3s interval)
- **Bans tab**: Active IP bans with pull-to-refresh
- **Detail views**: Full request/ban details with reverse DNS lookup
- **Filter**: Filter by IP, country, server, and date range
- **Exclude IPs**: Swipe left on any row to exclude an IP
- **Pause/Resume**: Pause the live stream from the menu
- **Color-coded IPs**: Same deterministic color palette as the TUI

### Authentication

Supported kubeconfig auth methods:
- Bearer token
- Basic auth (username/password)
- Custom CA certificates (`certificate-authority-data`)
- `insecure-skip-tls-verify`

## Architecture

The app communicates with Redis by executing `redis-cli` commands inside the
Redis pod via the Kubernetes exec WebSocket API (`v4.channel.k8s.io` protocol).
This avoids the need for SPDY port-forwarding on iOS. Lua EVAL scripts are used
to batch Redis commands for efficiency.

```
iPhone App
    |
    v (HTTPS WebSocket)
Kubernetes API Server
    |
    v (exec into pod)
redis-cli in BunkerWeb Redis Pod
    |
    v
Redis data (requests list, bans_* keys)
```
