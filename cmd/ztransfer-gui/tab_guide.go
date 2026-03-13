//go:build darwin || linux

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// BuildGuideTab creates a tabbed guide explaining all ztransfer features.
func (c *Controller) BuildGuideTab() fyne.CanvasObject {
	tabs := container.NewAppTabs(
		container.NewTabItem("Overview", buildGuideOverview()),
		container.NewTabItem("File Transfer", buildGuideTransfer()),
		container.NewTabItem("Remote Access", buildGuideRemote()),
		container.NewTabItem("Computer Use", buildGuideComputerUse()),
		container.NewTabItem("Audit Logs", buildGuideAudit()),
		container.NewTabItem("Tokens", buildGuideTokens()),
		container.NewTabItem("Security", buildGuideSecurity()),
		container.NewTabItem("CLI Reference", buildGuideCLI()),
	)
	tabs.SetTabLocation(container.TabLocationLeading)

	return tabs
}

func guideSection(md string) fyne.CanvasObject {
	rt := widget.NewRichTextFromMarkdown(md)
	rt.Wrapping = fyne.TextWrapWord
	return container.NewVScroll(rt)
}

func buildGuideOverview() fyne.CanvasObject {
	return guideSection(`# ztransfer

Secure file transfer and remote access with **post-quantum authentication**.

## How It Works

ztransfer uses **ML-DSA-65** (FIPS 204) digital signatures for all operations. Every file transfer and remote session is authenticated with post-quantum cryptography that protects against both classical and quantum computer attacks.

## Three Connection Modes

### Direct (LAN)
Machines on the same network connect directly over TLS 1.3. The server generates a one-time pairing token, the client pairs by exchanging public keys, and all subsequent requests are signed and verified.

### NAT Traversal
For machines behind different NATs, ztransfer uses STUN to discover public endpoints and UDP hole punching to establish a direct tunnel. The tunnel is encrypted with AES-256-GCM using a key derived from the warp code.

### Cloud Relay
When direct connections fail (strict corporate firewalls, CGNAT), sessions route through a Cloud Run relay. The relay **never sees plaintext** — it forwards encrypted tunnel bytes between peers paired by warp code hash. The relay is a dumb pipe; all data remains end-to-end encrypted.

## Tabs

- **Transfer** — Send and receive files between paired machines
- **Files** — Browse and manage local files shared by the server
- **Remote** — Connect to remote machines via warp code
- **Server** — Start/stop the file sharing server
- **Peers** — Manage paired machines
- **Audit** — View and verify tamper-evident session logs
- **Tokens** — Mint scoped authentication tokens
- **Settings** — Preferences, identity info, and this guide
`)
}

func buildGuideTransfer() fyne.CanvasObject {
	return guideSection(`# File Transfer

## Sending Files

1. Start the server on the machine sharing files (Server tab)
2. On the receiving machine, pair with the server (Peers tab)
3. In the Transfer tab, select the peer and browse their files
4. Click **Download** to receive files, or drag local files to upload

## Pairing

Pairing is a one-time process. The server generates a random token that you enter on the client. During pairing, both machines exchange ML-DSA-65 public keys. After pairing, all requests are cryptographically signed — no passwords needed.

## Drag & Drop

You can drag files directly onto the transfer window to upload them to the selected peer.

## CLI Equivalent

` + "```" + `
# On the server
ztransfer serve --dir ~/shared

# On the client (pair once)
ztransfer pair 192.168.1.50:9876 --token ABC123

# Transfer files
ztransfer ls peer:/
ztransfer get peer:/document.pdf
ztransfer put ./report.csv peer:/inbox/
` + "```" + `
`)
}

func buildGuideRemote() fyne.CanvasObject {
	return guideSection(`# Remote Access

## Warp Codes

A **warp code** is a human-readable connection identifier like ` + "`warp-429-delta`" + `. When a machine hosts a remote session, it generates a warp code that the connecting machine uses to find it.

Warp codes are:
- **6 bytes** encoded as 3 NATO-phonetic words
- **Checksummed** to catch typos
- Used to derive the AES-256-GCM encryption key

## Connection Modes

### Terminal
Interactive PTY shell over the encrypted tunnel. Full terminal emulation with resize support. Use the CLI for this mode — Fyne doesn't have a terminal widget.

### Viewer (Control)
Live screenshot stream of the remote desktop with mouse and keyboard forwarding. Like lightweight VNC over an encrypted tunnel. Screenshots are JPEG-compressed (107KB per frame) for bandwidth efficiency.

### Viewer (Watch)
Same live screenshot stream but **read-only** — no mouse or keyboard input is forwarded. Good for monitoring or guided troubleshooting where the remote user keeps control.

### Computer Use (AI)
Headless mode for AI agents (Claude, GPT, Gemini). The AI sends actions (click, type, scroll) and receives screenshots through the REST API. Used for automated diagnostics and repair.

## How to Connect

1. On the remote machine, run ` + "`ztransfer remote host`" + `
2. Note the warp code it displays
3. In the Remote tab, enter the warp code and select a mode
4. Click **Connect**

## CLI Equivalent

` + "```" + `
# Host a session
ztransfer remote host

# Connect interactively
ztransfer remote shell warp-429-delta

# Execute a single command
ztransfer remote exec warp-429-delta "uname -a"
` + "```" + `
`)
}

func buildGuideComputerUse() fyne.CanvasObject {
	return guideSection(`# Computer Use (AI Agents)

## What It Is

Computer Use lets AI agents (Claude, GPT-4, Gemini) see and control a remote computer through ztransfer's encrypted tunnel. The AI receives screenshots and sends mouse/keyboard actions — the same way a human would use the Viewer, but automated.

## Architecture

` + "```" + `
AI Provider ←→ REST API ←→ Encrypted Tunnel ←→ Remote Machine
(Claude)       (localhost)  (AES-256-GCM)      (screen + input)
` + "```" + `

## REST API Endpoints

` + "```" + `
POST /api/remote/computer/start   — Connect and start session
GET  /api/remote/computer/screen  — Capture screenshot (JPEG/PNG)
POST /api/remote/computer/action  — Execute mouse/keyboard action
GET  /api/remote/computer/info    — Get display resolution/scale
POST /api/remote/computer/stop    — End session
GET  /api/remote/computer/sessions — List active sessions
` + "```" + `

## Supported Actions

- **click** — Left click at (x, y)
- **double_click** — Double click at (x, y)
- **right_click** — Right click at (x, y)
- **move** — Move mouse to (x, y)
- **drag** — Click and drag from start to end coordinates
- **key** — Press a key (Return, Tab, Escape, F1-F12, etc.)
- **type** — Type text characters
- **scroll** — Scroll up/down/left/right at a position
- **wait** — Pause for 1 second

## Web Viewer

Open ` + "`http://localhost:9877/viewer`" + ` in your browser for a web-based remote desktop experience. The viewer refreshes at ~2fps with JPEG compression (~107KB per frame).

## Platform Requirements

**macOS:** Grant Terminal accessibility permission in System Settings → Privacy & Security → Accessibility. Screen Recording permission is also needed.

**Linux:** Install ` + "`xdotool`" + ` (X11) or ` + "`ydotool`" + ` (Wayland) for input injection. For screenshots: ` + "`grim`" + ` (Wayland) or ` + "`scrot`" + `/` + "`import`" + ` (X11).
`)
}

func buildGuideAudit() fyne.CanvasObject {
	return guideSection(`# Session Audit Logs

## Tamper-Evident Logging

Every remote session produces a **hash-chained audit log**. Each event in the log contains:

- A SHA-256 hash of its own content
- A reference to the previous event's hash

This creates a **blockchain-like chain** where modifying, inserting, or removing any event breaks the chain from that point forward.

## What's Logged

- **session_start** — Session opened, with relay/scope/version metadata
- **command_exec** — Every command executed on the remote machine
- **command_output** — Exit codes and output byte counts
- **file_transfer** — Every file uploaded or downloaded
- **auth_challenge** / **auth_result** — Authentication events
- **error** — Any errors during the session
- **heartbeat** — Screenshots captured (for computer use sessions)
- **session_end** — Session closed, with resolution/outcome

## Verification

Open an NDJSON audit log in the **Audit tab** to:

1. Verify the hash chain is intact (no tampering)
2. Browse individual events with timestamps
3. View event details including hash values
4. See session summary (commands, files, data transferred)

## CLI Verification

Customers can independently verify their logs:

` + "```" + `
# Verify chain integrity
ztransfer-audit verify session.log

# Get a readable report
ztransfer-audit summary session.log

# Machine-readable output
ztransfer-audit verify --json session.log
` + "```" + `

## BigQuery Streaming

Audit events can stream to BigQuery in real time for centralised monitoring and customer-facing dashboards. Configure the BigQuery sink with project, dataset, and table IDs.
`)
}

func buildGuideTokens() fyne.CanvasObject {
	return guideSection(`# Token Minting

## What Are Tokens For?

When ztransfer sessions route through the Cloud Run relay, the relay requires **OIDC identity tokens** for authentication. Interactive users get these automatically via gcloud, but automated workflows (AI agents on GCP VMs, CI/CD pipelines) need a programmatic way to mint tokens.

## Scopes

Tokens are scoped to limit what the bearer can do:

| Scope | Purpose |
|-------|---------|
| **relay** | Authenticate to the Cloud Run relay |
| **diagnostic** | Read-only system inspection |
| **repair** | Full repair session access |
| **full** | All permissions |

## Token Types

| Type | Purpose |
|------|---------|
| **identity** | OIDC token for Cloud Run authentication |
| **access** | OAuth2 token for direct GCP API calls |

## Token Sources

ztransfer-mint tries these sources in order:

1. **GCP Metadata Server** — On GCP VMs, zero configuration needed
2. **Application Default Credentials** — From ` + "`gcloud auth application-default login`" + `
3. **gcloud CLI** — Falls back to the gcloud identity token command

## Service Account

Tokens are minted by impersonating a dedicated service account:
` + "`ztransfer-agent@ztransfer-gcp-relay.iam.gserviceaccount.com`" + `

This service account has ` + "`roles/run.invoker`" + ` on the relay service, so minted tokens can authenticate to the relay.

## CLI Usage

` + "```" + `
# Mint a relay token
export ZTRANSFER_RELAY_TOKEN=$(ztransfer-mint --scope relay)

# Verbose output showing source and audience
ztransfer-mint --scope relay -v

# Access token for API calls
ztransfer-mint --type access --scope relay
` + "```" + `
`)
}

func buildGuideSecurity() fyne.CanvasObject {
	return guideSection(`# Security Architecture

## Post-Quantum Cryptography

ztransfer uses **ML-DSA-65** (FIPS 204, formerly CRYSTALS-Dilithium) for all digital signatures. This algorithm is resistant to attacks from both classical and quantum computers.

- **Key size:** 1952 bytes (public) / 4032 bytes (secret)
- **Signature size:** 3293 bytes
- **Security level:** NIST Level 3 (equivalent to AES-192)

## Trust Model

ztransfer uses **Trust On First Use (TOFU)** pairing. The first time you connect to a peer, you verify their identity via a one-time pairing token. After pairing, the peer's public key is stored locally and all future connections are verified cryptographically.

## Transport Security

- **TLS 1.3** for all direct file transfers
- **AES-256-GCM** for all tunnel traffic (warp code derived key)
- The relay **never** sees plaintext — it forwards encrypted bytes

## Relay Security

The Cloud Run relay:
- Requires OIDC JWT authentication (Google Cloud IAM)
- Pairs peers by **warp code hash** — never learns the actual code
- Forwards bytes bidirectionally with no inspection or logging
- Is stateless — sessions are cleaned up on disconnect

## Audit Trail

Every remote session produces a tamper-evident log. The hash chain ensures that if any event is modified, inserted, or removed, the chain breaks and verification fails. Customers can independently verify their logs.

## Permissions

**macOS:** Accessibility permission for keyboard/mouse injection. Screen Recording for screenshots.

**Linux:** No special permissions for CLI. GUI input injection requires xdotool (X11) or ydotool (Wayland).
`)
}

func buildGuideCLI() fyne.CanvasObject {
	return guideSection(`# CLI Reference

## File Transfer

` + "```" + `
ztransfer serve [--dir DIR] [--port PORT]   Start file server
ztransfer pair ADDRESS --token TOKEN        Pair with a server
ztransfer ls PEER:/path/                    List remote files
ztransfer get PEER:/path/file [LOCAL_DIR]   Download file
ztransfer put LOCAL_FILE PEER:/path/        Upload file
ztransfer peers                             List paired peers
ztransfer status ADDRESS                    Check server status
` + "```" + `

## Remote Access

` + "```" + `
ztransfer remote host [--port PORT]   Host a remote session
ztransfer remote shell CODE           Interactive shell via warp code
ztransfer remote exec CODE COMMAND    Run command remotely
` + "```" + `

## API Server

` + "```" + `
ztransfer api [--port PORT]           Start REST API (default 9877)
` + "```" + `

## Token Minting

` + "```" + `
ztransfer-mint --scope SCOPE          Mint scoped auth token
  --scope relay|diagnostic|repair|full
  --type identity|access              Token type (default: identity)
  -v                                  Verbose output
` + "```" + `

## Audit Verification

` + "```" + `
ztransfer-audit verify FILE           Verify audit log chain
ztransfer-audit summary FILE          Session audit report
  --json                              Machine-readable output
` + "```" + `

## Environment Variables

| Variable | Purpose |
|----------|---------|
| ` + "`ZTRANSFER_RELAY_URL`" + ` | Custom relay URL (default: production) |
| ` + "`ZTRANSFER_RELAY_TOKEN`" + ` | Bearer token for relay auth |
| ` + "`ZTRANSFER_RELAY=off`" + ` | Disable relay, direct connections only |

## GUI

` + "```" + `
ztransfer-gui                         Launch desktop application
` + "```" + `
`)
}
