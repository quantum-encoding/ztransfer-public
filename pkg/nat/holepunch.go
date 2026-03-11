package nat

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// UDPTunnel represents an established encrypted UDP tunnel between two peers.
// All data sent through the tunnel is authenticated and encrypted with
// AES-256-GCM using the shared secret derived from the warp code.
//
// Unlike the TCP-based Tunnel type, UDPTunnel operates over a hole-punched
// UDP connection and handles its own encryption and nonce management.
type UDPTunnel struct {
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr
	localAddr  *net.UDPAddr
	aead       cipher.AEAD
	nonce      atomic.Uint64 // Monotonic nonce counter to prevent reuse.
	mu         sync.Mutex
	closed     atomic.Bool
}

// PunchConfig configures UDP hole punching parameters.
type PunchConfig struct {
	// LocalPort is the local UDP port to bind. Use 0 for an ephemeral port.
	LocalPort int

	// RemoteIP is the peer's public IP address discovered via STUN.
	RemoteIP string

	// RemotePort is the peer's public UDP port discovered via STUN.
	RemotePort int

	// SharedSecret is a 32-byte key derived from the warp code via WarpCode.DeriveKey().
	SharedSecret [32]byte

	// MaxAttempts is the number of punch packets to send before giving up.
	// Defaults to 50 if zero.
	MaxAttempts int

	// RetryDelay is the interval between punch packets.
	// Defaults to 100ms if zero.
	RetryDelay time.Duration
}

// punchMagic is the 4-byte prefix on hole-punch probe packets.
// Both sides send this to open the NAT mapping; receiving it confirms the hole is open.
var punchMagic = []byte{0x5A, 0x54, 0x50, 0x01} // "ZTP\x01"

// maxUDPPayload is the maximum UDP payload we will read.
const maxUDPPayload = 65535

// Punch performs UDP hole punching and returns an encrypted tunnel.
//
// Both peers must call Punch concurrently with each other's STUN-discovered
// endpoints. The algorithm:
//  1. Bind a local UDP socket.
//  2. Send punch probe packets to the remote peer's public endpoint.
//  3. Concurrently listen for incoming punch probes.
//  4. When a probe is received, the NAT hole is open on both sides.
//  5. Initialize AES-256-GCM with the shared secret and return the tunnel.
func Punch(cfg PunchConfig) (*UDPTunnel, error) {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 50
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 100 * time.Millisecond
	}

	remoteAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", cfg.RemoteIP, cfg.RemotePort))
	if err != nil {
		return nil, fmt.Errorf("hole punch: resolve remote: %w", err)
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: cfg.LocalPort})
	if err != nil {
		return nil, fmt.Errorf("hole punch: listen: %w", err)
	}

	// Initialize AES-256-GCM AEAD cipher.
	block, err := aes.NewCipher(cfg.SharedSecret[:])
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("hole punch: create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("hole punch: create GCM: %w", err)
	}

	// Run the punch loop: send probes and listen for incoming probes concurrently.
	actualRemote, err := punchLoop(conn, remoteAddr, cfg.MaxAttempts, cfg.RetryDelay)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("hole punch: %w", err)
	}

	return &UDPTunnel{
		conn:       conn,
		remoteAddr: actualRemote,
		localAddr:  conn.LocalAddr().(*net.UDPAddr),
		aead:       aead,
	}, nil
}

// punchLoop sends punch probes and waits for one to arrive from the remote peer.
// It returns the actual remote address from which the probe was received, which
// may differ from the expected address if the remote NAT rewrites the source port.
func punchLoop(conn *net.UDPConn, remoteAddr *net.UDPAddr, maxAttempts int, retryDelay time.Duration) (*net.UDPAddr, error) {
	type result struct {
		addr *net.UDPAddr
		err  error
	}
	done := make(chan result, 1)

	// Receiver goroutine: listens for incoming punch probes.
	go func() {
		buf := make([]byte, 128)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				done <- result{err: err}
				return
			}
			// Validate that the received packet is a punch probe.
			if n >= len(punchMagic) && isPunchProbe(buf[:n]) {
				done <- result{addr: addr}
				return
			}
			// Ignore spurious packets and keep listening.
		}
	}()

	// Sender: send punch probes at regular intervals.
	for i := 0; i < maxAttempts; i++ {
		select {
		case r := <-done:
			if r.err != nil {
				return nil, fmt.Errorf("receive probe: %w", r.err)
			}
			// Send one more probe to ensure the other side also receives one,
			// since they may still be waiting.
			conn.WriteToUDP(punchMagic, r.addr)
			return r.addr, nil
		default:
		}

		if _, err := conn.WriteToUDP(punchMagic, remoteAddr); err != nil {
			return nil, fmt.Errorf("send probe %d: %w", i, err)
		}

		time.Sleep(retryDelay)
	}

	// Final check: wait a short time for a late response.
	select {
	case r := <-done:
		if r.err != nil {
			return nil, fmt.Errorf("receive probe: %w", r.err)
		}
		conn.WriteToUDP(punchMagic, r.addr)
		return r.addr, nil
	case <-time.After(2 * time.Second):
		return nil, fmt.Errorf("timed out after %d attempts", maxAttempts)
	}
}

// isPunchProbe checks whether a received packet matches the punch magic bytes.
func isPunchProbe(data []byte) bool {
	if len(data) < len(punchMagic) {
		return false
	}
	for i, b := range punchMagic {
		if data[i] != b {
			return false
		}
	}
	return true
}

// Send encrypts data with AES-256-GCM and sends it through the tunnel.
// Each packet uses a unique nonce derived from a monotonic counter to prevent
// nonce reuse. The nonce counter is prepended to the ciphertext so the receiver
// can reconstruct it.
//
// Wire format: [8 bytes nonce counter] [encrypted payload with GCM tag]
func (t *UDPTunnel) Send(data []byte) error {
	if t.closed.Load() {
		return fmt.Errorf("tunnel: send on closed tunnel")
	}

	// Increment nonce counter atomically.
	counter := t.nonce.Add(1)

	// Build the 12-byte GCM nonce: 4 zero bytes + 8-byte counter.
	// This ensures uniqueness as long as we don't wrap a uint64.
	var nonce [12]byte
	binary.BigEndian.PutUint64(nonce[4:], counter)

	// Encrypt the data. The nonce counter is sent in the clear as a prefix
	// so the receiver can reconstruct the nonce.
	ciphertext := t.aead.Seal(nil, nonce[:], data, nil)

	// Build the wire packet: [8-byte counter][ciphertext].
	packet := make([]byte, 8+len(ciphertext))
	binary.BigEndian.PutUint64(packet[:8], counter)
	copy(packet[8:], ciphertext)

	t.mu.Lock()
	_, err := t.conn.WriteToUDP(packet, t.remoteAddr)
	t.mu.Unlock()
	if err != nil {
		return fmt.Errorf("tunnel: send: %w", err)
	}
	return nil
}

// Receive reads an encrypted packet from the tunnel, decrypts it, and returns
// the plaintext. It blocks until a valid packet arrives or the tunnel is closed.
func (t *UDPTunnel) Receive() ([]byte, error) {
	if t.closed.Load() {
		return nil, fmt.Errorf("tunnel: receive on closed tunnel")
	}

	buf := make([]byte, maxUDPPayload)
	for {
		n, _, err := t.conn.ReadFromUDP(buf)
		if err != nil {
			if t.closed.Load() {
				return nil, fmt.Errorf("tunnel: closed")
			}
			return nil, fmt.Errorf("tunnel: receive: %w", err)
		}

		// Minimum packet: 8-byte counter + GCM overhead (at least 16-byte tag).
		if n < 8+t.aead.Overhead() {
			// Skip malformed packets (could be late punch probes).
			continue
		}

		// Extract nonce counter and reconstruct the 12-byte GCM nonce.
		counter := binary.BigEndian.Uint64(buf[:8])
		var nonce [12]byte
		binary.BigEndian.PutUint64(nonce[4:], counter)

		plaintext, err := t.aead.Open(nil, nonce[:], buf[8:n], nil)
		if err != nil {
			// Authentication failed - likely a stale punch probe or corrupted packet.
			// Skip and wait for the next packet.
			continue
		}

		return plaintext, nil
	}
}

// Close closes the tunnel and releases the underlying UDP socket.
func (t *UDPTunnel) Close() error {
	if t.closed.Swap(true) {
		return nil // Already closed.
	}
	return t.conn.Close()
}

// UDPLocalAddr returns the local UDP address of the tunnel.
func (t *UDPTunnel) UDPLocalAddr() *net.UDPAddr {
	return t.localAddr
}

// UDPRemoteAddr returns the remote UDP address of the tunnel.
func (t *UDPTunnel) UDPRemoteAddr() *net.UDPAddr {
	return t.remoteAddr
}

// newNonce generates a random nonce for one-off use (e.g., during handshake).
func newNonce(size int) ([]byte, error) {
	nonce := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return nonce, nil
}
