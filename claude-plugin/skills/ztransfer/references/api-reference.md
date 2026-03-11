# ztransfer API Reference

Complete documentation for all ztransfer REST API endpoints.

The API runs on localhost only (127.0.0.1) and requires no authentication.
Default port: 9877.

## Base URL

```
http://localhost:9877
```

## Response Format

All endpoints return JSON with this structure:

```json
{
  "ok": true,
  "message": "Human-readable description",
  "error": "Error description (only on failure)",
  "data": {}
}
```

---

## GET /

Health check endpoint.

**Response:**
```json
{
  "ok": true,
  "message": "ztransfer API",
  "data": {
    "version": "0.1.0",
    "docs": "GET /api/help",
    "identity": "my-hostname"
  }
}
```

---

## GET /api/status

Server status including identity, fingerprint, and peer information.

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

---

## GET /api/peers

List all paired peers with their connection details.

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

---

## GET /api/ls

List files in a remote directory on a paired peer.

**Query Parameters:**
| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| peer | Yes | - | Name of paired peer |
| path | No | / | Remote directory path |

**Example:**
```bash
curl 'http://localhost:9877/api/ls?peer=linux-box&path=/Documents/'
```

**Response:**
```json
{
  "ok": true,
  "data": [
    {
      "name": "report.pdf",
      "size": 1048576,
      "mode": "-rw-r--r--",
      "mod_time": "2026-03-10T12:00:00Z",
      "is_dir": false
    },
    {
      "name": "photos",
      "size": 0,
      "mode": "drwxr-xr-x",
      "mod_time": "2026-03-09T08:00:00Z",
      "is_dir": true
    }
  ]
}
```

---

## POST /api/get

Download a file from a remote peer to a local directory.

**Request Body (JSON):**
```json
{
  "peer": "linux-box",
  "remote_path": "/Documents/report.pdf",
  "local_path": "/tmp/"
}
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| peer | Yes | - | Name of paired peer |
| remote_path | Yes | - | Path to file on remote peer |
| local_path | No | download_dir | Local directory to save file |

**Response:**
```json
{
  "ok": true,
  "message": "Downloaded report.pdf (1048576 bytes)",
  "data": {
    "file": "report.pdf",
    "bytes": 1048576,
    "local_path": "/tmp/report.pdf"
  }
}
```

---

## POST /api/put

Upload a local file to a remote peer.

**Request Body (JSON):**
```json
{
  "peer": "linux-box",
  "local_path": "/tmp/data.csv",
  "remote_path": "/inbox/"
}
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| peer | Yes | - | Name of paired peer |
| local_path | Yes | - | Path to local file |
| remote_path | No | /filename | Destination path on remote peer |

**Response:**
```json
{
  "ok": true,
  "message": "Uploaded data.csv (2048 bytes)",
  "data": {
    "file": "data.csv",
    "bytes": 2048,
    "remote_path": "/inbox/data.csv"
  }
}
```

---

## POST /api/send

Upload a file via multipart form data. Useful for piping file content directly.

**Form Fields:**
| Field | Required | Description |
|-------|----------|-------------|
| file | Yes | File to upload (multipart) |
| peer | Yes | Name of paired peer |
| remote_path | No | Destination path (default: /filename) |

**Example:**
```bash
curl -X POST http://localhost:9877/api/send \
  -F file=@/tmp/data.csv \
  -F peer=linux-box \
  -F remote_path=/inbox/
```

**Response:**
```json
{
  "ok": true,
  "message": "Sent data.csv to linux-box:/inbox/data.csv (2048 bytes)",
  "data": {
    "file": "data.csv",
    "bytes": 2048,
    "peer": "linux-box",
    "remote_path": "/inbox/data.csv"
  }
}
```

**Max upload size:** 256 MB

---

## GET /api/receive

Stream a remote file directly as the HTTP response body. Returns raw file content
with Content-Disposition header for the filename.

**Query Parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| peer | Yes | Name of paired peer |
| path | Yes | Path to file on remote peer |

**Example:**
```bash
curl 'http://localhost:9877/api/receive?peer=linux-box&path=/data.csv' > data.csv
```

**Response Headers:**
```
Content-Type: application/octet-stream
Content-Disposition: attachment; filename="data.csv"
```

**Response Body:** Raw file content

---

## GET /api/help

Returns complete API documentation as JSON.

**Response:**
```json
{
  "ok": true,
  "data": {
    "description": "ztransfer local API — programmatic file transfer for Claude Code",
    "endpoints": {
      "GET  /api/status": "Server status, identity, and peer list",
      "GET  /api/peers": "List all paired peers with addresses and fingerprints",
      "GET  /api/ls": "List remote files. Params: peer, path (default: /)",
      "POST /api/get": "Download remote file. Body: {peer, remote_path, local_path?}",
      "POST /api/put": "Upload local file. Body: {peer, local_path, remote_path?}",
      "POST /api/send": "Send file via multipart. Fields: file, peer, remote_path",
      "GET  /api/receive": "Stream remote file content. Params: peer, path",
      "GET  /api/help": "This help message"
    },
    "examples": { ... }
  }
}
```

---

## Security Model

- **Localhost only**: API binds to 127.0.0.1, not accessible from network
- **No API auth needed**: Only local processes can reach it
- **Peer auth**: All remote operations use ML-DSA-65 signatures over TLS 1.3
- **TOFU pairing**: Peers exchange public keys once via one-time token

## CLI Equivalents

| API | CLI |
|-----|-----|
| GET /api/peers | `ztransfer peers` |
| GET /api/ls?peer=X&path=/ | `ztransfer ls X:/` |
| POST /api/get | `ztransfer get X:/file /tmp/` |
| POST /api/put | `ztransfer put /tmp/file X:/` |
| GET /api/status | `ztransfer status ADDRESS` |

## Starting the API

```bash
# Default port 9877
ztransfer api

# Custom port
ztransfer api --port 8080

# With custom identity name
ztransfer api --name my-machine
```

The API server runs in the foreground. Use `&` or a process manager to background it.
