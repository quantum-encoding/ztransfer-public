package nat

import (
	"bufio"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// DefaultRelayURL is the production relay server on GCP Cloud Run.
const DefaultRelayURL = "https://ztransfer-relay-967904281608.europe-west1.run.app"

// RelayConfig holds the configuration for relay fallback.
type RelayConfig struct {
	// URL is the relay server URL. Defaults to DefaultRelayURL.
	URL string

	// AuthToken is the Bearer token for relay authentication.
	// For OAuth, this should be a valid JWT or access token.
	AuthToken string
}

// ConnectViaRelay connects to a peer through the relay server when direct
// hole punching fails. Both peers connect to the relay with the same code
// hash, and the relay pairs them for bidirectional byte forwarding.
//
// Returns a *Tunnel wrapping the relayed TCP connection. The upper-layer
// protocol (remote shell/exec) handles its own encryption via the tunnel's
// Send/Recv framing — the relay only sees opaque bytes.
func ConnectViaRelay(cfg *RelayConfig, code *WarpCode) (*Tunnel, error) {
	if cfg == nil || cfg.URL == "" {
		return nil, fmt.Errorf("relay not configured")
	}

	h := code.Hash()
	codeHash := hex.EncodeToString(h[:])

	// Parse host from URL for the TCP dial.
	relayURL := strings.TrimRight(cfg.URL, "/")
	host := relayURL
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if idx := strings.Index(host, "/"); idx > 0 {
		host = host[:idx]
	}
	if !strings.Contains(host, ":") {
		if strings.HasPrefix(relayURL, "https://") {
			host += ":443"
		} else {
			host += ":80"
		}
	}

	// Dial the relay server. Use TLS for HTTPS endpoints.
	// Force HTTP/1.1 via ALPN — HTTP/2 doesn't support Connection: Upgrade.
	var conn net.Conn
	var err error
	if strings.HasPrefix(relayURL, "https://") {
		tlsHost := strings.Split(host, ":")[0]
		conn, err = tls.DialWithDialer(
			&net.Dialer{Timeout: 15 * time.Second},
			"tcp", host,
			&tls.Config{
				ServerName: tlsHost,
				NextProtos: []string{"http/1.1"},
			},
		)
	} else {
		conn, err = net.DialTimeout("tcp", host, 15*time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("dial relay: %w", err)
	}

	// Send the HTTP upgrade request.
	path := "/relay/" + codeHash
	hostHeader := strings.Split(host, ":")[0]
	req := fmt.Sprintf("POST %s HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: relay\r\nContent-Length: 0\r\n",
		path, hostHeader)
	if cfg.AuthToken != "" {
		req += fmt.Sprintf("Authorization: Bearer %s\r\n", cfg.AuthToken)
	}
	req += "\r\n"

	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send upgrade: %w", err)
	}

	// Read the response. May block up to 60s waiting for the peer to connect.
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read relay response: %w", err)
	}
	resp.Body.Close()
	conn.SetReadDeadline(time.Time{}) // Clear deadline

	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, fmt.Errorf("relay rejected: %d %s", resp.StatusCode, resp.Status)
	}

	// Connection is now a raw TCP stream paired with the other peer.
	// Wrap it in a Tunnel for length-prefixed framing.
	return NewTunnel(conn), nil
}

// HostViaRelay is the host-side equivalent: connects to the relay and waits
// for the peer. The relay pairs the first two connections with the same hash.
func HostViaRelay(cfg *RelayConfig, code *WarpCode) (*Tunnel, error) {
	return ConnectViaRelay(cfg, code)
}
