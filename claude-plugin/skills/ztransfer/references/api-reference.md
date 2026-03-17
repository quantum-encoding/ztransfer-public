# ztransfer API Reference v0.2.0

Complete documentation for all ztransfer REST API endpoints.

The API runs on localhost only (127.0.0.1) and requires no authentication.
Default port: 9877.

## Base URL

```
http://localhost:9877
```

## Response Format

All endpoints return JSON:

```json
{"ok": true, "message": "...", "data": {...}}
{"ok": false, "error": "error description"}
```

## Starting the API

```bash
ztransfer api                    # Default port 9877
ztransfer api --port 8080        # Custom port
ztransfer api --name my-machine  # Custom identity name
```

---

# File Transfer Endpoints

These require paired peers. Pair once with `ztransfer pair ADDRESS --token TOKEN`.

## GET /api/status

Server status including identity, fingerprint, and peer information.

```bash
curl -s http://localhost:9877/api/status
```

**Response:**
```json
{
  "ok": true,
  "data": {
    "identity": "my-hostname",
    "fingerprint": "a1b2c3d4e5f6g7h8",
    "peers": ["linux-box", "macbook"],
    "peer_count": 2,
    "download_dir": "."
  }
}
```

## GET /api/peers

List all paired peers with their connection details.

```bash
curl -s http://localhost:9877/api/peers
```

**Response:**
```json
{
  "ok": true,
  "data": [
    {
      "name": "linux-box",
      "address": "192.168.1.100:9876",
      "fingerprint": "d3c1011d0862c1c0",
      "paired_at": "2026-03-10T14:30:00Z"
    }
  ]
}
```

## GET /api/ls

List files in a remote directory on a paired peer.

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| peer | Yes | — | Name of paired peer |
| path | No | / | Remote directory path |

```bash
curl -s 'http://localhost:9877/api/ls?peer=linux-box&path=/Documents/'
```

**Response:**
```json
{
  "ok": true,
  "data": [
    {"name": "report.pdf", "size": 1048576, "mode": "-rw-r--r--", "mod_time": "2026-03-10T12:00:00Z", "is_dir": false},
    {"name": "photos", "size": 0, "mode": "drwxr-xr-x", "mod_time": "2026-03-09T08:00:00Z", "is_dir": true}
  ]
}
```

## POST /api/get

Download a file from a remote peer to a local directory.

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| peer | Yes | — | Name of paired peer |
| remote_path | Yes | — | Path to file on remote peer |
| local_path | No | download_dir | Local directory to save file |

```bash
curl -s -X POST http://localhost:9877/api/get \
  -d '{"peer":"linux-box","remote_path":"/Documents/report.pdf","local_path":"/tmp/"}'
```

**Response:**
```json
{
  "ok": true,
  "message": "Downloaded report.pdf (1048576 bytes)",
  "data": {"file": "report.pdf", "bytes": 1048576, "local_path": "/tmp/report.pdf"}
}
```

## POST /api/put

Upload a local file to a remote peer.

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| peer | Yes | — | Name of paired peer |
| local_path | Yes | — | Path to local file |
| remote_path | No | /filename | Destination path on remote peer |

```bash
curl -s -X POST http://localhost:9877/api/put \
  -d '{"peer":"linux-box","local_path":"/tmp/data.csv","remote_path":"/inbox/"}'
```

**Response:**
```json
{
  "ok": true,
  "message": "Uploaded data.csv (2048 bytes)",
  "data": {"file": "data.csv", "bytes": 2048, "remote_path": "/inbox/data.csv"}
}
```

## POST /api/send

Upload a file via multipart form data. Useful for piping content directly.

| Field | Required | Description |
|-------|----------|-------------|
| file | Yes | File to upload (multipart) |
| peer | Yes | Name of paired peer |
| remote_path | No | Destination path (default: /filename) |

```bash
curl -s -X POST http://localhost:9877/api/send \
  -F file=@/tmp/data.csv -F peer=linux-box -F remote_path=/inbox/
```

**Max upload size:** 256 MB

## GET /api/receive

Stream a remote file directly as the HTTP response body.

| Parameter | Required | Description |
|-----------|----------|-------------|
| peer | Yes | Name of paired peer |
| path | Yes | Path to file on remote peer |

```bash
curl -s 'http://localhost:9877/api/receive?peer=linux-box&path=/data.csv' > data.csv
```

Returns raw file content with `Content-Disposition: attachment` header.

---

# Remote Execution Endpoints

These require a **warp code** from a machine hosting a remote session.
On the remote machine: `ztransfer remote host` (prints a warp code like `warp-429-delta`).

No pairing needed — warp codes establish encrypted tunnels directly.

## POST /api/remote/exec

Execute a command on a remote machine. Creates a one-shot connection.

| Field | Required | Description |
|-------|----------|-------------|
| code | Yes | Warp code from remote host |
| command | Yes | Command to execute (shell-style string or binary name) |
| args | No | Array of arguments (if not embedded in command) |
| dir | No | Working directory on remote machine |
| host | No | Direct host address (bypasses warp code resolution) |

```bash
# Simple command
curl -s -X POST http://localhost:9877/api/remote/exec \
  -d '{"code":"warp-429-delta","command":"uname -a"}'

# Command with args
curl -s -X POST http://localhost:9877/api/remote/exec \
  -d '{"code":"warp-429-delta","command":"ls","args":["-la","/home"]}'

# Shell-style (auto-split on spaces)
curl -s -X POST http://localhost:9877/api/remote/exec \
  -d '{"code":"warp-429-delta","command":"sudo pacman -S --noconfirm brave-bin"}'
```

**Response:**
```json
{
  "ok": true,
  "data": {
    "stdout": "Linux archbox 6.12.1-arch1 ...\n",
    "stderr": "",
    "exit_code": 0
  }
}
```

**Note:** If `args` is empty and `command` contains spaces, the command string is automatically split on whitespace (shell-style). To pass a command with spaces as a single argument, use the `args` array explicitly.

## POST /api/remote/host

Start hosting a remote session on this machine. Returns a warp code that others can use to connect.

```bash
curl -s -X POST http://localhost:9877/api/remote/host
```

**Response:**
```json
{
  "ok": true,
  "message": "Remote session hosted",
  "data": {
    "code": "warp-429-delta",
    "connected": "2026-03-17T10:00:00Z"
  }
}
```

## POST /api/remote/connect

Establish a persistent connection to a remote host (keeps the tunnel open for multiple operations).

| Field | Required | Description |
|-------|----------|-------------|
| code | Yes | Warp code from remote host |
| host | No | Direct host address |

```bash
curl -s -X POST http://localhost:9877/api/remote/connect \
  -d '{"code":"warp-429-delta"}'
```

## POST /api/remote/disconnect

Close a persistent remote session.

```bash
curl -s -X POST http://localhost:9877/api/remote/disconnect \
  -d '{"code":"warp-429-delta"}'
```

## GET /api/remote/sessions

List active remote sessions.

```bash
curl -s http://localhost:9877/api/remote/sessions
```

---

# Computer Use Endpoints

Control a remote machine's screen — capture screenshots and inject mouse/keyboard input.
Ideal for AI agent control loops (Claude, GPT, Gemini).

Requires a warp code from a machine hosting a remote session.

## POST /api/remote/computer/start

Start a computer use session. Returns a session ID used for all subsequent calls.

| Field | Required | Description |
|-------|----------|-------------|
| code | Yes | Warp code from remote host |
| host | No | Direct host address |

```bash
curl -s -X POST http://localhost:9877/api/remote/computer/start \
  -d '{"code":"warp-429-delta"}'
```

**Response:**
```json
{
  "ok": true,
  "message": "Computer use session started",
  "data": {
    "session": "cu-a1b2c3d4e5f6g7h8",
    "peer_name": "archbox",
    "screen_info": {
      "width": 1920,
      "height": 1080,
      "scale": 1.0,
      "os": "linux"
    }
  }
}
```

## GET /api/remote/computer/screen

Capture a screenshot of the remote machine's screen.

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| session | Yes | — | Session ID from /start |
| format | No | jpeg | `jpeg` or `png` |
| quality | No | 65 | JPEG quality 1-100 |
| scale | No | 2 | Scale factor (2 = halve Retina resolution) |

```bash
# Get screenshot as JSON with base64 image
curl -s 'http://localhost:9877/api/remote/computer/screen?session=cu-abc123&format=jpeg&quality=65'

# Get raw image bytes
curl -s -H 'Accept: image/jpeg' \
  'http://localhost:9877/api/remote/computer/screen?session=cu-abc123' > screen.jpg
```

**JSON Response (default):**
```json
{
  "ok": true,
  "data": {
    "format": "jpeg",
    "base64": "/9j/4AAQ...",
    "size": 107520
  }
}
```

**Raw image response** (when `Accept: image/jpeg` or `Accept: image/*` header is set):
Returns raw JPEG/PNG bytes directly.

## POST /api/remote/computer/action

Execute a mouse or keyboard action on the remote machine.

| Field | Required | Description |
|-------|----------|-------------|
| session | Yes | Session ID from /start |
| action | Yes | Action object (see types below) |

### Action Types

**Click:**
```json
{"type": "click", "x": 500, "y": 300}
```

**Double click:**
```json
{"type": "double_click", "x": 500, "y": 300}
```

**Right click:**
```json
{"type": "right_click", "x": 500, "y": 300}
```

**Move mouse:**
```json
{"type": "move", "x": 500, "y": 300}
```

**Drag:**
```json
{"type": "drag", "start_x": 100, "start_y": 200, "end_x": 400, "end_y": 200}
```

**Type text:**
```json
{"type": "type", "text": "Hello World"}
```

**Press key:**
```json
{"type": "key", "key": "Return"}
```
Keys: `Return`, `Tab`, `Escape`, `BackSpace`, `Delete`, `space`, `Up`, `Down`, `Left`, `Right`, `Home`, `End`, `PageUp`, `PageDown`, `F1`-`F12`

**Key with modifier:**
```json
{"type": "key", "key": "ctrl+c"}
```
Modifiers: `ctrl+`, `alt+`, `shift+`, `super+` (combinable: `ctrl+shift+s`)

**Scroll:**
```json
{"type": "scroll", "direction": "down", "scroll_amount": 3}
```
Directions: `up`, `down`, `left`, `right`

**Wait:**
```json
{"type": "wait"}
```
Pauses for 1 second.

### Full curl examples

```bash
# Click
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"click","x":500,"y":300}}'

# Type text
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"type","text":"Hello World"}}'

# Press Enter
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"key","key":"Return"}}'

# Ctrl+C
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"key","key":"ctrl+c"}}'

# Scroll down
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"scroll","direction":"down","scroll_amount":3}}'
```

**Response:**
```json
{
  "ok": true,
  "data": {"success": true, "error": ""}
}
```

## GET /api/remote/computer/info

Get display information for a computer use session.

```bash
curl -s 'http://localhost:9877/api/remote/computer/info?session=cu-abc123'
```

**Response:**
```json
{
  "ok": true,
  "data": {
    "width": 1920,
    "height": 1080,
    "scale": 2.0,
    "os": "darwin"
  }
}
```

## POST /api/remote/computer/stop

End a computer use session. Saves audit log and cleans up resources.

```bash
curl -s -X POST http://localhost:9877/api/remote/computer/stop \
  -d '{"session":"cu-abc123"}'
```

## GET /api/remote/computer/sessions

List all active computer use sessions.

```bash
curl -s http://localhost:9877/api/remote/computer/sessions
```

**Response:**
```json
{
  "ok": true,
  "data": [
    {
      "session": "cu-abc123",
      "peer_name": "archbox",
      "screen_info": {"width": 1920, "height": 1080, "scale": 1.0, "os": "linux"},
      "connected": "2026-03-17T10:00:00Z"
    }
  ]
}
```

---

# Web Viewer

A browser-based remote desktop viewer is available at:

```
http://localhost:9877/viewer
```

- Without `session` param: shows a session picker (lobby)
- With `session=ID&mode=view`: watch-only mode
- With `session=ID&mode=control`: full mouse + keyboard control

Features: live screenshot streaming, click/double-click/right-click, keyboard input with modifiers, scroll, drag, FPS counter, latency metrics.

---

# Other Endpoints

## GET /

Health check.

```json
{"ok": true, "message": "ztransfer API", "data": {"version": "0.2.0", "docs": "GET /api/help", "identity": "my-hostname"}}
```

## GET /api/help

Returns complete endpoint documentation as structured JSON. Useful for AI agents to discover available operations.

---

# Security Model

- **Localhost only**: API binds to 127.0.0.1, not accessible from network
- **No API auth needed**: Only local processes can reach it
- **Peer auth**: All remote operations use ML-DSA-65 (FIPS 204) signatures over TLS 1.3
- **TOFU pairing**: Peers exchange public keys once via one-time token
- **Tunnel encryption**: AES-256-GCM for all remote/computer use traffic
- **Audit logging**: Computer use sessions produce tamper-evident hash-chained logs

# CLI Equivalents

| API | CLI |
|-----|-----|
| GET /api/peers | `ztransfer peers` |
| GET /api/ls?peer=X&path=/ | `ztransfer ls X:/` |
| POST /api/get | `ztransfer get X:/file /tmp/` |
| POST /api/put | `ztransfer put /tmp/file X:/` |
| GET /api/status | `ztransfer status` |
| POST /api/remote/exec | `ztransfer remote exec CODE COMMAND` |
| POST /api/remote/host | `ztransfer remote host` |

# Error Handling

Check the `ok` field. Common errors:

| Error | Cause |
|-------|-------|
| `"peer parameter required"` | Missing peer name |
| `"peer not found"` | Peer name doesn't match any paired peer |
| `"code and command required"` | Missing warp code or command for remote exec |
| `"code required"` | Missing warp code for computer use |
| `"session not found"` | Invalid or expired computer use session ID |
| `"invalid warp code"` | Malformed warp code (should be like `warp-429-delta`) |
| `"connect failed"` | Cannot reach remote host (check warp code, network, relay) |
| Connection refused on :9877 | API server not running — start with `ztransfer api` |

# Quick Start for AI Agents

```bash
# 1. Start the API
ztransfer api &

# 2. For file transfer: check peers
curl -s http://localhost:9877/api/peers

# 3. For remote access: get a warp code from the remote machine
#    (on remote: ztransfer remote host)

# 4. Execute a command
curl -s -X POST http://localhost:9877/api/remote/exec \
  -d '{"code":"warp-429-delta","command":"uname -a"}'

# 5. Or start a computer use session
SESSION=$(curl -s -X POST http://localhost:9877/api/remote/computer/start \
  -d '{"code":"warp-429-delta"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['session'])")

# 6. Take a screenshot
curl -s "http://localhost:9877/api/remote/computer/screen?session=$SESSION&format=jpeg&quality=65"

# 7. Click somewhere
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d "{\"session\":\"$SESSION\",\"action\":{\"type\":\"click\",\"x\":500,\"y\":300}}"

# 8. Clean up
curl -s -X POST http://localhost:9877/api/remote/computer/stop \
  -d "{\"session\":\"$SESSION\"}"
```
