---
name: ztransfer
description: >
  This skill should be used when the user asks to "transfer a file", "send a file to another machine",
  "receive a file from another machine", "list remote files", "check ztransfer peers", "sync files between machines",
  "get a file from my other computer", "put a file on the server", "copy files over LAN",
  "run a command remotely", "execute on remote machine", "remote shell", "connect to remote machine",
  "take a screenshot of remote machine", "control remote screen", "computer use", "click on remote screen",
  or mentions ztransfer, warp codes, file transfer between peers, remote execution, or computer use.
  Also triggers when Claude Code needs to programmatically operate on remote machines
  via ztransfer's secure tunnels.
version: 0.2.0
---

# ztransfer - Secure File Transfer, Remote Execution & Computer Use

Transfer files, run commands, and control remote screens using post-quantum authenticated encryption.
This skill enables Claude Code to programmatically operate on remote machines via the ztransfer REST API.

## Overview

ztransfer is a CLI + API tool for secure LAN file transfers and remote access. Two machines pair once using a one-time token,
then transfer files freely using ML-DSA-65 (FIPS 204) digital signatures for authentication over TLS 1.3.

For remote execution and computer use, machines connect via **warp codes** — human-readable connection strings
(e.g., `warp-429-delta`) that establish AES-256-GCM encrypted tunnels with NAT traversal or cloud relay fallback.

The local REST API (default port 9877) runs on localhost only and requires no authentication,
making it ideal for Claude Code automation via curl.

## Prerequisites

Before using ztransfer capabilities:

1. **ztransfer must be installed** on both machines
2. **For file transfer**: Machines must be paired (one-time setup)
3. **For remote access**: The remote machine must be hosting a session (provides a warp code)
4. **The API server must be running** on the local machine: `ztransfer api`

To check if the API is running:
```bash
curl -s http://localhost:9877/api/status
```

If not running, start it:
```bash
ztransfer api &
```

## API Endpoints Quick Reference

### File Transfer

| Method | Endpoint | Purpose |
|--------|----------|---------|
| GET | `/api/status` | Server status, identity, peer count |
| GET | `/api/peers` | List all paired peers |
| GET | `/api/ls?peer=X&path=/` | List remote directory |
| POST | `/api/get` | Download file (JSON body) |
| POST | `/api/put` | Upload file (JSON body) |
| POST | `/api/send` | Upload via multipart form |
| GET | `/api/receive?peer=X&path=/file` | Stream remote file |
| GET | `/api/help` | Full API documentation |

### Remote Execution

| Method | Endpoint | Purpose |
|--------|----------|---------|
| POST | `/api/remote/exec` | Execute command on remote machine |
| POST | `/api/remote/connect` | Establish persistent session |
| POST | `/api/remote/disconnect` | Close persistent session |
| GET | `/api/remote/sessions` | List active remote sessions |
| POST | `/api/remote/host` | Start hosting, returns warp code |

### Computer Use

| Method | Endpoint | Purpose |
|--------|----------|---------|
| POST | `/api/remote/computer/start` | Start computer use session |
| GET | `/api/remote/computer/screen` | Capture screenshot |
| POST | `/api/remote/computer/action` | Execute mouse/keyboard action |
| GET | `/api/remote/computer/info` | Get display info |
| POST | `/api/remote/computer/stop` | End computer use session |
| GET | `/api/remote/computer/sessions` | List active sessions |

## Core Workflows

### 1. File Transfer

#### Check Available Peers

```bash
curl -s http://localhost:9877/api/peers | python3 -m json.tool
```

Returns JSON array of paired peers with name, address, fingerprint, paired_at.

#### List Remote Files

```bash
curl -s 'http://localhost:9877/api/ls?peer=PEER_NAME&path=/' | python3 -m json.tool
```

Parameters:
- `peer` (required) - Name of the paired peer
- `path` (optional, default `/`) - Remote directory path to list

#### Download a File

```bash
curl -s -X POST http://localhost:9877/api/get \
  -d '{"peer":"PEER_NAME","remote_path":"/path/to/file.txt","local_path":"/tmp/"}'
```

#### Upload a File

```bash
curl -s -X POST http://localhost:9877/api/put \
  -d '{"peer":"PEER_NAME","local_path":"/tmp/file.txt","remote_path":"/"}'
```

#### Stream a File (pipe directly)

```bash
# Download to stdout
curl -s 'http://localhost:9877/api/receive?peer=PEER_NAME&path=/file.txt' > file.txt

# Upload via multipart form
curl -s -X POST http://localhost:9877/api/send \
  -F file=@/tmp/file.txt -F peer=PEER_NAME -F remote_path=/
```

### 2. Remote Command Execution

Remote execution uses **warp codes** — the remote machine must be hosting a session.

#### Execute a Single Command

```bash
curl -s -X POST http://localhost:9877/api/remote/exec \
  -H 'Content-Type: application/json' \
  -d '{"code":"warp-429-delta","command":"uname -a"}'
```

Request body:
- `code` (required) - Warp code from the remote host
- `command` (required) - Command to execute
- `args` (optional) - Array of arguments
- `dir` (optional) - Working directory

Response:
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

#### Common Remote Exec Patterns

```bash
# Install a package on Arch Linux
curl -s -X POST http://localhost:9877/api/remote/exec \
  -d '{"code":"warp-429-delta","command":"sudo pacman -S --noconfirm brave-bin"}'

# Check system status
curl -s -X POST http://localhost:9877/api/remote/exec \
  -d '{"code":"warp-429-delta","command":"systemctl status NetworkManager"}'

# Edit a config file
curl -s -X POST http://localhost:9877/api/remote/exec \
  -d '{"code":"warp-429-delta","command":"cat /etc/pacman.conf"}'
```

### 3. Computer Use (Screen Control)

Computer use lets you see and control a remote machine's screen — ideal for GUI operations.

#### Start a Computer Use Session

```bash
curl -s -X POST http://localhost:9877/api/remote/computer/start \
  -H 'Content-Type: application/json' \
  -d '{"code":"warp-429-delta"}'
```

Response:
```json
{
  "ok": true,
  "data": {
    "session": "cu-abc123",
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

#### Capture Screenshot

```bash
curl -s 'http://localhost:9877/api/remote/computer/screen?session=cu-abc123&format=jpeg&quality=65'
```

Parameters:
- `session` (required) - Session ID
- `format` (optional) - `jpeg` or `png` (default: jpeg)
- `quality` (optional) - JPEG quality 1-100 (default: 65)
- `scale` (optional) - Scale factor (e.g., 0.5 for half resolution)

Returns JSON with base64-encoded image, or raw image bytes if `Accept: image/*` header is set.

#### Perform Actions

```bash
# Click at coordinates
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"click","x":500,"y":300}}'

# Double click
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"double_click","x":500,"y":300}}'

# Right click
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"right_click","x":500,"y":300}}'

# Type text
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"type","text":"Hello World"}}'

# Press a key
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"key","key":"Return"}}'

# Key with modifier
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"key","key":"ctrl+c"}}'

# Scroll
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"scroll","direction":"down","scroll_amount":3}}'

# Drag
curl -s -X POST http://localhost:9877/api/remote/computer/action \
  -d '{"session":"cu-abc123","action":{"type":"drag","start_x":100,"start_y":200,"end_x":400,"end_y":200}}'
```

Action types: `click`, `double_click`, `right_click`, `move`, `drag`, `key`, `type`, `scroll`

#### Get Display Info

```bash
curl -s 'http://localhost:9877/api/remote/computer/info?session=cu-abc123'
```

#### Stop Session

```bash
curl -s -X POST http://localhost:9877/api/remote/computer/stop \
  -d '{"session":"cu-abc123"}'
```

## Error Handling

All API responses follow this format:
```json
{"ok": true, "message": "...", "data": {...}}
{"ok": false, "error": "error description"}
```

Check the `ok` field to determine success. On failure, the `error` field describes the issue.

Common errors:
- "peer parameter required" - Missing peer name in request
- "peer not found" - Peer name doesn't match any paired peer (check `GET /api/peers`)
- "missing code" - Warp code not provided for remote exec/computer use
- "session not found" - Invalid or expired computer use session ID
- Connection refused on port 9877 - API server not running

## Initial Setup (if not yet paired)

If no peers are available for file transfer, guide the user through pairing:

1. On the **server** machine: `ztransfer serve --dir /path/to/share`
   - Note the pairing token displayed on startup
2. On the **client** machine: `ztransfer pair SERVER_IP --token TOKEN`
3. Start the API: `ztransfer api`

For remote access (exec/computer use), no pairing is needed — just a warp code:

1. On the **remote** machine: `ztransfer remote host` — prints a warp code
2. Use that warp code in API calls from the local machine

## Additional Resources

### Reference Files

For complete API details including all request/response schemas:
- **`references/api-reference.md`** - Full endpoint documentation with request/response examples
