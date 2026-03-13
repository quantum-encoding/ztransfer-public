//go:build darwin || linux

// Web-based remote desktop viewer for human operators.
//
// Serves a single-page app from the ztransfer API that displays live
// screenshots of the remote machine and forwards mouse/keyboard events
// through the encrypted tunnel. Think lightweight VNC, but over ztransfer's
// infrastructure.
//
// Usage:
//
//	# Start a computer session first
//	curl -X POST http://localhost:9877/api/remote/computer/start \
//	  -d '{"code":"warp-429-delta"}'
//
//	# Open the viewer in your browser
//	open http://localhost:9877/viewer?session=cu-abc123
package api

import (
	"fmt"
	"net/http"
)

// RegisterViewerRoutes serves the web-based remote desktop viewer.
func (s *Server) RegisterViewerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/viewer", s.handleViewer)
}

func (s *Server) handleViewer(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		// Show session picker if no session specified
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, viewerLobbyHTML)
		return
	}

	// Verify session exists
	computerSessions.RLock()
	_, ok := computerSessions.m[sessionID]
	computerSessions.RUnlock()
	if !ok {
		http.Error(w, "Session not found", 404)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, viewerHTML, sessionID)
}

const viewerLobbyHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ztransfer — Remote Viewer</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, monospace;
    background: #0a0a0a; color: #e0e0e0;
    display: flex; flex-direction: column; align-items: center;
    justify-content: center; min-height: 100vh;
  }
  h1 { font-size: 1.5rem; margin-bottom: 1rem; color: #00ff88; }
  .card {
    background: #1a1a1a; border: 1px solid #333; border-radius: 8px;
    padding: 2rem; width: 400px; text-align: center;
  }
  input {
    background: #111; border: 1px solid #444; color: #e0e0e0;
    padding: 0.75rem; border-radius: 4px; width: 100%;
    font-family: inherit; font-size: 1rem; margin: 0.5rem 0;
  }
  input:focus { outline: none; border-color: #00ff88; }
  button {
    background: #00ff88; color: #0a0a0a; border: none;
    padding: 0.75rem 2rem; border-radius: 4px; font-weight: 600;
    font-size: 1rem; cursor: pointer; margin-top: 0.5rem; width: 100%;
  }
  button:hover { background: #00cc6a; }
  .sessions { margin-top: 1rem; text-align: left; }
  .session-item {
    display: flex; justify-content: space-between; align-items: center;
    padding: 0.5rem; border-bottom: 1px solid #222;
  }
  .session-item a { color: #00ff88; text-decoration: none; }
  .session-item a:hover { text-decoration: underline; }
  .peer { color: #888; font-size: 0.85rem; }
  #status { color: #888; margin-top: 0.5rem; font-size: 0.85rem; }
  .modes { display: flex; gap: 0.5rem; margin-top: 1rem; }
  .mode {
    flex: 1; padding: 0.75rem; border: 1px solid #333; border-radius: 4px;
    text-align: center; cursor: pointer; font-size: 0.8rem;
  }
  .mode:hover { border-color: #00ff88; }
  .mode.active { border-color: #00ff88; background: #0d3320; }
  .mode h3 { color: #00ff88; font-size: 0.9rem; margin-bottom: 0.25rem; }
</style>
</head>
<body>
<div class="card">
  <h1>ztransfer viewer</h1>
  <p style="color:#888;margin-bottom:1rem">Connect to a remote machine</p>
  <input id="code" placeholder="Warp code (e.g. warp-429-delta)" autofocus>
  <div class="modes">
    <div class="mode active" onclick="setMode('view')" id="mode-view">
      <h3>View</h3>
      Watch only
    </div>
    <div class="mode" onclick="setMode('control')" id="mode-control">
      <h3>Control</h3>
      Mouse + keyboard
    </div>
  </div>
  <button onclick="connect()">Connect</button>
  <div id="status"></div>
  <div class="sessions" id="sessions"></div>
</div>
<script>
let mode = 'view';
function setMode(m) {
  mode = m;
  document.querySelectorAll('.mode').forEach(e => e.classList.remove('active'));
  document.getElementById('mode-' + m).classList.add('active');
}
async function connect() {
  const code = document.getElementById('code').value.trim();
  if (!code) return;
  document.getElementById('status').textContent = 'Connecting...';
  try {
    const res = await fetch('/api/remote/computer/start', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({code: code})
    });
    const data = await res.json();
    if (data.ok) {
      window.location.href = '/viewer?session=' + data.data.session + '&mode=' + mode;
    } else {
      document.getElementById('status').textContent = 'Error: ' + data.error;
    }
  } catch(e) {
    document.getElementById('status').textContent = 'Connection failed: ' + e.message;
  }
}
// Load existing sessions
fetch('/api/remote/computer/sessions').then(r=>r.json()).then(data => {
  if (data.ok && data.data && data.data.length > 0) {
    const el = document.getElementById('sessions');
    el.innerHTML = '<p style="color:#666;font-size:0.8rem;margin-bottom:0.5rem">Active sessions:</p>';
    data.data.forEach(s => {
      el.innerHTML += '<div class="session-item"><a href="/viewer?session=' +
        s.session + '">' + s.session + '</a><span class="peer">' +
        s.peer_name + '</span></div>';
    });
  }
});
document.getElementById('code').addEventListener('keydown', e => {
  if (e.key === 'Enter') connect();
});
</script>
</body>
</html>`

const viewerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ztransfer — Remote Desktop</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, monospace;
    background: #0a0a0a; color: #e0e0e0; overflow: hidden;
  }
  #toolbar {
    position: fixed; top: 0; left: 0; right: 0; height: 36px;
    background: #111; border-bottom: 1px solid #333;
    display: flex; align-items: center; padding: 0 12px; gap: 12px;
    z-index: 100; font-size: 0.8rem;
  }
  #toolbar .logo { color: #00ff88; font-weight: 600; }
  #toolbar .sep { color: #333; }
  #toolbar .info { color: #888; }
  #toolbar .status { margin-left: auto; }
  .status-dot {
    display: inline-block; width: 8px; height: 8px;
    border-radius: 50%%; margin-right: 4px; vertical-align: middle;
  }
  .status-dot.connected { background: #00ff88; }
  .status-dot.disconnected { background: #ff4444; }
  .status-dot.loading { background: #ffaa00; animation: pulse 1s infinite; }
  @keyframes pulse { 50%% { opacity: 0.4; } }
  .btn {
    background: #222; border: 1px solid #444; color: #ccc;
    padding: 2px 8px; border-radius: 3px; cursor: pointer;
    font-size: 0.75rem;
  }
  .btn:hover { border-color: #00ff88; color: #00ff88; }
  .btn.active { background: #0d3320; border-color: #00ff88; color: #00ff88; }
  #canvas-wrap {
    position: fixed; top: 36px; left: 0; right: 0; bottom: 24px;
    display: flex; align-items: center; justify-content: center;
    background: #000;
  }
  canvas {
    max-width: 100%%; max-height: 100%%;
    image-rendering: auto;
  }
  #statusbar {
    position: fixed; bottom: 0; left: 0; right: 0; height: 24px;
    background: #111; border-top: 1px solid #222;
    display: flex; align-items: center; padding: 0 12px;
    font-size: 0.7rem; color: #666; gap: 16px;
  }
  #overlay {
    position: fixed; top: 36px; left: 0; right: 0; bottom: 24px;
    display: flex; align-items: center; justify-content: center;
    background: rgba(0,0,0,0.8); z-index: 50;
  }
  #overlay.hidden { display: none; }
  .overlay-msg { color: #888; font-size: 1.2rem; }
</style>
</head>
<body>
<div id="toolbar">
  <span class="logo">ztransfer</span>
  <span class="sep">|</span>
  <span class="info" id="session-id">%s</span>
  <span class="sep">|</span>
  <button class="btn" id="btn-view" onclick="setMode('view')">View</button>
  <button class="btn active" id="btn-control" onclick="setMode('control')">Control</button>
  <span class="sep">|</span>
  <button class="btn" onclick="requestScreenshot()">Refresh</button>
  <span class="status">
    <span class="status-dot loading" id="status-dot"></span>
    <span id="status-text">Connecting...</span>
  </span>
</div>

<div id="canvas-wrap">
  <canvas id="screen"></canvas>
</div>

<div id="overlay">
  <span class="overlay-msg">Connecting to remote machine...</span>
</div>

<div id="statusbar">
  <span id="fps">-- fps</span>
  <span id="latency">-- ms</span>
  <span id="resolution">--</span>
  <span id="transfer">-- KB</span>
  <span style="margin-left:auto" id="mode-label">Control mode</span>
</div>

<script>
const SESSION = document.getElementById('session-id').textContent;
const canvas = document.getElementById('screen');
const ctx = canvas.getContext('2d');
const overlay = document.getElementById('overlay');
const statusDot = document.getElementById('status-dot');
const statusText = document.getElementById('status-text');

let controlMode = new URLSearchParams(window.location.search).get('mode') !== 'view';
let screenWidth = 0, screenHeight = 0, screenScale = 1;
let refreshInterval = null;
let frameCount = 0, lastFpsTime = Date.now();
let lastLatency = 0, totalBytes = 0;

// Initialize mode buttons
if (!controlMode) {
  document.getElementById('btn-view').classList.add('active');
  document.getElementById('btn-control').classList.remove('active');
  document.getElementById('mode-label').textContent = 'View only';
}

function setMode(m) {
  controlMode = (m === 'control');
  document.getElementById('btn-view').classList.toggle('active', !controlMode);
  document.getElementById('btn-control').classList.toggle('active', controlMode);
  document.getElementById('mode-label').textContent = controlMode ? 'Control mode' : 'View only';
}

function setStatus(state, text) {
  statusDot.className = 'status-dot ' + state;
  statusText.textContent = text;
}

async function requestScreenshot() {
  const t0 = Date.now();
  try {
    const res = await fetch('/api/remote/computer/screen?session=' + SESSION + '&format=png');
    const data = await res.json();
    if (!data.ok) {
      setStatus('disconnected', 'Error: ' + data.error);
      return;
    }

    const img = new Image();
    img.onload = () => {
      if (canvas.width !== img.width || canvas.height !== img.height) {
        canvas.width = img.width;
        canvas.height = img.height;
        screenWidth = img.width;
        screenHeight = img.height;
        document.getElementById('resolution').textContent = img.width + 'x' + img.height;
      }
      ctx.drawImage(img, 0, 0);
      overlay.classList.add('hidden');
      setStatus('connected', 'Connected');

      frameCount++;
      lastLatency = Date.now() - t0;
      totalBytes += data.data.size || 0;
      document.getElementById('latency').textContent = lastLatency + ' ms';
      document.getElementById('transfer').textContent =
        (totalBytes / 1024).toFixed(0) + ' KB';
    };
    img.src = 'data:image/' + (data.data.format || 'jpeg') + ';base64,' + data.data.base64;
  } catch(e) {
    setStatus('disconnected', 'Connection lost');
  }
}

async function sendAction(action) {
  if (!controlMode) return;
  try {
    await fetch('/api/remote/computer/action', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({session: SESSION, action: action})
    });
    // Refresh screen after action
    setTimeout(requestScreenshot, 150);
  } catch(e) { /* ignore */ }
}

// Convert canvas coords to remote screen coords
function canvasToScreen(e) {
  const rect = canvas.getBoundingClientRect();
  const scaleX = canvas.width / rect.width;
  const scaleY = canvas.height / rect.height;
  return {
    x: Math.round((e.clientX - rect.left) * scaleX),
    y: Math.round((e.clientY - rect.top) * scaleY)
  };
}

// Mouse events
canvas.addEventListener('click', (e) => {
  const p = canvasToScreen(e);
  sendAction({type: 'click', x: p.x, y: p.y, button: 'left'});
});

canvas.addEventListener('dblclick', (e) => {
  e.preventDefault();
  const p = canvasToScreen(e);
  sendAction({type: 'double_click', x: p.x, y: p.y});
});

canvas.addEventListener('contextmenu', (e) => {
  e.preventDefault();
  const p = canvasToScreen(e);
  sendAction({type: 'right_click', x: p.x, y: p.y});
});

canvas.addEventListener('mousemove', (e) => {
  // Only send on significant movement to avoid flooding
  if (!controlMode) return;
  // Could add throttled mouse move tracking here if needed
});

// Scroll
canvas.addEventListener('wheel', (e) => {
  e.preventDefault();
  const p = canvasToScreen(e);
  const direction = e.deltaY > 0 ? 'down' : 'up';
  const amount = Math.max(1, Math.abs(Math.round(e.deltaY / 100)));
  sendAction({
    type: 'scroll', scroll_x: p.x, scroll_y: p.y,
    direction: direction, scroll_amount: amount
  });
}, {passive: false});

// Keyboard events
document.addEventListener('keydown', (e) => {
  if (!controlMode) return;
  // Don't capture browser shortcuts
  if (e.metaKey || (e.ctrlKey && ['r','l','t','w','n'].includes(e.key))) return;

  e.preventDefault();

  if (e.key.length === 1 && !e.ctrlKey && !e.altKey) {
    // Printable character — type it
    sendAction({type: 'type', text: e.key});
  } else {
    // Special key
    let key = e.key;
    const keyMap = {
      'ArrowUp': 'Up', 'ArrowDown': 'Down',
      'ArrowLeft': 'Left', 'ArrowRight': 'Right',
      'Backspace': 'Backspace', 'Delete': 'Delete',
      'Enter': 'Return', 'Tab': 'Tab', 'Escape': 'Escape',
      ' ': 'Space',
    };
    if (keyMap[key]) key = keyMap[key];

    // Handle modifier combos
    if (e.ctrlKey && key !== 'Control') key = 'ctrl+' + key;
    if (e.altKey && key !== 'Alt') key = 'alt+' + key;

    sendAction({type: 'key', key: key});
  }
});

// Drag support
let dragStart = null;
canvas.addEventListener('mousedown', (e) => {
  if (!controlMode || e.button !== 0) return;
  dragStart = canvasToScreen(e);
});
canvas.addEventListener('mouseup', (e) => {
  if (!controlMode || !dragStart) return;
  const end = canvasToScreen(e);
  const dx = Math.abs(end.x - dragStart.x);
  const dy = Math.abs(end.y - dragStart.y);
  if (dx > 10 || dy > 10) {
    // This was a drag, not a click
    sendAction({
      type: 'drag',
      start_x: dragStart.x, start_y: dragStart.y,
      end_x: end.x, end_y: end.y
    });
    e.stopPropagation(); // Prevent click handler
  }
  dragStart = null;
});

// FPS counter
setInterval(() => {
  const now = Date.now();
  const elapsed = (now - lastFpsTime) / 1000;
  document.getElementById('fps').textContent =
    (frameCount / elapsed).toFixed(1) + ' fps';
  frameCount = 0;
  lastFpsTime = now;
}, 2000);

// Start polling screenshots
requestScreenshot();
refreshInterval = setInterval(requestScreenshot, 500);
</script>
</body>
</html>`
