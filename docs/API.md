# ztransfer API Documentation

Complete reference for the ztransfer REST API. This API runs on **localhost:9877** and lets you programmatically transfer files, run remote commands, and control remote screens.

## Getting Started

```bash
# Start the API server
ztransfer api

# Check it's running
curl http://localhost:9877/api/status
```

The API binds to `127.0.0.1` only (not network-accessible). No authentication needed for local access.

## Endpoint Summary

### File Transfer (requires paired peers)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/status` | Server status, identity, peer list |
| GET | `/api/peers` | List paired peers |
| GET | `/api/ls?peer=X&path=/` | List remote files |
| POST | `/api/get` | Download file from peer |
| POST | `/api/put` | Upload file to peer |
| POST | `/api/send` | Upload via multipart form |
| GET | `/api/receive?peer=X&path=/file` | Stream remote file |

### Remote Execution (requires warp code)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/remote/exec` | Run command on remote machine |
| POST | `/api/remote/host` | Start hosting (returns warp code) |
| POST | `/api/remote/connect` | Persistent session |
| POST | `/api/remote/disconnect` | Close session |
| GET | `/api/remote/sessions` | List active sessions |

### Computer Use (requires warp code)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/remote/computer/start` | Start screen control session |
| GET | `/api/remote/computer/screen` | Capture screenshot |
| POST | `/api/remote/computer/action` | Mouse/keyboard action |
| GET | `/api/remote/computer/info` | Display resolution info |
| POST | `/api/remote/computer/stop` | End session |
| GET | `/api/remote/computer/sessions` | List active sessions |

### Other

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/` | Health check |
| GET | `/api/help` | Full API docs as JSON |
| GET | `/viewer` | Web-based remote desktop |

---

## Response Format

All endpoints return JSON:

```json
{"ok": true, "message": "...", "data": {...}}
{"ok": false, "error": "error description"}
```

---

## File Transfer

### Prerequisites

Machines must be **paired** first (one-time setup):

```bash
# On the server (machine sharing files):
ztransfer serve --dir ~/shared

# On the client (note the pair token from server output):
ztransfer pair 192.168.1.50:9876 --token ABC123
```

### List Peers

```bash
curl -s http://localhost:9877/api/peers
```

### List Remote Files

```bash
curl -s 'http://localhost:9877/api/ls?peer=linux-box&path=/'
```

### Download a File

```bash
curl -s -X POST http://localhost:9877/api/get \
  -d '{"peer":"linux-box","remote_path":"/report.pdf","local_path":"/tmp/"}'
```

### Upload a File

```bash
curl -s -X POST http://localhost:9877/api/put \
  -d '{"peer":"linux-box","local_path":"/tmp/data.csv","remote_path":"/"}'
```

### Stream Download (pipe to file)

```bash
curl -s 'http://localhost:9877/api/receive?peer=linux-box&path=/data.csv' > data.csv
```

### Stream Upload (multipart)

```bash
curl -s -X POST http://localhost:9877/api/send \
  -F file=@data.csv -F peer=linux-box -F remote_path=/inbox/
```

---

## Remote Execution

### Prerequisites

The **remote machine** must be hosting a session:

```bash
# On the remote machine:
ztransfer remote host
# → prints a warp code like: warp-429-delta
```

No pairing needed. The warp code establishes an AES-256-GCM encrypted tunnel.

### Run a Command

```bash
curl -s -X POST http://localhost:9877/api/remote/exec \
  -d '{"code":"warp-429-delta","command":"uname -a"}'
```

Response:
```json
{"ok": true, "data": {"stdout": "Linux archbox 6.12.1...\n", "stderr": "", "exit_code": 0}}
```

Shell-style commands work — if `args` is empty and `command` contains spaces, it's auto-split:

```bash
curl -s -X POST http://localhost:9877/api/remote/exec \
  -d '{"code":"warp-429-delta","command":"sudo pacman -S --noconfirm brave-bin"}'
```

### Host a Session via API

```bash
curl -s -X POST http://localhost:9877/api/remote/host
```

Returns a warp code that others can use to connect.

---

## Computer Use

Screen capture and mouse/keyboard control for AI agents or remote desktop.

### Start a Session

```bash
curl -s -X POST http://localhost:9877/api/remote/computer/start \
  -d '{"code":"warp-429-delta"}'
```

Response includes a `session` ID and screen dimensions:
```json
{
  "ok": true,
  "data": {
    "session": "cu-a1b2c3d4e5f6g7h8",
    "peer_name": "archbox",
    "screen_info": {"width": 1920, "height": 1080, "scale": 1.0, "os": "linux"}
  }
}
```

### Take a Screenshot

```bash
# JSON response with base64 image
curl -s 'http://localhost:9877/api/remote/computer/screen?session=cu-abc123&format=jpeg&quality=65'

# Raw image bytes (save to file)
curl -s -H 'Accept: image/jpeg' \
  'http://localhost:9877/api/remote/computer/screen?session=cu-abc123' > screen.jpg
```

Parameters: `session` (required), `format` (jpeg/png), `quality` (1-100), `scale`

### Perform Actions

```bash
# Click
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"click","x":500,"y":300}}'

# Type text
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"type","text":"Hello World"}}'

# Press a key
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"key","key":"Return"}}'

# Key with modifier (Ctrl+C)
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"key","key":"ctrl+c"}}'

# Scroll down
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"scroll","direction":"down","scroll_amount":3}}'

# Double click
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"double_click","x":500,"y":300}}'

# Right click
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"right_click","x":500,"y":300}}'

# Drag
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"drag","start_x":100,"start_y":200,"end_x":400,"end_y":200}}'

# Move mouse
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"move","x":500,"y":300}}'
```

**Action types:** `click`, `double_click`, `right_click`, `move`, `drag`, `key`, `type`, `scroll`, `wait`

**Key names:** `Return`, `Tab`, `Escape`, `BackSpace`, `Delete`, `space`, `Up`, `Down`, `Left`, `Right`, `Home`, `End`, `PageUp`, `PageDown`, `F1`-`F12`

**Modifiers:** `ctrl+`, `alt+`, `shift+`, `super+` (combinable: `ctrl+shift+s`)

### Stop a Session

```bash
curl -s -X POST http://localhost:9877/api/remote/computer/stop \
  -d '{"session":"cu-abc123"}'
```

### Web Viewer

Open `http://localhost:9877/viewer` in a browser for a visual remote desktop with mouse and keyboard support.

---

## Error Reference

| Error | Fix |
|-------|-----|
| Connection refused on :9877 | Run `ztransfer api` |
| `"peer not found"` | Check `GET /api/peers` for correct name |
| `"code and command required"` | Include `code` and `command` in exec request |
| `"session not found"` | Session expired or wrong ID — start a new one |
| `"connect failed"` | Remote not hosting, wrong warp code, or network issue |
| `"invalid warp code"` | Format should be `warp-XXX-XXXX` (3 NATO words) |

## Security

- **API:** Localhost-only, no auth needed
- **File transfer:** ML-DSA-65 (FIPS 204) signatures over TLS 1.3
- **Remote/computer use:** AES-256-GCM encrypted tunnels via warp codes
- **Audit:** Computer use sessions produce tamper-evident hash-chained logs
- **Pairing:** Trust On First Use (TOFU) — exchange keys once via one-time token
