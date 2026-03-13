// Package api provides a local REST API for programmatic access to ztransfer.
//
// This allows Claude Code (or any CLI tool) to transfer files between machines
// using simple curl commands instead of the interactive CLI.
//
// Usage:
//
//	ztransfer api                          # Start API on localhost:9877
//	ztransfer api --port 9877              # Custom port
//
// Then from Claude Code on either machine:
//
//	# List files on a paired peer
//	curl http://localhost:9877/api/peers
//	curl http://localhost:9877/api/ls?peer=linux-box&path=/
//
//	# Download a file from remote peer to local path
//	curl -X POST http://localhost:9877/api/get \
//	  -d '{"peer":"linux-box","remote_path":"/data.tar.gz","local_path":"/tmp/"}'
//
//	# Upload a local file to remote peer
//	curl -X POST http://localhost:9877/api/put \
//	  -d '{"peer":"linux-box","local_path":"/tmp/file.txt","remote_path":"/inbox/"}'
//
//	# Send a file directly (pipe content)
//	curl -X POST http://localhost:9877/api/send \
//	  -F "file=@/tmp/file.txt" -F "peer=linux-box" -F "remote_path=/inbox/"
//
//	# Receive a file (returns raw content)
//	curl http://localhost:9877/api/receive?peer=linux-box&path=/file.txt > file.txt
//
//	# Check status
//	curl http://localhost:9877/api/status
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/quantum-encoding/ztransfer/pkg/auth"
	"github.com/quantum-encoding/ztransfer/pkg/client"
)

// Server is the local API server for programmatic access.
type Server struct {
	Identity    *auth.Identity
	PeerStore   *auth.PeerStore
	Client      *client.Client
	DownloadDir string
	Port        int
}

type apiResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Data    any    `json:"data,omitempty"`
}

type transferRequest struct {
	Peer       string `json:"peer"`
	RemotePath string `json:"remote_path"`
	LocalPath  string `json:"local_path"`
}

func writeJSON(w http.ResponseWriter, status int, resp apiResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// Start starts the local API server on localhost only.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/peers", s.handlePeers)
	mux.HandleFunc("/api/ls", s.handleList)
	mux.HandleFunc("/api/get", s.handleGet)
	mux.HandleFunc("/api/put", s.handlePut)
	mux.HandleFunc("/api/send", s.handleSend)
	mux.HandleFunc("/api/receive", s.handleReceive)
	mux.HandleFunc("/api/help", s.handleHelp)

	// Remote access endpoints
	s.RegisterRemoteRoutes(mux)

	// Computer use endpoints (screen capture, input injection)
	s.RegisterComputerRoutes(mux)

	// Web-based remote desktop viewer
	s.RegisterViewerRoutes(mux)

	// Health check
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, apiResponse{
			OK:      true,
			Message: "ztransfer API",
			Data: map[string]string{
				"version":  "0.1.0",
				"docs":     "GET /api/help",
				"identity": s.Identity.Name,
			},
		})
	})

	addr := fmt.Sprintf("127.0.0.1:%d", s.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 10 * time.Minute,
	}

	fmt.Printf("\n  ztransfer API running\n")
	fmt.Printf("  %-14s http://%s\n", "Endpoint:", addr)
	fmt.Printf("  %-14s %s\n", "Identity:", s.Identity.Name)
	fmt.Printf("  %-14s %s\n", "Fingerprint:", s.Identity.Fingerprint())
	fmt.Printf("  %-14s %d paired\n", "Peers:", len(s.PeerStore.ListPeers()))
	fmt.Printf("\n  Claude Code usage:\n")
	fmt.Printf("    curl http://%s/api/peers\n", addr)
	fmt.Printf("    curl http://%s/api/ls?peer=NAME&path=/\n", addr)
	fmt.Printf("    curl -X POST http://%s/api/get -d '{\"peer\":\"NAME\",\"remote_path\":\"/file.txt\",\"local_path\":\"/tmp/\"}'\n", addr)
	fmt.Printf("    curl -X POST http://%s/api/put -d '{\"peer\":\"NAME\",\"local_path\":\"/tmp/file.txt\",\"remote_path\":\"/\"}'\n", addr)
	fmt.Printf("    curl 'http://%s/api/receive?peer=NAME&path=/file.txt' > file.txt\n", addr)
	fmt.Printf("    curl -X POST http://%s/api/send -F file=@/tmp/file.txt -F peer=NAME -F remote_path=/\n", addr)
	fmt.Println()

	return srv.ListenAndServe()
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	peers := s.PeerStore.ListPeers()
	peerNames := make([]string, len(peers))
	for i, p := range peers {
		peerNames[i] = p.Name
	}
	writeJSON(w, 200, apiResponse{
		OK: true,
		Data: map[string]any{
			"identity":    s.Identity.Name,
			"fingerprint": s.Identity.Fingerprint(),
			"peers":       peerNames,
			"peer_count":  len(peers),
			"download_dir": s.DownloadDir,
		},
	})
}

func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	peers := s.PeerStore.ListPeers()
	type peerInfo struct {
		Name        string `json:"name"`
		Address     string `json:"address"`
		Fingerprint string `json:"fingerprint"`
		PairedAt    string `json:"paired_at"`
	}
	result := make([]peerInfo, len(peers))
	for i, p := range peers {
		result[i] = peerInfo{
			Name:        p.Name,
			Address:     p.Address,
			Fingerprint: p.Fingerprint,
			PairedAt:    p.PairedAt.Format(time.RFC3339),
		}
	}
	writeJSON(w, 200, apiResponse{OK: true, Data: result})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	peer := r.URL.Query().Get("peer")
	path := r.URL.Query().Get("path")
	if peer == "" {
		writeJSON(w, 400, apiResponse{Error: "peer parameter required"})
		return
	}
	if path == "" {
		path = "/"
	}
	files, err := s.Client.List(peer, path)
	if err != nil {
		writeJSON(w, 500, apiResponse{Error: err.Error()})
		return
	}
	writeJSON(w, 200, apiResponse{OK: true, Data: files})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, apiResponse{Error: "POST required"})
		return
	}

	var req transferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, apiResponse{Error: "invalid JSON: " + err.Error()})
		return
	}
	if req.Peer == "" || req.RemotePath == "" {
		writeJSON(w, 400, apiResponse{Error: "peer and remote_path required"})
		return
	}
	localDir := req.LocalPath
	if localDir == "" {
		localDir = s.DownloadDir
	}

	written, err := s.Client.Download(req.Peer, req.RemotePath, localDir)
	if err != nil {
		writeJSON(w, 500, apiResponse{Error: err.Error()})
		return
	}

	filename := filepath.Base(req.RemotePath)
	writeJSON(w, 200, apiResponse{
		OK:      true,
		Message: fmt.Sprintf("Downloaded %s (%d bytes)", filename, written),
		Data: map[string]any{
			"file":       filename,
			"bytes":      written,
			"local_path": filepath.Join(localDir, filename),
		},
	})
}

func (s *Server) handlePut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, apiResponse{Error: "POST required"})
		return
	}

	var req transferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, apiResponse{Error: "invalid JSON: " + err.Error()})
		return
	}
	if req.Peer == "" || req.LocalPath == "" {
		writeJSON(w, 400, apiResponse{Error: "peer and local_path required"})
		return
	}
	remotePath := req.RemotePath
	if remotePath == "" {
		remotePath = "/" + filepath.Base(req.LocalPath)
	}

	written, err := s.Client.Upload(req.Peer, req.LocalPath, remotePath)
	if err != nil {
		writeJSON(w, 500, apiResponse{Error: err.Error()})
		return
	}

	writeJSON(w, 200, apiResponse{
		OK:      true,
		Message: fmt.Sprintf("Uploaded %s (%d bytes)", filepath.Base(req.LocalPath), written),
		Data: map[string]any{
			"file":        filepath.Base(req.LocalPath),
			"bytes":       written,
			"remote_path": remotePath,
		},
	})
}

// handleSend accepts multipart file upload and sends to remote peer.
// Usage: curl -X POST /api/send -F file=@/path/file.txt -F peer=name -F remote_path=/dest/
func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, apiResponse{Error: "POST required"})
		return
	}

	r.ParseMultipartForm(256 << 20) // 256 MB max

	peer := r.FormValue("peer")
	remotePath := r.FormValue("remote_path")
	if peer == "" {
		writeJSON(w, 400, apiResponse{Error: "peer field required"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, apiResponse{Error: "file field required: " + err.Error()})
		return
	}
	defer file.Close()

	// Save to temp file, then upload
	tmpFile, err := os.CreateTemp("", "ztransfer-send-*-"+header.Filename)
	if err != nil {
		writeJSON(w, 500, apiResponse{Error: "temp file: " + err.Error()})
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, file); err != nil {
		writeJSON(w, 500, apiResponse{Error: "write temp: " + err.Error()})
		return
	}
	tmpFile.Close()

	if remotePath == "" || remotePath[len(remotePath)-1] == '/' {
		remotePath += header.Filename
	}

	written, err := s.Client.Upload(peer, tmpFile.Name(), remotePath)
	if err != nil {
		writeJSON(w, 500, apiResponse{Error: err.Error()})
		return
	}

	writeJSON(w, 200, apiResponse{
		OK:      true,
		Message: fmt.Sprintf("Sent %s to %s:%s (%d bytes)", header.Filename, peer, remotePath, written),
		Data: map[string]any{
			"file":        header.Filename,
			"bytes":       written,
			"peer":        peer,
			"remote_path": remotePath,
		},
	})
}

// handleReceive streams a remote file directly back as the HTTP response.
// Usage: curl 'http://localhost:9877/api/receive?peer=name&path=/file.txt' > file.txt
func (s *Server) handleReceive(w http.ResponseWriter, r *http.Request) {
	peer := r.URL.Query().Get("peer")
	path := r.URL.Query().Get("path")
	if peer == "" || path == "" {
		writeJSON(w, 400, apiResponse{Error: "peer and path parameters required"})
		return
	}

	// Download to temp, then stream back
	tmpDir, err := os.MkdirTemp("", "ztransfer-recv-*")
	if err != nil {
		writeJSON(w, 500, apiResponse{Error: "temp dir: " + err.Error()})
		return
	}
	defer os.RemoveAll(tmpDir)

	_, err = s.Client.Download(peer, path, tmpDir)
	if err != nil {
		writeJSON(w, 500, apiResponse{Error: err.Error()})
		return
	}

	filename := filepath.Base(path)
	localPath := filepath.Join(tmpDir, filename)

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, localPath)
}

func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	help := map[string]any{
		"description": "ztransfer local API — programmatic file transfer for Claude Code",
		"endpoints": map[string]string{
			"GET  /api/status":  "Server status, identity, and peer list",
			"GET  /api/peers":   "List all paired peers with addresses and fingerprints",
			"GET  /api/ls":      "List remote files. Params: peer, path (default: /)",
			"POST /api/get":     "Download remote file. Body: {peer, remote_path, local_path?}",
			"POST /api/put":     "Upload local file. Body: {peer, local_path, remote_path?}",
			"POST /api/send":    "Send file via multipart. Fields: file, peer, remote_path",
			"GET  /api/receive": "Stream remote file content. Params: peer, path",
			"GET  /api/help":    "This help message",
		},
		"examples": map[string]string{
			"list_peers":    "curl http://localhost:9877/api/peers",
			"list_files":    "curl 'http://localhost:9877/api/ls?peer=linux-box&path=/'",
			"download":      "curl -X POST http://localhost:9877/api/get -d '{\"peer\":\"linux-box\",\"remote_path\":\"/data.csv\",\"local_path\":\"/tmp/\"}'",
			"upload":        "curl -X POST http://localhost:9877/api/put -d '{\"peer\":\"linux-box\",\"local_path\":\"/tmp/report.pdf\",\"remote_path\":\"/inbox/\"}'",
			"pipe_download": "curl 'http://localhost:9877/api/receive?peer=linux-box&path=/data.csv' > data.csv",
			"pipe_upload":   "curl -X POST http://localhost:9877/api/send -F file=@data.csv -F peer=linux-box -F remote_path=/inbox/",
		},
	}
	writeJSON(w, 200, apiResponse{OK: true, Data: help})
}
