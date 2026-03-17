# ztransfer Setup Guide

How to set up ztransfer between two machines.

## Install

Both machines need the `ztransfer` binary. On macOS:

```bash
# From source (requires Go 1.22+ and CGO)
git clone https://github.com/quantum-encoding/ztransfer-public.git
cd ztransfer-public
make build

# The binary is at bin/ztransfer
sudo cp bin/ztransfer /usr/local/bin/
```

## File Transfer (LAN)

### Step 1: Start the server

On the machine sharing files:

```bash
ztransfer serve --dir ~/shared
```

This prints a **pair token** (e.g., `ABC123`) and starts listening on port 9876.

### Step 2: Pair the client

On the other machine, use the server's IP address and the pair token:

```bash
ztransfer pair 192.168.1.50:9876 --token ABC123
```

Pairing is **one-time** — after this, both machines remember each other permanently.

### Step 3: Transfer files

```bash
# List what's on the server
ztransfer ls my-server:/

# Download a file
ztransfer get my-server:/report.pdf

# Upload a file
ztransfer put ./data.csv my-server:/inbox/
```

## Remote Access (any network)

Remote access works over the internet — no LAN or pairing required.

### Step 1: Host on the remote machine

```bash
ztransfer remote host
```

This prints a **warp code** like `warp-429-delta`. Share this with the person connecting.

### Step 2: Connect from the other machine

```bash
# Interactive shell
ztransfer remote shell warp-429-delta

# Run a single command
ztransfer remote exec warp-429-delta "uname -a"
```

### Connection modes

- **Direct (LAN):** Automatic if both machines are on the same network
- **NAT Traversal:** STUN + UDP hole punching for machines behind different NATs
- **Cloud Relay:** Automatic fallback when direct connection fails (requires Google login)

## Cloud Relay Authentication

For connections that go through the relay (cross-network), you need to authenticate:

### Option A: Google Login (interactive)

```bash
ztransfer login
```

Opens your browser for Google sign-in. Credentials are saved to `~/.ztransfer/credentials.json`.

### Option B: Admin authorization

If someone has admin access, they can authorize your Google account:

```bash
# Admin runs:
ztransfer admin authorize clive@gmail.com --scope relay
```

Then you login:

```bash
ztransfer login
```

### Option C: Token (automated/headless)

For scripts, CI/CD, or machines without a browser:

```bash
# Mint a token (requires GCP credentials)
export ZTRANSFER_RELAY_TOKEN=$(ztransfer-mint --scope relay)

# Or set it manually
export ZTRANSFER_RELAY_TOKEN="eyJhbGciOiJS..."
```

## API Mode (for Claude Code / AI agents)

Start the REST API for programmatic access:

```bash
ztransfer api &
```

This runs on `localhost:9877`. See [API.md](API.md) for full endpoint documentation.

Quick test:

```bash
curl http://localhost:9877/api/status
curl http://localhost:9877/api/peers
```

## Check Everything Works

```bash
# Local status + relay health
ztransfer status

# Version info
ztransfer version
```

## GUI

```bash
ztransfer-gui
```

Desktop app with 5 tabs: Transfer, Files, Remote, Connect, Settings.

## Platform Notes

**macOS:** Grant Accessibility permission (System Settings > Privacy > Accessibility) for remote input injection. Screen Recording permission for screenshots.

**Linux:** Install `xdotool` (X11) or `ydotool` (Wayland) for remote input. For screenshots: `grim` (Wayland) or `scrot`/`import` (X11).

## Troubleshooting

| Issue | Fix |
|-------|-----|
| "peer not found" | Run `ztransfer peers` to check paired machines |
| Connection refused on :9876 | Server not running — start with `ztransfer serve` |
| Connection refused on :9877 | API not running — start with `ztransfer api` |
| Relay auth failed | Run `ztransfer login` or set `ZTRANSFER_RELAY_TOKEN` |
| "invalid warp code" | Should look like `warp-429-delta` (3 NATO words) |
| Port in use | Use `--port 9877` to pick a different port |
