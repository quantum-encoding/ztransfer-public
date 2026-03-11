//go:build darwin || linux

package remote

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/quantum-encoding/ztransfer-public/pkg/auth"
	"github.com/quantum-encoding/ztransfer-public/pkg/nat"
)

// Session represents an active remote connection between two peers.
type Session struct {
	Tunnel      *nat.Tunnel
	PeerName    string
	WarpCode    string
	ConnectedAt time.Time
	Identity    *auth.Identity
}

// HostSession starts hosting a remote session by listening for an incoming
// connection on the specified port. It generates a warp code that the
// connecting peer uses to establish the tunnel.
func HostSession(identity *auth.Identity, port int) (*Session, error) {
	code, err := nat.GenerateWarpCode()
	if err != nil {
		return nil, fmt.Errorf("host session: generate warp code: %w", err)
	}

	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("host session: listen on %s: %w", addr, err)
	}

	fmt.Printf("Session hosted on port %d\n", port)
	fmt.Printf("Warp code: %s\n", code.String())
	fmt.Printf("Waiting for peer to connect...\n")

	conn, err := listener.Accept()
	if err != nil {
		listener.Close()
		return nil, fmt.Errorf("host session: accept: %w", err)
	}
	listener.Close()

	tunnel := nat.NewTunnel(conn)

	// Exchange identity information with the peer.
	localInfo := peerInfo{
		Name:        identity.Name,
		Fingerprint: identity.Fingerprint(),
	}
	infoData, err := json.Marshal(localInfo)
	if err != nil {
		tunnel.Close()
		return nil, fmt.Errorf("host session: marshal identity: %w", err)
	}
	if err := tunnel.Send(infoData); err != nil {
		tunnel.Close()
		return nil, fmt.Errorf("host session: send identity: %w", err)
	}

	peerData, err := tunnel.Recv()
	if err != nil {
		tunnel.Close()
		return nil, fmt.Errorf("host session: recv peer identity: %w", err)
	}
	var peer peerInfo
	if err := json.Unmarshal(peerData, &peer); err != nil {
		tunnel.Close()
		return nil, fmt.Errorf("host session: parse peer identity: %w", err)
	}

	fmt.Printf("Connected to peer: %s (%s)\n", peer.Name, peer.Fingerprint)

	return &Session{
		Tunnel:      tunnel,
		PeerName:    peer.Name,
		WarpCode:    code.String(),
		ConnectedAt: time.Now(),
		Identity:    identity,
	}, nil
}

// ConnectSession connects to a hosted session using a warp code.
// The hostAddr should be in "host:port" format.
func ConnectSession(identity *auth.Identity, code string, hostAddr string) (*Session, error) {
	warpCode, err := nat.ParseWarpCode(code)
	if err != nil {
		return nil, fmt.Errorf("connect session: %w", err)
	}

	conn, err := net.DialTimeout("tcp", hostAddr, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect session: dial %s: %w", hostAddr, err)
	}

	tunnel := nat.NewTunnel(conn)

	// Receive the host's identity.
	peerData, err := tunnel.Recv()
	if err != nil {
		tunnel.Close()
		return nil, fmt.Errorf("connect session: recv host identity: %w", err)
	}
	var peer peerInfo
	if err := json.Unmarshal(peerData, &peer); err != nil {
		tunnel.Close()
		return nil, fmt.Errorf("connect session: parse host identity: %w", err)
	}

	// Send our identity.
	localInfo := peerInfo{
		Name:        identity.Name,
		Fingerprint: identity.Fingerprint(),
	}
	infoData, err := json.Marshal(localInfo)
	if err != nil {
		tunnel.Close()
		return nil, fmt.Errorf("connect session: marshal identity: %w", err)
	}
	if err := tunnel.Send(infoData); err != nil {
		tunnel.Close()
		return nil, fmt.Errorf("connect session: send identity: %w", err)
	}

	fmt.Printf("Connected to host: %s (%s)\n", peer.Name, peer.Fingerprint)

	return &Session{
		Tunnel:      tunnel,
		PeerName:    peer.Name,
		WarpCode:    warpCode.String(),
		ConnectedAt: time.Now(),
		Identity:    identity,
	}, nil
}

// Shell opens an interactive shell on the remote machine.
// On the host side, this should be preceded by a call to ServeShell.
func (s *Session) Shell() error {
	return ConnectShell(s.Tunnel)
}

// Exec runs a single command on the remote machine and returns the result.
func (s *Session) Exec(command string, args ...string) (*ExecResponse, error) {
	req := ExecRequest{
		Command: command,
		Args:    args,
	}
	return ExecClient(s.Tunnel, req)
}

// Close ends the session and closes the underlying tunnel.
func (s *Session) Close() error {
	if s.Tunnel != nil {
		return s.Tunnel.Close()
	}
	return nil
}

// peerInfo is exchanged during session establishment to identify peers.
type peerInfo struct {
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
}
