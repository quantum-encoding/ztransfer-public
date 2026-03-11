# ztransfer

Secure file transfer and remote access with post-quantum authentication.

Transfer files between machines on your LAN, or remotely control machines across networks using encrypted tunnels with warp codes.

## Features

- **Post-quantum auth** — ML-DSA-65 signatures (FIPS 204) for all operations
- **TLS 1.3** — Encrypted transport with self-signed certs
- **Remote shell** — Interactive PTY over encrypted UDP tunnel
- **Remote exec** — Run commands on remote machines
- **NAT traversal** — STUN + UDP hole punching for cross-network access
- **Warp codes** — Human-readable connection codes (`warp-729-alpha`)
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

## Building

```bash
go build ./cmd/ztransfer/
```

Requires CGO for the post-quantum crypto library. Prebuilt static libraries are included for:
- `darwin-arm64` (Apple Silicon Mac)
- `darwin-amd64` (Intel Mac)
- `linux-amd64` (x86_64 Linux)
- `linux-arm64` (ARM64 Linux)

## Project Structure

```
cmd/ztransfer/       CLI binary
cmd/ztransfer-gui/   Desktop GUI (Fyne)
pkg/auth/            Identity, pairing, peer store, TLS
pkg/crypto/          Post-quantum crypto FFI (ML-DSA-65)
pkg/server/          HTTPS file server
pkg/client/          File transfer client
pkg/api/             REST API + remote control endpoints
pkg/nat/             STUN, UDP hole punching, warp codes
pkg/remote/          PTY shell, exec, session management
libs/                Prebuilt quantum_vault static libraries
```

## License

MIT
