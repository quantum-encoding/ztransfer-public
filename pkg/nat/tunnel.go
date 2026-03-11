// Package nat implements NAT traversal and encrypted tunneling for ztransfer.
package nat

import (
	"fmt"
	"io"
	"net"
	"sync"
)

// Tunnel represents an encrypted bidirectional communication channel between
// two peers established via NAT traversal. It wraps an underlying net.Conn
// with framed message reading and writing.
type Tunnel struct {
	conn   net.Conn
	mu     sync.Mutex // protects writes
	closed chan struct{}
}

// NewTunnel wraps an existing connection into a Tunnel.
func NewTunnel(conn net.Conn) *Tunnel {
	return &Tunnel{
		conn:   conn,
		closed: make(chan struct{}),
	}
}

// Send writes a length-prefixed frame to the tunnel.
// Frame format: [length_u32_be][payload]
func (t *Tunnel) Send(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	length := uint32(len(data))
	header := []byte{
		byte(length >> 24),
		byte(length >> 16),
		byte(length >> 8),
		byte(length),
	}

	if _, err := t.conn.Write(header); err != nil {
		return fmt.Errorf("tunnel send header: %w", err)
	}
	if _, err := t.conn.Write(data); err != nil {
		return fmt.Errorf("tunnel send payload: %w", err)
	}
	return nil
}

// Recv reads a length-prefixed frame from the tunnel.
func (t *Tunnel) Recv() ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(t.conn, header); err != nil {
		return nil, fmt.Errorf("tunnel recv header: %w", err)
	}

	length := uint32(header[0])<<24 | uint32(header[1])<<16 |
		uint32(header[2])<<8 | uint32(header[3])

	if length > 16*1024*1024 { // 16 MiB max frame
		return nil, fmt.Errorf("tunnel recv: frame too large (%d bytes)", length)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(t.conn, payload); err != nil {
		return nil, fmt.Errorf("tunnel recv payload: %w", err)
	}
	return payload, nil
}

// Close shuts down the tunnel.
func (t *Tunnel) Close() error {
	select {
	case <-t.closed:
		return nil
	default:
		close(t.closed)
	}
	return t.conn.Close()
}

// Done returns a channel that is closed when the tunnel is shut down.
func (t *Tunnel) Done() <-chan struct{} {
	return t.closed
}

// LocalAddr returns the local network address.
func (t *Tunnel) LocalAddr() net.Addr {
	return t.conn.LocalAddr()
}

// RemoteAddr returns the remote network address.
func (t *Tunnel) RemoteAddr() net.Addr {
	return t.conn.RemoteAddr()
}
