// Command ztransfer-viewer-test runs a local test of the computer use viewer.
// It captures this machine's own screen and serves it through the viewer,
// so you can verify screenshot capture, mouse forwarding, and keyboard
// injection work before testing over a real tunnel.
//
// Usage:
//
//	go run ./cmd/ztransfer-viewer-test/
//	open http://localhost:9878
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/quantum-encoding/ztransfer/pkg/remote"
)

func main() {
	mux := http.NewServeMux()

	// Serve the viewer page directly (embedded, no session needed)
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/remote/computer/screen", handleScreen)
	mux.HandleFunc("/api/remote/computer/action", handleAction)
	mux.HandleFunc("/api/remote/computer/info", handleInfo)
	mux.HandleFunc("/api/remote/computer/sessions", handleSessions)

	addr := "localhost:9878"
	fmt.Printf("ztransfer viewer test server\n")
	fmt.Printf("Open http://%s in your browser\n", addr)
	fmt.Printf("This captures YOUR screen — for testing only\n\n")

	log.Fatal(http.ListenAndServe(addr, mux))
}

type apiResp struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, resp apiResp) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func handleScreen(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "jpeg"
	}

	var data []byte
	var err error

	if format == "jpeg" {
		data, err = remote.CaptureScreenJPEGPublic(65, 2) // quality 65, halve Retina
	} else {
		data, err = remote.CaptureScreenPublic(remote.ScreenRequest{Format: "png"})
	}

	if err != nil {
		writeJSON(w, 500, apiResp{Error: "screenshot failed: " + err.Error()})
		return
	}

	writeJSON(w, 200, apiResp{
		OK: true,
		Data: map[string]any{
			"format": format,
			"base64": base64.StdEncoding.EncodeToString(data),
			"size":   len(data),
		},
	})
}

func handleAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action remote.ComputerAction `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, apiResp{Error: "invalid JSON"})
		return
	}

	err := remote.ExecuteInputPublic(req.Action)
	result := map[string]any{"success": err == nil}
	if err != nil {
		result["error"] = err.Error()
	}

	writeJSON(w, 200, apiResp{OK: err == nil, Data: result})
}

func handleInfo(w http.ResponseWriter, r *http.Request) {
	info, err := remote.GetScreenInfoPublic()
	if err != nil {
		writeJSON(w, 500, apiResp{Error: err.Error()})
		return
	}
	writeJSON(w, 200, apiResp{OK: true, Data: info})
}

func handleSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, apiResp{OK: true, Data: []any{}})
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, testViewerHTML)
}

const testViewerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ztransfer — Local Viewer Test</title>
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
  #toolbar .info { color: #ff8800; }
  .btn {
    background: #222; border: 1px solid #444; color: #ccc;
    padding: 2px 8px; border-radius: 3px; cursor: pointer; font-size: 0.75rem;
  }
  .btn:hover { border-color: #00ff88; color: #00ff88; }
  .btn.active { background: #0d3320; border-color: #00ff88; color: #00ff88; }
  .status-dot {
    display: inline-block; width: 8px; height: 8px;
    border-radius: 50%; margin-right: 4px; vertical-align: middle;
  }
  .status-dot.connected { background: #00ff88; }
  .status-dot.loading { background: #ffaa00; animation: pulse 1s infinite; }
  @keyframes pulse { 50% { opacity: 0.4; } }
  #canvas-wrap {
    position: fixed; top: 36px; left: 0; right: 0; bottom: 24px;
    display: flex; align-items: center; justify-content: center; background: #000;
  }
  canvas { max-width: 100%; max-height: 100%; }
  #statusbar {
    position: fixed; bottom: 0; left: 0; right: 0; height: 24px;
    background: #111; border-top: 1px solid #222;
    display: flex; align-items: center; padding: 0 12px;
    font-size: 0.7rem; color: #666; gap: 16px;
  }
</style>
</head>
<body>
<div id="toolbar">
  <span class="logo">ztransfer</span>
  <span class="sep">|</span>
  <span class="info">LOCAL TEST (this machine)</span>
  <span class="sep">|</span>
  <button class="btn" id="btn-view" onclick="setMode(false)">View</button>
  <button class="btn active" id="btn-control" onclick="setMode(true)">Control</button>
  <span class="sep">|</span>
  <button class="btn" onclick="requestScreenshot()">Refresh</button>
  <span style="margin-left:auto">
    <span class="status-dot loading" id="dot"></span>
    <span id="status">Loading...</span>
  </span>
</div>
<div id="canvas-wrap"><canvas id="screen"></canvas></div>
<div id="statusbar">
  <span id="fps">-- fps</span>
  <span id="latency">-- ms</span>
  <span id="resolution">--</span>
  <span id="transfer">-- KB</span>
  <span style="margin-left:auto" id="mode-label">Control mode</span>
</div>
<script>
const canvas = document.getElementById('screen');
const ctx = canvas.getContext('2d');
let control = true, frameCount = 0, lastFps = Date.now(), totalKB = 0;

function setMode(c) {
  control = c;
  document.getElementById('btn-view').classList.toggle('active', !c);
  document.getElementById('btn-control').classList.toggle('active', c);
  document.getElementById('mode-label').textContent = c ? 'Control mode' : 'View only';
}

async function requestScreenshot() {
  const t0 = Date.now();
  try {
    const res = await fetch('/api/remote/computer/screen?session=local');
    const data = await res.json();
    if (!data.ok) { document.getElementById('status').textContent = data.error; return; }
    const img = new Image();
    img.onload = () => {
      if (canvas.width !== img.width || canvas.height !== img.height) {
        canvas.width = img.width; canvas.height = img.height;
        document.getElementById('resolution').textContent = img.width+'x'+img.height;
      }
      ctx.drawImage(img, 0, 0);
      document.getElementById('dot').className = 'status-dot connected';
      document.getElementById('status').textContent = 'Connected';
      frameCount++;
      document.getElementById('latency').textContent = (Date.now()-t0)+' ms';
      totalKB += (data.data.size||0)/1024;
      document.getElementById('transfer').textContent = totalKB.toFixed(0)+' KB';
    };
    img.src = 'data:image/' + (data.data.format || 'jpeg') + ';base64,' + data.data.base64;
  } catch(e) { document.getElementById('status').textContent = 'Error'; }
}

async function sendAction(a) {
  if (!control) return;
  await fetch('/api/remote/computer/action', {
    method:'POST', headers:{'Content-Type':'application/json'},
    body: JSON.stringify({session:'local', action:a})
  });
  setTimeout(requestScreenshot, 150);
}

function coords(e) {
  const r = canvas.getBoundingClientRect();
  return { x: Math.round((e.clientX-r.left)*(canvas.width/r.width)),
           y: Math.round((e.clientY-r.top)*(canvas.height/r.height)) };
}

canvas.addEventListener('click', e => { const p=coords(e); sendAction({type:'click',x:p.x,y:p.y}); });
canvas.addEventListener('dblclick', e => { e.preventDefault(); const p=coords(e); sendAction({type:'double_click',x:p.x,y:p.y}); });
canvas.addEventListener('contextmenu', e => { e.preventDefault(); const p=coords(e); sendAction({type:'right_click',x:p.x,y:p.y}); });
canvas.addEventListener('wheel', e => {
  e.preventDefault();
  const p=coords(e);
  sendAction({type:'scroll',scroll_x:p.x,scroll_y:p.y,direction:e.deltaY>0?'down':'up',scroll_amount:Math.max(1,Math.abs(Math.round(e.deltaY/100)))});
}, {passive:false});
let drag=null;
canvas.addEventListener('mousedown', e => { if(control&&e.button===0) drag=coords(e); });
canvas.addEventListener('mouseup', e => {
  if(!drag) return;
  const end=coords(e);
  if(Math.abs(end.x-drag.x)>10||Math.abs(end.y-drag.y)>10)
    sendAction({type:'drag',start_x:drag.x,start_y:drag.y,end_x:end.x,end_y:end.y});
  drag=null;
});
document.addEventListener('keydown', e => {
  if(!control) return;
  if(e.metaKey||(e.ctrlKey&&['r','l','t','w','n'].includes(e.key))) return;
  e.preventDefault();
  if(e.key.length===1&&!e.ctrlKey&&!e.altKey) { sendAction({type:'type',text:e.key}); return; }
  let key=e.key;
  const m={'ArrowUp':'Up','ArrowDown':'Down','ArrowLeft':'Left','ArrowRight':'Right',
    'Backspace':'Backspace','Delete':'Delete','Enter':'Return','Tab':'Tab','Escape':'Escape',' ':'Space'};
  if(m[key]) key=m[key];
  if(e.ctrlKey&&key!=='Control') key='ctrl+'+key;
  if(e.altKey&&key!=='Alt') key='alt+'+key;
  sendAction({type:'key',key:key});
});

setInterval(() => {
  const s=(Date.now()-lastFps)/1000;
  document.getElementById('fps').textContent=(frameCount/s).toFixed(1)+' fps';
  frameCount=0; lastFps=Date.now();
}, 2000);

requestScreenshot();
setInterval(requestScreenshot, 500);
</script>
</body>
</html>`
