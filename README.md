# ztransfer

Secure file transfer and remote access with post-quantum authentication.

Transfer files between machines on your LAN, or remotely control machines across networks using encrypted tunnels with warp codes. Sessions are routed through a Cloud Run relay when direct NAT traversal isn't possible, with tamper-evident audit logging for full transparency.

## Features

- **Post-quantum auth** — ML-DSA-65 signatures (FIPS 204) for all operations
- **TLS 1.3** — Encrypted transport with self-signed certs
- **Remote shell** — Interactive PTY over encrypted UDP tunnel
- **Remote exec** — Run commands on remote machines
- **Cloud relay** — HTTP upgrade relay on Cloud Run for when direct connections fail
- **NAT traversal** — STUN + UDP hole punching for cross-network access
- **Warp codes** — Human-readable connection codes (`warp-729-alpha`)
- **Session audit** — Hash-chained, tamper-evident logging of every command and file transfer
- **Computer use** — AI-driven screen capture and mouse/keyboard control on remote machines
- **REST API** — Claude Code integration for programmatic access
- **Cross-platform** — macOS (arm64/amd64) and Linux (amd64/arm64)
- **GUI** — Cross-platform desktop app via Fyne

## Quick Start

### File Transfer (LAN)

**On the machine sharing files:**
```bash
ztransfer serve --dir ~/shared
```

**On the other machine, pair once:**
```bash
ztransfer pair 192.168.1.100:9876 --token ABC123
```

**Then transfer files:**
```bash
ztransfer ls peer:/
ztransfer get peer:/file.txt
ztransfer put ./local-file.txt peer:/
```

### Remote Access

**On the remote machine:**
```bash
ztransfer remote host
# Prints: warp-429-delta
```

**On your machine:**
```bash
# Interactive shell
ztransfer remote shell warp-429-delta

# Run a single command
ztransfer remote exec warp-429-delta "sudo pacman -S brave-bin"
```

### Cloud Relay

When direct peer-to-peer connections aren't possible (strict corporate firewalls, CGNAT), sessions route through the relay. The relay never sees plaintext — it forwards AES-256-GCM encrypted tunnel bytes between paired peers.

```bash
# Host announces via relay
ztransfer remote host --relay https://ztransfer-relay-xxx.run.app

# Client connects via relay
ztransfer remote shell warp-429-delta --relay https://ztransfer-relay-xxx.run.app
```

### Computer Use

Start a computer use session to see and control a remote machine's screen — ideal for GUI-based tasks or AI agent control loops.

```bash
# Start a computer use session (remote machine must be hosting)
curl -s -X POST http://localhost:9877/api/remote/computer/start \
  -d '{"code":"warp-429-delta"}'
# Returns: {"session": "cu-abc123", "screen_info": {"width": 1920, "height": 1080, ...}}

# Take a screenshot
curl -s 'http://localhost:9877/api/remote/computer/screen?session=cu-abc123&format=jpeg&quality=65'

# Click, type, scroll
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"click","x":500,"y":300}}'
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"type","text":"Hello from Claude"}}'

# Stop session
curl -s -X POST http://localhost:9877/api/remote/computer/stop \
  -d '{"session":"cu-abc123"}'
```

### Session Audit

Every remote session produces a tamper-evident audit log — a hash-chained sequence of events where each entry references the SHA-256 hash of the previous one. Modifying, inserting, or removing any event breaks the chain. Audit logs are written as NDJSON files and can optionally stream to BigQuery for centralised monitoring.

### Claude Code API

Start the API server for programmatic access:

```bash
ztransfer api
```

Then from Claude Code or any HTTP client:

```bash
# List peers
curl http://localhost:9877/api/peers

# List remote files
curl 'http://localhost:9877/api/ls?peer=archbox&path=/'

# Download a file
curl -X POST http://localhost:9877/api/get \
  -d '{"peer":"archbox","remote_path":"/data.csv","local_path":"/tmp/"}'

# Execute command on remote machine
curl -X POST http://localhost:9877/api/remote/exec \
  -d '{"code":"warp-429-delta","command":"uname -a"}'

# Start computer use session
curl -X POST http://localhost:9877/api/remote/computer/start \
  -d '{"code":"warp-429-delta"}'

# Screenshot + click
curl 'http://localhost:9877/api/remote/computer/screen?session=cu-abc123'
curl -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"click","x":500,"y":300}}'
```

## Commands

```
ztransfer serve [--dir DIR] [--port PORT]     Start file server
ztransfer pair ADDRESS --token TOKEN          Pair with a server (one-time)
ztransfer ls PEER:/path/                      List remote files
ztransfer get PEER:/path/file [LOCAL_DIR]     Download file
ztransfer put LOCAL_FILE PEER:/path/          Upload file
ztransfer peers                               List paired peers
ztransfer status ADDRESS                      Check server status
ztransfer remote host [--port PORT]           Host a remote session
ztransfer remote shell CODE                   Interactive shell
ztransfer remote exec CODE COMMAND            Run command remotely
ztransfer api [--port PORT]                   Start REST API
ztransfer version                             Show version
```

## How It Works

### File Transfer
1. Server generates a one-time pairing token
2. Client pairs by exchanging ML-DSA-65 public keys
3. All subsequent requests are signed and verified
4. Files transferred over HTTPS/TLS 1.3

### Remote Access
1. Host generates a warp code (e.g., `warp-429-delta`)
2. Both peers discover public endpoints via STUN
3. UDP hole punching establishes a direct tunnel
4. Tunnel encrypted with AES-256-GCM (key derived from warp code)
5. PTY shell or command exec multiplexed over the tunnel

### Cloud Relay
1. Both peers connect to the relay via HTTPS and upgrade to raw TCP
2. Relay pairs peers by warp code hash — it never learns the actual warp code
3. Relay forwards encrypted bytes bidirectionally (32KB buffer, zero-copy)
4. All tunnel data remains AES-256-GCM encrypted end-to-end — the relay is a dumb pipe
5. OIDC JWT authentication validates operator identity via Google Cloud IAM

### Session Audit
1. Session start creates a new hash chain (genesis hash → first event)
2. Every command, output, file transfer, and error is appended as a sealed event
3. Each event contains: SHA-256 hash of its content + reference to previous event's hash
4. Tampering with any event breaks the chain from that point forward
5. Events stream to local NDJSON files and optionally to BigQuery
6. Customers receive the log file and verify independently with `ztransfer-audit`

## Building

```bash
# CLI
go build ./cmd/ztransfer/

# GUI (requires Fyne)
go build ./cmd/ztransfer-gui/
```

Requires CGO for the post-quantum crypto library. Prebuilt static libraries are included for:
- `darwin-arm64` (Apple Silicon Mac)
- `darwin-amd64` (Intel Mac)
- `linux-amd64` (x86_64 Linux)
- `linux-arm64` (ARM64 Linux)

## Project Structure

```
cmd/ztransfer/         CLI binary
cmd/ztransfer-gui/     Desktop GUI (Fyne)
pkg/auth/              Identity, pairing, peer store, TLS
pkg/crypto/            Post-quantum crypto FFI (ML-DSA-65)
pkg/server/            HTTPS file server
pkg/client/            File transfer client
pkg/api/               REST API + remote control endpoints
pkg/nat/               STUN, UDP hole punching, warp codes
pkg/remote/            PTY shell, exec, session management
pkg/relay/             Relay client for Cloud Run routing
pkg/audit/             Hash-chained session audit logging + BigQuery sink
libs/                  Prebuilt quantum_vault static libraries
```

## License

MIT
