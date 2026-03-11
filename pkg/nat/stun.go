package nat

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

// STUNResult holds the public endpoint discovered via STUN, along with the
// local address of the socket used for discovery.
type STUNResult struct {
	PublicIP   string
	PublicPort int
	LocalIP    string
	LocalPort  int
}

// stunServers is the list of public STUN servers queried in order.
// We try multiple servers to handle transient failures.
var stunServers = []string{
	"stun.l.google.com:19302",
	"stun1.l.google.com:19302",
	"stun.cloudflare.com:3478",
}

// STUN message constants (RFC 5389).
const (
	stunMagicCookie   = 0x2112A442
	stunBindingMethod = 0x0001 // Binding Request
	stunHeaderSize    = 20

	// Attribute types.
	stunAttrMappedAddress    = 0x0001
	stunAttrXORMappedAddress = 0x0020

	// Response class check: top two bits of the method encode the class.
	stunClassSuccess = 0x0100 // Binding Success Response = 0x0101
)

// stunTimeout is the per-server read deadline for a STUN response.
const stunTimeout = 3 * time.Second

// DiscoverPublicEndpoint queries STUN servers to discover our public IP:port
// as seen by the internet. It tries each server in order and returns the first
// successful result.
func DiscoverPublicEndpoint() (*STUNResult, error) {
	return DiscoverPublicEndpointFrom(nil)
}

// DiscoverPublicEndpointFrom queries STUN servers using an existing UDP
// connection. If conn is nil, a new connection is created on an ephemeral port.
// Using an existing connection is important for hole punching, since the
// discovered public endpoint is only valid for the socket that performed the
// STUN query.
func DiscoverPublicEndpointFrom(conn *net.UDPConn) (*STUNResult, error) {
	if conn == nil {
		var err error
		conn, err = net.ListenUDP("udp4", &net.UDPAddr{Port: 0})
		if err != nil {
			return nil, fmt.Errorf("stun: listen UDP: %w", err)
		}
		defer conn.Close()
	}

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// Generate a random 12-byte transaction ID.
	var txID [12]byte
	if _, err := rand.Read(txID[:]); err != nil {
		return nil, fmt.Errorf("stun: generate transaction ID: %w", err)
	}

	// Build a STUN Binding Request (RFC 5389 Section 6).
	// Header: 2 bytes type + 2 bytes length + 4 bytes magic cookie + 12 bytes txID.
	req := buildSTUNBindingRequest(txID)

	var lastErr error
	for _, server := range stunServers {
		result, err := querySTUNServer(conn, localAddr, server, req, txID)
		if err != nil {
			lastErr = err
			continue
		}
		return result, nil
	}

	// conn.Close() is handled by the deferred close above when ownConn is true.
	return nil, fmt.Errorf("stun: all servers failed, last error: %w", lastErr)
}

// buildSTUNBindingRequest constructs a minimal 20-byte STUN Binding Request.
func buildSTUNBindingRequest(txID [12]byte) []byte {
	req := make([]byte, stunHeaderSize)
	// Message Type: Binding Request (0x0001).
	binary.BigEndian.PutUint16(req[0:2], stunBindingMethod)
	// Message Length: 0 (no attributes).
	binary.BigEndian.PutUint16(req[2:4], 0)
	// Magic Cookie.
	binary.BigEndian.PutUint32(req[4:8], stunMagicCookie)
	// Transaction ID.
	copy(req[8:20], txID[:])
	return req
}

// querySTUNServer sends a binding request to a single STUN server and parses
// the XOR-MAPPED-ADDRESS from the response.
func querySTUNServer(conn *net.UDPConn, localAddr *net.UDPAddr, server string, req []byte, txID [12]byte) (*STUNResult, error) {
	serverAddr, err := net.ResolveUDPAddr("udp4", server)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", server, err)
	}

	if _, err := conn.WriteToUDP(req, serverAddr); err != nil {
		return nil, fmt.Errorf("send to %s: %w", server, err)
	}

	// Wait for response with a timeout.
	if err := conn.SetReadDeadline(time.Now().Add(stunTimeout)); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}
	defer conn.SetReadDeadline(time.Time{}) // Clear deadline after read.

	buf := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, fmt.Errorf("read from %s: %w", server, err)
	}

	if n < stunHeaderSize {
		return nil, fmt.Errorf("response from %s too short (%d bytes)", server, n)
	}

	// Validate response header.
	msgType := binary.BigEndian.Uint16(buf[0:2])
	if msgType != (stunBindingMethod | stunClassSuccess) {
		return nil, fmt.Errorf("unexpected STUN message type 0x%04x from %s", msgType, server)
	}

	cookie := binary.BigEndian.Uint32(buf[4:8])
	if cookie != stunMagicCookie {
		return nil, fmt.Errorf("invalid magic cookie from %s", server)
	}

	// Verify transaction ID matches.
	var respTxID [12]byte
	copy(respTxID[:], buf[8:20])
	if respTxID != txID {
		return nil, fmt.Errorf("transaction ID mismatch from %s", server)
	}

	// Parse attributes to find XOR-MAPPED-ADDRESS (preferred) or MAPPED-ADDRESS.
	msgLen := int(binary.BigEndian.Uint16(buf[2:4]))
	if stunHeaderSize+msgLen > n {
		return nil, fmt.Errorf("truncated response from %s", server)
	}

	ip, port, err := parseSTUNAttributes(buf[stunHeaderSize:stunHeaderSize+msgLen])
	if err != nil {
		return nil, fmt.Errorf("parse attributes from %s: %w", server, err)
	}

	return &STUNResult{
		PublicIP:   ip.String(),
		PublicPort: port,
		LocalIP:    localAddr.IP.String(),
		LocalPort:  localAddr.Port,
	}, nil
}

// parseSTUNAttributes walks through STUN attributes looking for
// XOR-MAPPED-ADDRESS or MAPPED-ADDRESS and returns the discovered IP and port.
func parseSTUNAttributes(attrs []byte) (net.IP, int, error) {
	var fallbackIP net.IP
	var fallbackPort int

	offset := 0
	for offset+4 <= len(attrs) {
		attrType := binary.BigEndian.Uint16(attrs[offset : offset+2])
		attrLen := int(binary.BigEndian.Uint16(attrs[offset+2 : offset+4]))
		offset += 4

		if offset+attrLen > len(attrs) {
			break
		}

		value := attrs[offset : offset+attrLen]

		switch attrType {
		case stunAttrXORMappedAddress:
			ip, port, err := parseXORMappedAddress(value)
			if err == nil {
				return ip, port, nil // Preferred attribute, return immediately.
			}
		case stunAttrMappedAddress:
			ip, port, err := parseMappedAddress(value)
			if err == nil {
				fallbackIP = ip
				fallbackPort = port
			}
		}

		// Attributes are padded to 4-byte boundaries (RFC 5389 Section 15).
		padding := (4 - (attrLen % 4)) % 4
		offset += attrLen + padding
	}

	if fallbackIP != nil {
		return fallbackIP, fallbackPort, nil
	}
	return nil, 0, fmt.Errorf("no MAPPED-ADDRESS or XOR-MAPPED-ADDRESS found")
}

// parseXORMappedAddress decodes a STUN XOR-MAPPED-ADDRESS attribute value
// (RFC 5389 Section 15.2). The port and IP are XORed with the magic cookie.
func parseXORMappedAddress(value []byte) (net.IP, int, error) {
	if len(value) < 8 {
		return nil, 0, fmt.Errorf("XOR-MAPPED-ADDRESS too short")
	}

	family := value[1]
	xorPort := binary.BigEndian.Uint16(value[2:4])
	port := int(xorPort ^ uint16(stunMagicCookie>>16))

	switch family {
	case 0x01: // IPv4
		xorIP := binary.BigEndian.Uint32(value[4:8])
		ipVal := xorIP ^ stunMagicCookie
		ip := net.IPv4(byte(ipVal>>24), byte(ipVal>>16), byte(ipVal>>8), byte(ipVal))
		return ip, port, nil
	case 0x02: // IPv6
		// IPv6 XOR-MAPPED-ADDRESS requires XORing with magic cookie + transaction ID.
		// We only support IPv4 for UDP hole punching.
		return nil, 0, fmt.Errorf("IPv6 not supported for NAT traversal")
	default:
		return nil, 0, fmt.Errorf("unknown address family 0x%02x", family)
	}
}

// parseMappedAddress decodes a STUN MAPPED-ADDRESS attribute value
// (RFC 5389 Section 15.1). This is the non-XORed fallback.
func parseMappedAddress(value []byte) (net.IP, int, error) {
	if len(value) < 8 {
		return nil, 0, fmt.Errorf("MAPPED-ADDRESS too short")
	}

	family := value[1]
	port := int(binary.BigEndian.Uint16(value[2:4]))

	switch family {
	case 0x01: // IPv4
		ip := net.IPv4(value[4], value[5], value[6], value[7])
		return ip, port, nil
	default:
		return nil, 0, fmt.Errorf("unsupported address family 0x%02x", family)
	}
}
