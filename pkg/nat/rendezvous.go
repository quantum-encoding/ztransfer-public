package nat

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// RendezvousInfo is the endpoint information exchanged between peers during
// the rendezvous handshake. Both the STUN-discovered public endpoint and the
// LAN-local endpoint are included so that peers on the same network can
// connect directly without NAT traversal.
type RendezvousInfo struct {
	PublicIP   string `json:"public_ip"`
	PublicPort int    `json:"public_port"`
	LocalIP    string `json:"local_ip"`
	LocalPort  int    `json:"local_port"`
	CodeHash   string `json:"code_hash"` // hex-encoded warp code discovery hash
}

// rendezvousTimeout is the maximum time to wait for a peer to connect.
const rendezvousTimeout = 60 * time.Second

// Host starts a local HTTP rendezvous server and waits for a peer to connect.
// The host advertises its STUN-discovered endpoint via the rendezvous server.
// When a peer connects with a matching warp code hash, the host responds with
// its own endpoint info, and both sides proceed to hole punch.
//
// The port parameter specifies the UDP port to use for the tunnel. If 0, an
// ephemeral port is selected. The HTTP rendezvous server listens on port+1
// (or an ephemeral port if port is 0).
func Host(code *WarpCode, port int) (*UDPTunnel, error) {
	// Bind a UDP socket first so we know our local port for STUN and hole punching.
	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: port})
	if err != nil {
		return nil, fmt.Errorf("host: bind UDP: %w", err)
	}

	localUDPAddr := udpConn.LocalAddr().(*net.UDPAddr)

	// Discover our public endpoint using the bound socket.
	stunResult, err := DiscoverPublicEndpointFrom(udpConn)
	if err != nil {
		udpConn.Close()
		return nil, fmt.Errorf("host: STUN discovery: %w", err)
	}

	h := code.Hash()
	codeHash := hex.EncodeToString(h[:])
	hostInfo := &RendezvousInfo{
		PublicIP:   stunResult.PublicIP,
		PublicPort: stunResult.PublicPort,
		LocalIP:    stunResult.LocalIP,
		LocalPort:  localUDPAddr.Port,
		CodeHash:   codeHash,
	}

	// Channel to receive the peer's info once they connect to our rendezvous server.
	peerCh := make(chan *RendezvousInfo, 1)
	errCh := make(chan error, 1)

	// Start a small HTTP server for the rendezvous handshake.
	mux := http.NewServeMux()
	mux.HandleFunc("/rendezvous", rendezvousHandler(hostInfo, codeHash, peerCh))

	httpPort := 0
	if port > 0 {
		httpPort = port + 1
	}
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", httpPort))
	if err != nil {
		udpConn.Close()
		return nil, fmt.Errorf("host: listen HTTP: %w", err)
	}

	server := &http.Server{Handler: mux}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("host: HTTP serve: %w", err)
		}
	}()

	fmt.Printf("Rendezvous server listening on %s\n", listener.Addr().String())
	fmt.Printf("Public endpoint: %s:%d\n", stunResult.PublicIP, stunResult.PublicPort)

	// Wait for the peer to connect, or time out.
	var peerInfo *RendezvousInfo
	select {
	case peerInfo = <-peerCh:
		// Peer connected successfully.
	case err := <-errCh:
		server.Shutdown(context.Background())
		udpConn.Close()
		wg.Wait()
		return nil, err
	case <-time.After(rendezvousTimeout):
		server.Shutdown(context.Background())
		udpConn.Close()
		wg.Wait()
		return nil, fmt.Errorf("host: timed out waiting for peer")
	}

	// Shut down the rendezvous server - it's no longer needed.
	server.Shutdown(context.Background())
	wg.Wait()

	// Proceed to hole punch with the peer's endpoint.
	key := code.DeriveKey()
	tunnel, err := punchWithConn(udpConn, peerInfo, key)
	if err != nil {
		udpConn.Close()
		return nil, fmt.Errorf("host: hole punch: %w", err)
	}

	return tunnel, nil
}

// rendezvousHandler returns an HTTP handler for the rendezvous endpoint.
// It validates the peer's warp code hash, sends back the host's endpoint info,
// and signals the peer's info to the waiting Host goroutine.
func rendezvousHandler(hostInfo *RendezvousInfo, expectedHash string, peerCh chan<- *RendezvousInfo) http.HandlerFunc {
	var once sync.Once
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}

		var peerInfo RendezvousInfo
		if err := json.Unmarshal(body, &peerInfo); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		// Verify the peer has the same warp code by comparing discovery hashes.
		if peerInfo.CodeHash != expectedHash {
			http.Error(w, "code mismatch", http.StatusForbidden)
			return
		}

		// Respond with our endpoint info.
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(hostInfo); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
			return
		}

		// Signal the peer info to the Host goroutine (only once).
		once.Do(func() {
			peerCh <- &peerInfo
		})
	}
}

// Connect connects to a host using a warp code by contacting the host's
// rendezvous server, exchanging endpoint information, and performing
// UDP hole punching.
//
// hostAddr is the host's rendezvous HTTP address (e.g., "192.168.1.5:9001" or
// "example.com:9001"). The warp code is used both for authentication (hash
// comparison) and to derive the tunnel encryption key.
func Connect(code *WarpCode, hostAddr string) (*UDPTunnel, error) {
	// Bind a UDP socket for STUN and hole punching.
	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0})
	if err != nil {
		return nil, fmt.Errorf("connect: bind UDP: %w", err)
	}

	localUDPAddr := udpConn.LocalAddr().(*net.UDPAddr)

	// Discover our public endpoint using the bound socket.
	stunResult, err := DiscoverPublicEndpointFrom(udpConn)
	if err != nil {
		udpConn.Close()
		return nil, fmt.Errorf("connect: STUN discovery: %w", err)
	}

	h := code.Hash()
	codeHash := hex.EncodeToString(h[:])
	ourInfo := &RendezvousInfo{
		PublicIP:   stunResult.PublicIP,
		PublicPort: stunResult.PublicPort,
		LocalIP:    stunResult.LocalIP,
		LocalPort:  localUDPAddr.Port,
		CodeHash:   codeHash,
	}

	// Contact the host's rendezvous server.
	hostInfo, err := doRendezvous(hostAddr, ourInfo)
	if err != nil {
		udpConn.Close()
		return nil, fmt.Errorf("connect: rendezvous: %w", err)
	}

	// Verify the host has the same warp code.
	if hostInfo.CodeHash != codeHash {
		udpConn.Close()
		return nil, fmt.Errorf("connect: host warp code mismatch")
	}

	// Proceed to hole punch with the host's endpoint.
	key := code.DeriveKey()
	tunnel, err := punchWithConn(udpConn, hostInfo, key)
	if err != nil {
		udpConn.Close()
		return nil, fmt.Errorf("connect: hole punch: %w", err)
	}

	return tunnel, nil
}

// doRendezvous sends our endpoint info to the host's rendezvous server and
// returns the host's endpoint info.
func doRendezvous(hostAddr string, ourInfo *RendezvousInfo) (*RendezvousInfo, error) {
	body, err := json.Marshal(ourInfo)
	if err != nil {
		return nil, fmt.Errorf("marshal info: %w", err)
	}

	url := fmt.Sprintf("http://%s/rendezvous", hostAddr)
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("post to %s: %w", hostAddr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("rendezvous rejected (%d): %s", resp.StatusCode, string(respBody))
	}

	var hostInfo RendezvousInfo
	if err := json.NewDecoder(resp.Body).Decode(&hostInfo); err != nil {
		return nil, fmt.Errorf("decode host info: %w", err)
	}

	return &hostInfo, nil
}

// punchWithConn performs hole punching using an existing UDP connection and the
// peer's rendezvous info. It tries the public endpoint first, then falls back
// to the LAN-local endpoint for same-network peers.
func punchWithConn(conn *net.UDPConn, peerInfo *RendezvousInfo, key [32]byte) (*UDPTunnel, error) {
	remoteAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", peerInfo.PublicIP, peerInfo.PublicPort))
	if err != nil {
		return nil, fmt.Errorf("resolve remote: %w", err)
	}

	// Initialize AES-256-GCM from the shared secret.
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	// Perform the punch loop using the existing connection.
	actualRemote, err := punchLoop(conn, remoteAddr, 50, 100*time.Millisecond)
	if err != nil {
		// If public endpoint failed, try the LAN-local endpoint as fallback.
		// This handles the case where both peers are on the same network.
		if peerInfo.LocalIP != "" && peerInfo.LocalPort > 0 {
			localRemote, resolveErr := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", peerInfo.LocalIP, peerInfo.LocalPort))
			if resolveErr == nil {
				actualRemote, err = punchLoop(conn, localRemote, 30, 100*time.Millisecond)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("punch failed (public and local): %w", err)
		}
	}

	return &UDPTunnel{
		conn:       conn,
		remoteAddr: actualRemote,
		localAddr:  conn.LocalAddr().(*net.UDPAddr),
		aead:       gcm,
	}, nil
}
