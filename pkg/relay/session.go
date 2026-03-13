package relay

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// RelaySession represents an active relay between two peers. Both connections
// carry AES-256-GCM encrypted data — the relay only copies raw bytes.
type RelaySession struct {
	// CodeHash identifies the warp code that paired these two peers.
	CodeHash string

	// peerA and peerB are the two hijacked TCP connections.
	peerA net.Conn
	peerB net.Conn

	// createdAt records when the session was established.
	createdAt time.Time

	// lastActivity is updated atomically every time bytes flow through
	// the relay, used by the cleanup loop to detect stale sessions.
	lastActivity atomic.Value // stores time.Time

	// once ensures Close() is idempotent.
	once sync.Once
}

// NewSession creates a relay session between two peer connections.
func NewSession(codeHash string, a, b net.Conn) *RelaySession {
	s := &RelaySession{
		CodeHash:  codeHash,
		peerA:     a,
		peerB:     b,
		createdAt: time.Now(),
	}
	s.lastActivity.Store(time.Now())
	return s
}

// LastActivity returns the most recent time bytes were relayed.
func (s *RelaySession) LastActivity() time.Time {
	return s.lastActivity.Load().(time.Time)
}

// Forward starts bidirectional byte forwarding between the two peers. It
// blocks until both directions have finished (either peer disconnects or an
// error occurs).
func (s *RelaySession) Forward() {
	var wg sync.WaitGroup
	wg.Add(2)

	// A -> B
	go func() {
		defer wg.Done()
		s.copy(s.peerB, s.peerA)
		// When one direction ends, close both so the other direction unblocks.
		s.Close()
	}()

	// B -> A
	go func() {
		defer wg.Done()
		s.copy(s.peerA, s.peerB)
		s.Close()
	}()

	wg.Wait()
}

// copy transfers bytes from src to dst, updating the last-activity timestamp
// on each successful read. It uses a 32 KiB buffer, which is a good balance
// between syscall overhead and memory usage.
func (s *RelaySession) copy(dst, src net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			s.lastActivity.Store(time.Now())
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return
			}
		}
		if readErr != nil {
			// EOF or connection closed — normal termination.
			if readErr != io.EOF {
				// Unexpected error, but we still just return. The caller
				// will close both connections.
				_ = readErr
			}
			return
		}
	}
}

// Close shuts down both peer connections. It is safe to call multiple times.
func (s *RelaySession) Close() {
	s.once.Do(func() {
		s.peerA.Close()
		s.peerB.Close()
	})
}
