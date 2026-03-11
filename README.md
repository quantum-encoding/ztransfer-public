# ztransfer

Secure LAN file transfer with post-quantum authentication.

Transfer files between machines on your local network using **ML-DSA-65** (FIPS 204) digital signatures for identity verification and **TLS 1.3** for transport encryption. One-time TOFU (Trust On First Use) pairing, then seamless authenticated transfers.

## Features

- **Post-quantum authentication** — ML-DSA-65 signatures (via Zig FFI)
- **TOFU pairing** — One-time token exchange, then permanent trust
- **CLI + GUI** — Full command-line tool and cross-platform Fyne desktop app
- **REST API** — Local HTTP API for programmatic access (great for AI agents)
- **Claude Code plugin** — Included plugin for AI-assisted file transfers
- **Cross-platform** — macOS (arm64/amd64), Linux (amd64/arm64)

## Quick Start

### Install

```bash
# Build from source (requires Go 1.22+ and prebuilt libquantum_vault.a)
git clone https://github.com/quantum-encoding/ztransfer-public.git
cd ztransfer-public
go build -o ztransfer ./cmd/ztransfer/
```

### Usage

**Machine A** (server):
```bash
ztransfer serve --dir ~/shared --port 9876
# Displays a one-time pairing token
```

**Machine B** (client):
```bash
# Pair (one-time)
ztransfer pair 192.168.1.100:9876 --token ABC123

# List files
ztransfer ls machine-a:/

# Download
ztransfer get machine-a:/document.pdf /tmp/

# Upload
ztransfer put ./report.csv machine-a:/inbox/
```

### API Mode

Start the local REST API for programmatic access:

```bash
ztransfer api --port 9877
```

Then use curl (or any HTTP client):

```bash
# List peers
curl http://localhost:9877/api/peers

# List remote files
curl 'http://localhost:9877/api/ls?peer=machine-a&path=/'

# Download
curl -X POST http://localhost:9877/api/get \
  -d '{"peer":"machine-a","remote_path":"/file.txt","local_path":"/tmp/"}'

# Upload
curl -X POST http://localhost:9877/api/put \
  -d '{"peer":"machine-a","local_path":"/tmp/file.txt","remote_path":"/"}'

# Stream a file directly
curl 'http://localhost:9877/api/receive?peer=machine-a&path=/file.txt' > file.txt
```

### GUI

Build and run the cross-platform desktop app:

```bash
go build -o ztransfer-gui ./cmd/ztransfer-gui/
./ztransfer-gui
```

Features tabbed interface with Server, Transfer, Peers, and Settings views. Supports dark and light themes.

## Architecture

```
┌──────────────┐     TLS 1.3 + ML-DSA-65      ┌──────────────┐
│  Machine A   │◄──────────────────────────────►│  Machine B   │
│  (server)    │   Signed request/response      │  (client)    │
│              │                                │              │
│  ztransfer   │                                │  ztransfer   │
│  serve       │                                │  get/put/ls  │
└──────┬───────┘                                └──────┬───────┘
       │                                               │
       ▼                                               ▼
  libquantum_vault.a                            libquantum_vault.a
  (Zig FFI: ML-DSA-65)                         (Zig FFI: ML-DSA-65)
```

### Security Model

1. **Identity**: Each machine generates an ML-DSA-65 keypair on first run (`~/.ztransfer/identity.json`)
2. **Pairing**: One-time token exchange validates both parties, stores public keys (`~/.ztransfer/known_peers.json`)
3. **Authentication**: Every request is signed with ML-DSA-65 and verified by the server
4. **Transport**: TLS 1.3 encrypts all traffic (self-signed certs — trust comes from ML-DSA, not PKI)
5. **API security**: REST API binds to localhost only — no network exposure

### Project Structure

```
ztransfer-public/
├── cmd/
│   ├── ztransfer/          # CLI binary
│   └── ztransfer-gui/      # Fyne desktop app
├── pkg/
│   ├── api/                # Local REST API server
│   ├── auth/               # Identity, pairing, peer management, TLS
│   ├── client/             # HTTP client with ML-DSA auth
│   ├── crypto/             # CGo bindings to Zig quantum vault
│   └── server/             # HTTPS file server with auth middleware
├── libs/
│   ├── include/            # C header for Zig library
│   └── lib/                # Prebuilt static libraries per platform
├── claude-plugin/          # Claude Code plugin for AI-assisted transfers
├── build.sh                # Cross-platform build script
└── Makefile
```

## Building the Zig Library

The `libquantum_vault.a` static library provides ML-DSA-65 and ML-KEM-768 via Zig. Prebuilt binaries are included for darwin-arm64. To build for other platforms from the [quantum-zig-forge](https://github.com/quantum-encoding/quantum-zig-forge) source:

```bash
cd quantum-zig-forge/quantum-vault
zig build -Doptimize=ReleaseFast
# Output: zig-out/lib/libquantum_vault.a
```

Copy the built library to `libs/lib/<os>-<arch>/libquantum_vault.a`.

## Claude Code Plugin

A Claude Code plugin is included at `claude-plugin/`. Install it locally:

```json
// Add to ~/.claude/settings.json under "enabledPlugins":
{
  "/path/to/ztransfer-public/claude-plugin": true
}
```

This gives Claude Code the `/ztransfer` command and auto-triggers on file transfer requests.

## Commands

| Command | Description |
|---------|-------------|
| `serve` | Start file server (share a directory) |
| `api` | Start local REST API for programmatic access |
| `pair` | Pair with a remote server (one-time) |
| `ls` | List files on a paired peer |
| `get` | Download a file from a peer |
| `put` | Upload a file to a peer |
| `peers` | List all paired peers |
| `status` | Check a server's status |
| `version` | Show version info |

## License

MIT - See [LICENSE](LICENSE)

## Author

Quantum Encoding Ltd
