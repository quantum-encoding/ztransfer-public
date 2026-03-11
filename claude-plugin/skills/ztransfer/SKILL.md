---
name: ztransfer
description: >
  This skill should be used when the user asks to "transfer a file", "send a file to another machine",
  "receive a file from another machine", "list remote files", "check ztransfer peers", "sync files between machines",
  "get a file from my other computer", "put a file on the server", "copy files over LAN",
  or mentions ztransfer, file transfer between peers, or cross-machine file operations.
  Also triggers when Claude Code needs to programmatically move files between two machines
  on the same network.
version: 0.1.0
---

# ztransfer - Secure LAN File Transfer

Transfer files between machines on the same network using post-quantum authenticated encryption.
This skill enables Claude Code to programmatically send and receive files via the ztransfer REST API.

## Overview

ztransfer is a CLI + API tool for secure LAN file transfers. Two machines pair once using a one-time token,
then transfer files freely using ML-DSA-65 (FIPS 204) digital signatures for authentication over TLS 1.3.

The local REST API (default port 9877) runs on localhost only and requires no authentication,
making it ideal for Claude Code automation via curl.

## Prerequisites

Before using file transfer capabilities:

1. **ztransfer must be installed** on both machines
2. **Machines must be paired** (one-time setup)
3. **The API server must be running** on the local machine: `ztransfer api`

To check if the API is running:
```bash
curl -s http://localhost:9877/ 2>/dev/null | head -1
```

If not running, start it:
```bash
ztransfer api &
```

## Core Workflow

### 1. Check Available Peers

```bash
curl -s http://localhost:9877/api/peers | python3 -m json.tool
```

Returns JSON array of paired peers with name, address, fingerprint, paired_at.

### 2. List Remote Files

```bash
curl -s 'http://localhost:9877/api/ls?peer=PEER_NAME&path=/' | python3 -m json.tool
```

Parameters:
- `peer` (required) - Name of the paired peer
- `path` (optional, default `/`) - Remote directory path to list

### 3. Download a File

```bash
curl -s -X POST http://localhost:9877/api/get \
  -d '{"peer":"PEER_NAME","remote_path":"/path/to/file.txt","local_path":"/tmp/"}'
```

The `local_path` specifies where to save the downloaded file locally.

### 4. Upload a File

```bash
curl -s -X POST http://localhost:9877/api/put \
  -d '{"peer":"PEER_NAME","local_path":"/tmp/file.txt","remote_path":"/"}'
```

### 5. Stream a File (pipe directly)

Download remote file content directly to stdout:
```bash
curl -s 'http://localhost:9877/api/receive?peer=PEER_NAME&path=/file.txt' > file.txt
```

Upload via multipart form:
```bash
curl -s -X POST http://localhost:9877/api/send \
  -F file=@/tmp/file.txt -F peer=PEER_NAME -F remote_path=/
```

## API Endpoints Quick Reference

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

## Common Patterns

### Transfer a file between machines

```bash
# Check peers first
PEER=$(curl -s http://localhost:9877/api/peers | python3 -c "import sys,json; print(json.load(sys.stdin)['data'][0]['name'])")

# Upload
curl -s -X POST http://localhost:9877/api/put \
  -d "{\"peer\":\"$PEER\",\"local_path\":\"$(pwd)/myfile.txt\",\"remote_path\":\"/\"}"

# Download
curl -s -X POST http://localhost:9877/api/get \
  -d "{\"peer\":\"$PEER\",\"remote_path\":\"/myfile.txt\",\"local_path\":\"/tmp/\"}"
```

### Browse and download interactively

```bash
# List root
curl -s 'http://localhost:9877/api/ls?peer=PEER&path=/' | python3 -m json.tool

# List subdirectory
curl -s 'http://localhost:9877/api/ls?peer=PEER&path=/Documents/' | python3 -m json.tool

# Download specific file
curl -s -X POST http://localhost:9877/api/get \
  -d '{"peer":"PEER","remote_path":"/Documents/report.pdf","local_path":"/tmp/"}'
```

## Initial Setup (if not yet paired)

If no peers are available, guide the user through pairing:

1. On the **server** machine: `ztransfer serve --dir /path/to/share`
   - Note the pairing token displayed on startup
2. On the **client** machine: `ztransfer pair SERVER_IP --token TOKEN`
3. Start the API: `ztransfer api`

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
- Connection refused on port 9877 - API server not running

## Additional Resources

### Reference Files

For complete API details including all request/response schemas:
- **`references/api-reference.md`** - Full endpoint documentation with request/response examples
