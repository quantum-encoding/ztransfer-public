package auth

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/quantum-encoding/ztransfer/pkg/crypto"
)

// Identity represents this machine's ML-DSA-65 identity.
type Identity struct {
	PublicKey  [crypto.MLDSAPublicKeySize]byte
	SecretKey  [crypto.MLDSASecretKeySize]byte
	Name       string
	configDir  string
}

type identityFile struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	SecretKey string `json:"secret_key"`
}

// ConfigDir returns the ztransfer config directory (~/.ztransfer).
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".ztransfer")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// LoadOrCreateIdentity loads the machine identity or creates a new one.
func LoadOrCreateIdentity(name string) (*Identity, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	idPath := filepath.Join(dir, "identity.json")

	data, err := os.ReadFile(idPath)
	if err == nil {
		return loadIdentity(data, dir)
	}

	// Generate new identity
	kp, err := crypto.GenerateMLDSAKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate identity: %w", err)
	}

	id := &Identity{
		PublicKey:  kp.PublicKey,
		SecretKey:  kp.SecretKey,
		Name:       name,
		configDir:  dir,
	}

	if err := id.save(idPath); err != nil {
		return nil, err
	}
	return id, nil
}

func loadIdentity(data []byte, dir string) (*Identity, error) {
	var f identityFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse identity: %w", err)
	}
	id := &Identity{Name: f.Name, configDir: dir}

	pk, err := hex.DecodeString(f.PublicKey)
	if err != nil || len(pk) != crypto.MLDSAPublicKeySize {
		return nil, fmt.Errorf("invalid public key in identity file")
	}
	copy(id.PublicKey[:], pk)

	sk, err := hex.DecodeString(f.SecretKey)
	if err != nil || len(sk) != crypto.MLDSASecretKeySize {
		return nil, fmt.Errorf("invalid secret key in identity file")
	}
	copy(id.SecretKey[:], sk)
	return id, nil
}

func (id *Identity) save(path string) error {
	f := identityFile{
		Name:      id.Name,
		PublicKey: hex.EncodeToString(id.PublicKey[:]),
		SecretKey: hex.EncodeToString(id.SecretKey[:]),
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Fingerprint returns the short hex fingerprint of this identity.
func (id *Identity) Fingerprint() string {
	return crypto.Fingerprint(&id.PublicKey)
}

// Sign signs a message with this identity's secret key.
func (id *Identity) Sign(message []byte) ([crypto.MLDSASignatureSize]byte, error) {
	return crypto.Sign(&id.SecretKey, message)
}
