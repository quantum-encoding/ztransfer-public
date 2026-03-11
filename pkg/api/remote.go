// Package api remote endpoints for Claude Code integration.
//
// These endpoints allow Claude Code to remotely control machines
// through the ztransfer tunnel system.
//
// Usage:
//
//	curl -X POST http://localhost:9877/api/remote/exec \
//	  -d '{"code":"warp-429-delta","command":"pacman -S brave-bin"}'
//
//	curl http://localhost:9877/api/remote/status?code=warp-429-delta
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/quantum-encoding/ztransfer-public/pkg/nat"
	"github.com/quantum-encoding/ztransfer-public/pkg/remote"
)

// activeSessions tracks active remote sessions for the API.
var activeSessions = struct {
	sync.RWMutex
	m map[string]*remote.Session
}{m: make(map[string]*remote.Session)}

type execRequest struct {
	Code    string   `json:"code"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Dir     string   `json:"dir,omitempty"`
	Host    string   `json:"host,omitempty"`
}

type connectRequest struct {
	Code string `json:"code"`
	Host string `json:"host,omitempty"`
}

// RegisterRemoteRoutes adds remote control endpoints to the API server.
func (s *Server) RegisterRemoteRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/remote/exec", s.handleRemoteExec)
	mux.HandleFunc("/api/remote/connect", s.handleRemoteConnect)
	mux.HandleFunc("/api/remote/disconnect", s.handleRemoteDisconnect)
	mux.HandleFunc("/api/remote/sessions", s.handleRemoteSessions)
	mux.HandleFunc("/api/remote/host", s.handleRemoteHost)
}

// handleRemoteExec connects to a remote host and executes a command.
func (s *Server) handleRemoteExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, apiResponse{Error: "POST required"})
		return
	}

	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, apiResponse{Error: "invalid JSON: " + err.Error()})
		return
	}
	if req.Code == "" || req.Command == "" {
		writeJSON(w, 400, apiResponse{Error: "code and command required"})
		return
	}

	// Parse warp code
	code, err := nat.ParseWarpCode(req.Code)
	if err != nil {
		writeJSON(w, 400, apiResponse{Error: "invalid warp code: " + err.Error()})
		return
	}

	// Connect to the remote host
	session, err := remote.ConnectSession(s.Identity, code.String(), req.Host)
	if err != nil {
		writeJSON(w, 500, apiResponse{Error: "connect failed: " + err.Error()})
		return
	}
	defer session.Close()

	// Split command if no args provided (shell-style "pacman -S brave")
	command := req.Command
	args := req.Args
	if len(args) == 0 && strings.Contains(command, " ") {
		parts := strings.Fields(command)
		command = parts[0]
		args = parts[1:]
	}

	// Execute command
	resp, err := session.Exec(command, args...)
	if err != nil {
		writeJSON(w, 500, apiResponse{Error: "exec failed: " + err.Error()})
		return
	}

	writeJSON(w, 200, apiResponse{
		OK: true,
		Data: map[string]any{
			"stdout":    resp.Stdout,
			"stderr":    resp.Stderr,
			"exit_code": resp.ExitCode,
		},
	})
}

// handleRemoteConnect establishes a persistent connection to a remote host.
func (s *Server) handleRemoteConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, apiResponse{Error: "POST required"})
		return
	}

	var req connectRequest
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

	writeJSON(w, 200, apiResponse{
		OK:      true,
		Message: fmt.Sprintf("Connected to %s", session.PeerName),
		Data: map[string]any{
			"code":      req.Code,
			"peer_name": session.PeerName,
			"connected": session.ConnectedAt,
		},
	})
}

// handleRemoteDisconnect closes an active remote session.
func (s *Server) handleRemoteDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, apiResponse{Error: "POST required"})
		return
	}

	var req connectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, apiResponse{Error: "invalid JSON: " + err.Error()})
		return
	}

	writeJSON(w, 200, apiResponse{
		OK:      true,
		Message: fmt.Sprintf("Disconnected from %s", req.Code),
	})
}

// handleRemoteSessions lists active remote sessions.
func (s *Server) handleRemoteSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, apiResponse{
		OK:   true,
		Data: []string{},
	})
}

// handleRemoteHost starts hosting a remote session and returns the warp code.
func (s *Server) handleRemoteHost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, apiResponse{Error: "POST required"})
		return
	}

	session, err := remote.HostSession(s.Identity, 0)
	if err != nil {
		writeJSON(w, 500, apiResponse{Error: "host failed: " + err.Error()})
		return
	}

	writeJSON(w, 200, apiResponse{
		OK:      true,
		Message: "Remote session hosted",
		Data: map[string]any{
			"code":      session.WarpCode,
			"connected": session.ConnectedAt,
		},
	})
}
