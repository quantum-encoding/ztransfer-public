package auth

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/quantum-encoding/ztransfer-public/pkg/crypto"
)

// Peer represents a trusted remote machine.
type Peer struct {
	Name        string    `json:"name"`
	Address     string    `json:"address"`
	PublicKey   string    `json:"public_key"`
	Fingerprint string   `json:"fingerprint"`
	PairedAt    time.Time `json:"paired_at"`
}

// PeerStore manages known trusted peers.
type PeerStore struct {
	mu    sync.RWMutex
	peers map[string]*Peer // keyed by name
	path  string
}

// LoadPeerStore loads the peer store from disk.
func LoadPeerStore() (*PeerStore, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "known_peers.json")
	ps := &PeerStore{peers: make(map[string]*Peer), path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		return ps, nil // empty store is fine
	}

	var peers []*Peer
	if err := json.Unmarshal(data, &peers); err != nil {
		return nil, fmt.Errorf("parse peers: %w", err)
	}
	for _, p := range peers {
		ps.peers[p.Name] = p
	}
	return ps, nil
}

// AddPeer adds or updates a trusted peer.
func (ps *PeerStore) AddPeer(name, address string, pk *[crypto.MLDSAPublicKeySize]byte) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.peers[name] = &Peer{
		Name:        name,
		Address:     address,
		PublicKey:   hex.EncodeToString(pk[:]),
		Fingerprint: crypto.Fingerprint(pk),
		PairedAt:    time.Now(),
	}
	return ps.save()
}

// GetPeer returns a peer by name.
func (ps *PeerStore) GetPeer(name string) (*Peer, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	p, ok := ps.peers[name]
	return p, ok
}

// GetPeerPublicKey decodes and returns the peer's public key bytes.
func (ps *PeerStore) GetPeerPublicKey(name string) (*[crypto.MLDSAPublicKeySize]byte, error) {
	p, ok := ps.GetPeer(name)
	if !ok {
		return nil, fmt.Errorf("unknown peer: %s", name)
	}
	pkBytes, err := hex.DecodeString(p.PublicKey)
	if err != nil || len(pkBytes) != crypto.MLDSAPublicKeySize {
		return nil, fmt.Errorf("invalid public key for peer %s", name)
	}
	var pk [crypto.MLDSAPublicKeySize]byte
	copy(pk[:], pkBytes)
	return &pk, nil
}

// ListPeers returns all known peers.
func (ps *PeerStore) ListPeers() []*Peer {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	result := make([]*Peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		result = append(result, p)
	}
	return result
}

// RemovePeer removes a peer by name.
func (ps *PeerStore) RemovePeer(name string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.peers, name)
	return ps.save()
}

func (ps *PeerStore) save() error {
	peers := make([]*Peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		peers = append(peers, p)
	}
	data, err := json.MarshalIndent(peers, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ps.path, data, 0600)
}
