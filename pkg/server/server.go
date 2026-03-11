package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/quantum-encoding/ztransfer-public/pkg/auth"
	"github.com/quantum-encoding/ztransfer-public/pkg/crypto"
)

// FileInfo represents a file or directory entry.
type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
	Mode    string `json:"mode"`
}

// Server is the ztransfer file server.
type Server struct {
	RootDir   string
	Identity  *auth.Identity
	PeerStore *auth.PeerStore
	PairToken string
	Port      int
}

// authMiddleware verifies the client is a known peer via ML-DSA signature.
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		peerName := r.Header.Get("X-ZTransfer-Peer")
		sigHex := r.Header.Get("X-ZTransfer-Sig")
		nonce := r.Header.Get("X-ZTransfer-Nonce")

		if peerName == "" || sigHex == "" || nonce == "" {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}

		pk, err := s.PeerStore.GetPeerPublicKey(peerName)
		if err != nil {
			http.Error(w, "unknown peer", http.StatusForbidden)
			return
		}

		// Verify ML-DSA signature over: method + path + nonce
		message := []byte(r.Method + r.URL.Path + nonce)
		var sig [crypto.MLDSASignatureSize]byte
		sigBytes, err := hexDecode(sigHex)
		if err != nil || len(sigBytes) != crypto.MLDSASignatureSize {
			http.Error(w, "invalid signature format", http.StatusBadRequest)
			return
		}
		copy(sig[:], sigBytes)

		if !crypto.Verify(pk, message, &sig) {
			http.Error(w, "signature verification failed", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

// Start starts the HTTPS file server.
func (s *Server) Start() error {
	tlsConfig, err := auth.LoadOrCreateTLSConfig()
	if err != nil {
		return fmt.Errorf("TLS setup: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/pair", auth.HandlePair(s.Identity, s.PeerStore, s.PairToken))
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/ls", s.authMiddleware(s.handleList))
	mux.HandleFunc("/api/v1/file", s.authMiddleware(s.handleFile))
	mux.HandleFunc("/api/v1/upload", s.authMiddleware(s.handleUpload))

	addr := fmt.Sprintf(":%d", s.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	server := &http.Server{
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	// Print connection info
	fmt.Printf("\n  ztransfer server running\n")
	fmt.Printf("  %-14s %s\n", "Serving:", s.RootDir)
	fmt.Printf("  %-14s %s\n", "Identity:", s.Identity.Name)
	fmt.Printf("  %-14s %s\n", "Fingerprint:", s.Identity.Fingerprint())
	fmt.Printf("  %-14s %d\n", "Port:", s.Port)
	printLocalAddresses(s.Port)
	fmt.Printf("\n  Pair command:\n")
	fmt.Printf("    ztransfer pair <address>:%d --token %s\n\n", s.Port, s.PairToken)

	return server.ServeTLS(ln, "", "")
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"name":        s.Identity.Name,
		"fingerprint": s.Identity.Fingerprint(),
		"version":     crypto.Version(),
		"root_dir":    s.RootDir,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	reqPath := r.URL.Query().Get("path")
	if reqPath == "" {
		reqPath = "/"
	}

	// Resolve and validate path is within root
	fullPath := filepath.Join(s.RootDir, filepath.Clean(reqPath))
	if !strings.HasPrefix(fullPath, s.RootDir) {
		http.Error(w, "path traversal denied", http.StatusForbidden)
		return
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	files := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		relPath, _ := filepath.Rel(s.RootDir, filepath.Join(fullPath, e.Name()))
		files = append(files, FileInfo{
			Name:    e.Name(),
			Path:    "/" + relPath,
			Size:    info.Size(),
			IsDir:   e.IsDir(),
			ModTime: info.ModTime().Format(time.RFC3339),
			Mode:    info.Mode().String(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return files[i].Name < files[j].Name
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	reqPath := r.URL.Query().Get("path")
	if reqPath == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(s.RootDir, filepath.Clean(reqPath))
	if !strings.HasPrefix(fullPath, s.RootDir) {
		http.Error(w, "path traversal denied", http.StatusForbidden)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if info.IsDir() {
		http.Error(w, "is a directory", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(fullPath)))
	w.Header().Set("X-File-Size", fmt.Sprintf("%d", info.Size()))
	w.Header().Set("X-File-ModTime", info.ModTime().Format(time.RFC3339))

	http.ServeFile(w, r, fullPath)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	destPath := r.URL.Query().Get("path")
	if destPath == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(s.RootDir, filepath.Clean(destPath))
	if !strings.HasPrefix(fullPath, s.RootDir) {
		http.Error(w, "path traversal denied", http.StatusForbidden)
		return
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		http.Error(w, "mkdir failed", http.StatusInternalServerError)
		return
	}

	f, err := os.Create(fullPath)
	if err != nil {
		http.Error(w, "create failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	written, err := io.Copy(f, r.Body)
	if err != nil {
		http.Error(w, "write failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"path": destPath,
		"size": written,
	})
}

func printLocalAddresses(port int) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return
	}
	fmt.Printf("  %-14s", "Listening:")
	first := true
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			if first {
				fmt.Printf("https://%s:%d\n", ipnet.IP, port)
				first = false
			} else {
				fmt.Printf("  %-14s https://%s:%d\n", "", ipnet.IP, port)
			}
		}
	}
	if first {
		fmt.Printf("https://127.0.0.1:%d\n", port)
	}
}

func hexDecode(s string) ([]byte, error) {
	b := make([]byte, len(s)/2)
	for i := 0; i < len(b); i++ {
		var hi, lo byte
		hi = unhex(s[2*i])
		lo = unhex(s[2*i+1])
		if hi == 0xff || lo == 0xff {
			return nil, fmt.Errorf("invalid hex")
		}
		b[i] = hi<<4 | lo
	}
	return b, nil
}

func unhex(c byte) byte {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10
	default:
		return 0xff
	}
}
