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
- **Token minting** — Scoped OIDC/OAuth2 tokens for automated and headless workflows
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

### Token Minting (Automated/Headless)

For CI/CD pipelines, GCP VMs, or Claude operator workflows where interactive login isn't possible:

```bash
# Mint a scoped identity token for relay authentication
export ZTRANSFER_RELAY_TOKEN=$(ztransfer-mint --scope relay)

# Token sources (tried in order):
#   1. GCP metadata server (on VMs — zero config)
#   2. Application Default Credentials (gcloud auth application-default login)
#   3. gcloud CLI fallback

# Available scopes
ztransfer-mint --scope relay        # Relay authentication
ztransfer-mint --scope diagnostic   # Read-only diagnostics
ztransfer-mint --scope repair       # Full repair session
ztransfer-mint --scope full         # All permissions

# Access tokens for direct API calls
ztransfer-mint --type access --scope relay
```

### Session Audit

Every remote session produces a tamper-evident audit log — a hash-chained sequence of events where each entry references the SHA-256 hash of the previous one. Modifying, inserting, or removing any event breaks the chain.

```bash
# Verify a session log hasn't been tampered with
ztransfer-audit verify session.log

# Output:
# VERIFIED — 15 events, chain intact
#   Session:  sess-2026-0313-001
#   Operator: operator@company.co.uk
#   Target:   client-laptop-WIN10-JB

# Get a readable session report
ztransfer-audit summary session.log

# Output:
# ═══════════════════════════════════════════════════
#   SESSION AUDIT REPORT
# ═══════════════════════════════════════════════════
#   Session ID:  sess-2026-0313-001
#   Operator:    operator@company.co.uk
#   Target:      client-laptop-WIN10-JB
#   Duration:    12m 34s
#   Chain:       ✓ VERIFIED (all hashes valid)
#   Commands:    6 executed
#   Files:       1 transferred
#   ─── Commands ──────────────────────────────────
#     1. systemctl status nginx
#     2. journalctl -u nginx --since '1 hour ago'
#     ...

# Machine-readable output
ztransfer-audit verify --json session.log
```

Audit logs can also stream to BigQuery in real time for centralised monitoring and customer-facing dashboards.

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

ztransfer-mint --scope SCOPE [--type TYPE]    Mint scoped auth tokens
ztransfer-audit verify FILE                   Verify audit log chain
ztransfer-audit summary FILE                  Session audit report
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
# Main CLI
go build ./cmd/ztransfer/

# Token minter (no CGO required)
go build ./cmd/ztransfer-mint/

# Audit verifier (no CGO required)
go build ./cmd/ztransfer-audit/
```

The main CLI requires CGO for the post-quantum crypto library. Prebuilt static libraries are included for:
- `darwin-arm64` (Apple Silicon Mac)
- `darwin-amd64` (Intel Mac)
- `linux-amd64` (x86_64 Linux)
- `linux-arm64` (ARM64 Linux)

The `ztransfer-mint` and `ztransfer-audit` binaries are pure Go with no CGO dependency.

## Project Structure

```
cmd/ztransfer/         CLI binary
cmd/ztransfer-gui/     Desktop GUI (Fyne)
cmd/ztransfer-mint/    Scoped token minter for automated workflows
cmd/ztransfer-audit/   Session audit log verifier
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
