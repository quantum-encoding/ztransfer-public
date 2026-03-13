package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/quantum-encoding/ztransfer/pkg/crypto"
)

const tokenLength = 6

// GeneratePairToken creates a short one-time pairing token.
func GeneratePairToken() (string, error) {
	b := make([]byte, tokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// Uppercase hex, easy to read aloud
	return strings.ToUpper(hex.EncodeToString(b)[:tokenLength]), nil
}

// PairRequest is sent by the client to initiate pairing.
type PairRequest struct {
	Token     string `json:"token"`
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
}

// PairResponse is sent by the server after successful pairing.
type PairResponse struct {
	Name        string `json:"name"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
}

// HandlePair handles the server side of the pairing handshake.
// The server verifies the token then exchanges ML-DSA public keys.
func HandlePair(identity *Identity, peerStore *PeerStore, validToken string) http.HandlerFunc {
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

		var req PairRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Token != validToken {
			http.Error(w, "invalid pairing token", http.StatusForbidden)
			return
		}

		// Decode client's public key
		pkBytes, err := hex.DecodeString(req.PublicKey)
		if err != nil || len(pkBytes) != crypto.MLDSAPublicKeySize {
			http.Error(w, "invalid public key", http.StatusBadRequest)
			return
		}
		var clientPK [crypto.MLDSAPublicKeySize]byte
		copy(clientPK[:], pkBytes)

		// Store the client as a trusted peer
		addr := r.RemoteAddr
		if idx := strings.LastIndex(addr, ":"); idx >= 0 {
			addr = addr[:idx]
		}
		if err := peerStore.AddPeer(req.Name, addr, &clientPK); err != nil {
			http.Error(w, "failed to store peer", http.StatusInternalServerError)
			return
		}

		// Send back our public key
		resp := PairResponse{
			Name:        identity.Name,
			PublicKey:   hex.EncodeToString(identity.PublicKey[:]),
			Fingerprint: identity.Fingerprint(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)

		fmt.Printf("  Paired with %s (fingerprint: %s)\n", req.Name, crypto.Fingerprint(&clientPK))
	}
}

// RequestPair performs the client side of the pairing handshake.
func RequestPair(serverAddr, token string, identity *Identity, peerStore *PeerStore) error {
	req := PairRequest{
		Token:     token,
		Name:      identity.Name,
		PublicKey: hex.EncodeToString(identity.PublicKey[:]),
	}

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://%s/api/v1/pair", serverAddr)

	client := insecureTLSClient()
	resp, err := client.Post(url, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("pair request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pairing rejected (%d): %s", resp.StatusCode, string(respBody))
	}

	var pairResp PairResponse
	if err := json.NewDecoder(resp.Body).Decode(&pairResp); err != nil {
		return fmt.Errorf("invalid pair response: %w", err)
	}

	pkBytes, err := hex.DecodeString(pairResp.PublicKey)
	if err != nil || len(pkBytes) != crypto.MLDSAPublicKeySize {
		return fmt.Errorf("invalid server public key")
	}
	var serverPK [crypto.MLDSAPublicKeySize]byte
	copy(serverPK[:], pkBytes)

	if err := peerStore.AddPeer(pairResp.Name, serverAddr, &serverPK); err != nil {
		return fmt.Errorf("store peer: %w", err)
	}

	fmt.Printf("  Paired with %s (fingerprint: %s)\n", pairResp.Name, pairResp.Fingerprint)
	return nil
}
