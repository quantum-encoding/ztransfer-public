//go:build darwin || linux

// Computer use API endpoints for Claude Code / Anthropic computer use integration.
//
// These endpoints allow an AI agent to control a remote machine through
// ztransfer's encrypted tunnel: capture screenshots, execute mouse/keyboard
// actions, and manage computer use sessions.
//
// Usage:
//
//	# Start a computer use session
//	curl -X POST http://localhost:9877/api/remote/computer/start \
//	  -d '{"code":"warp-429-delta"}'
//
//	# Get a screenshot
//	curl http://localhost:9877/api/remote/computer/screen?session=sess-123
//
//	# Send a click action
//	curl -X POST http://localhost:9877/api/remote/computer/action \
//	  -d '{"session":"sess-123","action":{"type":"click","x":500,"y":300}}'
package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/quantum-encoding/ztransfer/pkg/audit"
	"github.com/quantum-encoding/ztransfer/pkg/remote"
)

// computerSessions tracks active computer use sessions.
var computerSessions = struct {
	sync.RWMutex
	m map[string]*computerSessionEntry
}{m: make(map[string]*computerSessionEntry)}

type computerSessionEntry struct {
	client  *remote.ComputerClient
	session *remote.Session
	audit   *audit.Chain
}

type computerStartRequest struct {
	Code string `json:"code"`
	Host string `json:"host,omitempty"`
}

type computerActionRequest struct {
	Session string               `json:"session"`
	Action  remote.ComputerAction `json:"action"`
}

// RegisterComputerRoutes adds computer use endpoints to the API server.
func (s *Server) RegisterComputerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/remote/computer/start", s.handleComputerStart)
	mux.HandleFunc("/api/remote/computer/screen", s.handleComputerScreen)
	mux.HandleFunc("/api/remote/computer/action", s.handleComputerAction)
	mux.HandleFunc("/api/remote/computer/info", s.handleComputerInfo)
	mux.HandleFunc("/api/remote/computer/stop", s.handleComputerStop)
	mux.HandleFunc("/api/remote/computer/sessions", s.handleComputerSessions)
}

func (s *Server) handleComputerStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, apiResponse{Error: "POST required"})
		return
	}

	var req computerStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, apiResponse{Error: "invalid JSON: " + err.Error()})
		return
	}
	if req.Code == "" {
		writeJSON(w, 400, apiResponse{Error: "code required"})
		return
	}

	session, err := remote.ConnectSession(s.Identity, req.Code, req.Host)
	if err != nil {
		writeJSON(w, 500, apiResponse{Error: "connect failed: " + err.Error()})
		return
	}

	sessionID := generateSessionID()

	// Create audit chain for this computer use session
	auditChain := audit.NewChain(
		sessionID,
		s.Identity.Name,
		session.PeerName,
	)

	client := remote.NewComputerClient(session.Tunnel)

	computerSessions.Lock()
	computerSessions.m[sessionID] = &computerSessionEntry{
		client:  client,
		session: session,
		audit:   auditChain,
	}
	computerSessions.Unlock()

	writeJSON(w, 200, apiResponse{
		OK:      true,
		Message: "Computer use session started",
		Data: map[string]any{
			"session":     sessionID,
			"peer_name":   session.PeerName,
			"screen_info": client.Info,
		},
	})
}

func (s *Server) handleComputerScreen(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		writeJSON(w, 400, apiResponse{Error: "session query parameter required"})
		return
	}

	computerSessions.RLock()
	entry, ok := computerSessions.m[sessionID]
	computerSessions.RUnlock()
	if !ok {
		writeJSON(w, 404, apiResponse{Error: "session not found"})
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "jpeg" // Default to JPEG for bandwidth efficiency
	}

	qualityStr := r.URL.Query().Get("quality")
	quality := 65
	if qualityStr != "" {
		if q, err := strconv.Atoi(qualityStr); err == nil && q > 0 && q <= 100 {
			quality = q
		}
	}

	req := remote.ScreenRequest{Format: format, Quality: quality}
	// For JPEG via tunnel, tell the remote to compress before sending
	if format == "jpeg" {
		req.Scale = 2 // Halve Retina resolution
	}

	data, err := entry.client.Screenshot(req)
	if err != nil {
		writeJSON(w, 500, apiResponse{Error: "screenshot failed: " + err.Error()})
		return
	}

	// Check Accept header — return raw image or base64 JSON
	accept := r.Header.Get("Accept")
	if accept == "image/jpeg" || accept == "image/png" || accept == "image/*" {
		contentType := "image/jpeg"
		if format == "png" {
			contentType = "image/png"
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(200)
		w.Write(data)
		return
	}

	// Default: return base64-encoded in JSON (for AI providers and viewer)
	writeJSON(w, 200, apiResponse{
		OK: true,
		Data: map[string]any{
			"format": format,
			"base64": base64.StdEncoding.EncodeToString(data),
			"size":   len(data),
		},
	})
}

func (s *Server) handleComputerAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, apiResponse{Error: "POST required"})
		return
	}

	var req computerActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, apiResponse{Error: "invalid JSON: " + err.Error()})
		return
	}
	if req.Session == "" {
		writeJSON(w, 400, apiResponse{Error: "session required"})
		return
	}

	computerSessions.RLock()
	entry, ok := computerSessions.m[req.Session]
	computerSessions.RUnlock()
	if !ok {
		writeJSON(w, 404, apiResponse{Error: "session not found"})
		return
	}

	result, err := entry.client.Execute(req.Action)
	if err != nil {
		writeJSON(w, 500, apiResponse{Error: "action failed: " + err.Error()})
		return
	}

	writeJSON(w, 200, apiResponse{
		OK: result.Success,
		Data: map[string]any{
			"success": result.Success,
			"error":   result.Error,
		},
	})
}

func (s *Server) handleComputerInfo(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		writeJSON(w, 400, apiResponse{Error: "session query parameter required"})
		return
	}

	computerSessions.RLock()
	entry, ok := computerSessions.m[sessionID]
	computerSessions.RUnlock()
	if !ok {
		writeJSON(w, 404, apiResponse{Error: "session not found"})
		return
	}

	info, err := entry.client.GetScreenInfo()
	if err != nil {
		writeJSON(w, 500, apiResponse{Error: "get info failed: " + err.Error()})
		return
	}

	writeJSON(w, 200, apiResponse{
		OK:   true,
		Data: info,
	})
}

func (s *Server) handleComputerStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, apiResponse{Error: "POST required"})
		return
	}

	var req struct {
		Session string `json:"session"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, apiResponse{Error: "invalid JSON: " + err.Error()})
		return
	}
	if req.Session == "" {
		writeJSON(w, 400, apiResponse{Error: "session required"})
		return
	}

	computerSessions.Lock()
	entry, ok := computerSessions.m[req.Session]
	if ok {
		delete(computerSessions.m, req.Session)
	}
	computerSessions.Unlock()

	if !ok {
		writeJSON(w, 404, apiResponse{Error: "session not found"})
		return
	}

	entry.client.Close()
	if entry.audit != nil {
		entry.audit.SessionEnd(nil)
		entry.audit.Close()
	}
	entry.session.Close()

	writeJSON(w, 200, apiResponse{
		OK:      true,
		Message: fmt.Sprintf("Computer use session %s stopped", req.Session),
	})
}

func (s *Server) handleComputerSessions(w http.ResponseWriter, r *http.Request) {
	computerSessions.RLock()
	defer computerSessions.RUnlock()

	sessions := make([]map[string]any, 0, len(computerSessions.m))
	for id, entry := range computerSessions.m {
		sessions = append(sessions, map[string]any{
			"session":     id,
			"peer_name":   entry.session.PeerName,
			"screen_info": entry.client.Info,
			"connected":   entry.session.ConnectedAt,
		})
	}

	writeJSON(w, 200, apiResponse{
		OK:   true,
		Data: sessions,
	})
}

func generateSessionID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "cu-" + hex.EncodeToString(b)
}
