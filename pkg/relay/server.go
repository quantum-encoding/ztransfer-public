// Package relay implements a zero-dependency TCP relay server that pairs peers
// behind double CGNAT (e.g., Starlink) when UDP hole punching fails.
//
// Peers connect via HTTP upgrade to /relay/{code_hash}. The server hijacks the
// connection and, once two peers present the same code hash, forwards raw bytes
// bidirectionally. The payload is already AES-256-GCM encrypted by the tunnel
// layer, so the relay never sees plaintext.
package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// RelayServer pairs peers that connect with the same warp code hash and
// forwards raw bytes between them.
type RelayServer struct {
	// ListenAddr is the address to listen on (e.g., ":8080").
	ListenAddr string

	// AuthToken, if non-empty, requires every request to carry a matching
	// Bearer token in the Authorization header. Ignored when OAuth is configured.
	AuthToken string

	// OAuth holds OAuth 2.0 / OIDC configuration. When Issuer is set,
	// JWT validation is used instead of static token auth.
	OAuth *OAuthConfig

	// MaxSessions caps the number of concurrent relay sessions. A session is
	// counted once both peers have connected.
	MaxSessions int

	// SessionTimeout is how long a session (or a waiting peer) can remain
	// idle before being cleaned up.
	SessionTimeout time.Duration

	// mu guards the sessions and waiting maps.
	mu sync.RWMutex

	// waiting holds the first peer for a given code hash while it waits for
	// its counterpart.
	waiting map[string]*pendingPeer

	// sessions holds active relay sessions keyed by code hash.
	sessions map[string]*RelaySession

	// sessionCount is an atomic counter for fast, lock-free reads from the
	// /status endpoint.
	sessionCount atomic.Int64

	// server is the underlying HTTP server, retained for graceful shutdown.
	server *http.Server

	// done is closed when the server has fully stopped.
	done chan struct{}

	logger *log.Logger
}

// pendingPeer represents the first peer waiting for its counterpart.
type pendingPeer struct {
	conn      net.Conn
	createdAt time.Time
}

// New creates a RelayServer with sensible defaults for any zero-value config
// fields.
func New(listenAddr, authToken string, maxSessions int) *RelayServer {
	if listenAddr == "" {
		listenAddr = ":8080"
	}
	if maxSessions <= 0 {
		maxSessions = 100
	}

	return &RelayServer{
		ListenAddr:     listenAddr,
		AuthToken:      authToken,
		MaxSessions:    maxSessions,
		SessionTimeout: 5 * time.Minute,
		waiting:        make(map[string]*pendingPeer),
		sessions:       make(map[string]*RelaySession),
		done:           make(chan struct{}),
		logger:         log.New(os.Stdout, "[relay] ", log.LstdFlags|log.Lmsgprefix),
	}
}

// Start begins listening and serving. It blocks until the server is shut down
// via Shutdown or the context is cancelled.
func (s *RelayServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/relay/", s.handleRelay)

	// Wrap the mux with auth middleware.
	var handler http.Handler = mux
	if s.OAuth != nil && s.OAuth.Issuer != "" {
		// OAuth 2.0 / OIDC JWT validation.
		handler = oauthMiddleware(s.OAuth, mux)
	} else if s.AuthToken != "" {
		// Static Bearer token fallback.
		handler = s.authMiddleware(mux)
	}

	s.server = &http.Server{
		Addr:              s.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start the background cleaner for stale sessions and waiting peers.
	cleanerCtx, cleanerCancel := context.WithCancel(ctx)
	go s.cleanupLoop(cleanerCtx)

	// Shut down gracefully when the context is cancelled.
	go func() {
		<-ctx.Done()
		s.logger.Println("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
		cleanerCancel()
		s.closeAllSessions()
		close(s.done)
	}()

	s.logger.Printf("listening on %s (max_sessions=%d, auth=%v)",
		s.ListenAddr, s.MaxSessions, s.AuthToken != "")

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		cleanerCancel()
		return fmt.Errorf("relay server: %w", err)
	}
	return nil
}

// Wait blocks until the server has fully stopped after a shutdown.
func (s *RelayServer) Wait() {
	<-s.done
}

// --------------------------------------------------------------------------
// HTTP handlers
// --------------------------------------------------------------------------

// handleHealth is a trivial liveness probe.
func (s *RelayServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// handleStatus returns a JSON object with the current number of active
// sessions and waiting peers.
func (s *RelayServer) handleStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	waitingCount := len(s.waiting)
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"active_sessions": s.sessionCount.Load(),
		"waiting_peers":   waitingCount,
		"max_sessions":    s.MaxSessions,
	})
}

// handleRelay is the main endpoint. It expects requests of the form:
//
//	POST /relay/{code_hash}
//	Connection: Upgrade
//	Upgrade: relay
//
// The server hijacks the TCP connection and either parks it (first peer) or
// pairs it with a waiting peer and starts bidirectional forwarding.
func (s *RelayServer) handleRelay(w http.ResponseWriter, r *http.Request) {
	// Extract the code hash from the URL path.
	codeHash := strings.TrimPrefix(r.URL.Path, "/relay/")
	if codeHash == "" {
		http.Error(w, "missing code hash", http.StatusBadRequest)
		return
	}

	// Validate the upgrade request.
	if !headerContains(r.Header, "Connection", "upgrade") ||
		!headerContains(r.Header, "Upgrade", "relay") {
		http.Error(w, "expected Connection: Upgrade and Upgrade: relay headers", http.StatusBadRequest)
		return
	}

	// Hijack the underlying TCP connection.
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "server does not support hijacking", http.StatusInternalServerError)
		return
	}

	conn, buf, err := hj.Hijack()
	if err != nil {
		s.logger.Printf("hijack failed for %s: %v", codeHash, err)
		http.Error(w, "hijack failed", http.StatusInternalServerError)
		return
	}

	// Flush any buffered data from the bufio.ReadWriter — we need raw access.
	// First, send the 101 Switching Protocols response so the client knows
	// the upgrade succeeded.
	resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: relay\r\nConnection: Upgrade\r\n\r\n"
	if _, err := buf.WriteString(resp); err != nil {
		s.logger.Printf("write upgrade response failed for %s: %v", codeHash, err)
		conn.Close()
		return
	}
	if err := buf.Flush(); err != nil {
		s.logger.Printf("flush upgrade response failed for %s: %v", codeHash, err)
		conn.Close()
		return
	}

	s.pairPeer(codeHash, conn)
}

// --------------------------------------------------------------------------
// Peer pairing
// --------------------------------------------------------------------------

// pairPeer either parks the connection as a waiting peer or, if a peer is
// already waiting for the same code hash, creates a relay session.
func (s *RelayServer) pairPeer(codeHash string, conn net.Conn) {
	s.mu.Lock()

	// Check if there is already a waiting peer for this code hash.
	peer, exists := s.waiting[codeHash]
	if exists {
		delete(s.waiting, codeHash)

		// Check the session limit before creating a new one.
		if int(s.sessionCount.Load()) >= s.MaxSessions {
			s.mu.Unlock()
			s.logger.Printf("max sessions reached, rejecting peer for %s", codeHash)
			conn.Close()
			peer.conn.Close()
			return
		}

		session := NewSession(codeHash, peer.conn, conn)
		s.sessions[codeHash] = session
		s.sessionCount.Add(1)
		s.mu.Unlock()

		s.logger.Printf("session started: %s", codeHash)

		// Run forwarding in the background. When it finishes, clean up.
		go func() {
			session.Forward()

			s.mu.Lock()
			delete(s.sessions, codeHash)
			s.mu.Unlock()
			s.sessionCount.Add(-1)

			s.logger.Printf("session ended: %s", codeHash)
		}()
		return
	}

	// No peer waiting — park this connection.
	s.waiting[codeHash] = &pendingPeer{
		conn:      conn,
		createdAt: time.Now(),
	}
	s.mu.Unlock()

	s.logger.Printf("peer waiting: %s", codeHash)
}

// --------------------------------------------------------------------------
// Auth middleware
// --------------------------------------------------------------------------

// authMiddleware rejects requests that do not carry the expected Bearer token.
// The /health endpoint is exempt so load balancers can probe without auth.
func (s *RelayServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health check is always public.
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		expected := "Bearer " + s.AuthToken
		if auth != expected {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// --------------------------------------------------------------------------
// Cleanup
// --------------------------------------------------------------------------

// cleanupLoop periodically removes stale waiting peers and timed-out sessions.
func (s *RelayServer) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupStale()
		}
	}
}

// cleanupStale closes waiting peers and sessions that have exceeded the
// inactivity timeout.
func (s *RelayServer) cleanupStale() {
	now := time.Now()
	timeout := s.SessionTimeout

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clean up waiting peers that have been idle too long.
	for hash, peer := range s.waiting {
		if now.Sub(peer.createdAt) > timeout {
			s.logger.Printf("cleaning up stale waiting peer: %s", hash)
			peer.conn.Close()
			delete(s.waiting, hash)
		}
	}

	// Clean up sessions that have been inactive too long.
	for hash, session := range s.sessions {
		if now.Sub(session.LastActivity()) > timeout {
			s.logger.Printf("cleaning up stale session: %s", hash)
			session.Close()
			delete(s.sessions, hash)
			s.sessionCount.Add(-1)
		}
	}
}

// closeAllSessions tears down every active session and waiting peer. Called
// during graceful shutdown.
func (s *RelayServer) closeAllSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for hash, peer := range s.waiting {
		peer.conn.Close()
		delete(s.waiting, hash)
	}
	for hash, session := range s.sessions {
		session.Close()
		delete(s.sessions, hash)
		s.sessionCount.Add(-1)
	}
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// headerContains checks whether a multi-valued HTTP header contains a
// case-insensitive token.
func headerContains(h http.Header, key, token string) bool {
	for _, v := range h[http.CanonicalHeaderKey(key)] {
		for _, s := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(s), token) {
				return true
			}
		}
	}
	return false
}
