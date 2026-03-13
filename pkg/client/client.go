package client

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/quantum-encoding/ztransfer/pkg/auth"
	"github.com/quantum-encoding/ztransfer/pkg/server"
)

// Client is the ztransfer file transfer client.
type Client struct {
	Identity  *auth.Identity
	PeerStore *auth.PeerStore
	http      *http.Client
}

// New creates a new ztransfer client.
func New(identity *auth.Identity, peerStore *auth.PeerStore) *Client {
	return &Client{
		Identity:  identity,
		PeerStore: peerStore,
		http: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					MinVersion:        tls.VersionTLS13,
				},
			},
			Timeout: 5 * time.Minute,
		},
	}
}

// signRequest adds ML-DSA authentication headers.
func (c *Client) signRequest(req *http.Request) error {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	nonceHex := hex.EncodeToString(nonce)

	message := []byte(req.Method + req.URL.Path + nonceHex)
	sig, err := c.Identity.Sign(message)
	if err != nil {
		return fmt.Errorf("sign request: %w", err)
	}

	req.Header.Set("X-ZTransfer-Peer", c.Identity.Name)
	req.Header.Set("X-ZTransfer-Sig", hex.EncodeToString(sig[:]))
	req.Header.Set("X-ZTransfer-Nonce", nonceHex)
	return nil
}

// List lists files at a remote path.
func (c *Client) List(peerName, remotePath string) ([]server.FileInfo, error) {
	peer, ok := c.PeerStore.GetPeer(peerName)
	if !ok {
		return nil, fmt.Errorf("unknown peer: %s (run 'ztransfer pair' first)", peerName)
	}

	reqURL := fmt.Sprintf("https://%s/api/v1/ls?path=%s", peer.Address, url.QueryEscape(remotePath))
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	if err := c.signRequest(req); err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
	}

	var files []server.FileInfo
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}
	return files, nil
}

// Download downloads a file from a remote peer.
func (c *Client) Download(peerName, remotePath, localDir string) (int64, error) {
	peer, ok := c.PeerStore.GetPeer(peerName)
	if !ok {
		return 0, fmt.Errorf("unknown peer: %s", peerName)
	}

	reqURL := fmt.Sprintf("https://%s/api/v1/file?path=%s", peer.Address, url.QueryEscape(remotePath))
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return 0, err
	}
	if err := c.signRequest(req); err != nil {
		return 0, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
	}

	fileName := filepath.Base(remotePath)
	localPath := filepath.Join(localDir, fileName)

	f, err := os.Create(localPath)
	if err != nil {
		return 0, fmt.Errorf("create local file: %w", err)
	}
	defer f.Close()

	written, err := io.Copy(f, resp.Body)
	if err != nil {
		return 0, fmt.Errorf("write: %w", err)
	}

	return written, nil
}

// Upload uploads a local file to a remote peer.
func (c *Client) Upload(peerName, localPath, remotePath string) (int64, error) {
	peer, ok := c.PeerStore.GetPeer(peerName)
	if !ok {
		return 0, fmt.Errorf("unknown peer: %s", peerName)
	}

	f, err := os.Open(localPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0, err
	}

	// If remotePath is a directory-like path, append the filename
	if remotePath == "" || remotePath[len(remotePath)-1] == '/' {
		remotePath += filepath.Base(localPath)
	}

	reqURL := fmt.Sprintf("https://%s/api/v1/upload?path=%s", peer.Address, url.QueryEscape(remotePath))
	req, err := http.NewRequest("POST", reqURL, f)
	if err != nil {
		return 0, err
	}
	req.ContentLength = info.Size()
	if err := c.signRequest(req); err != nil {
		return 0, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
	}

	return info.Size(), nil
}

// Status gets the remote server status.
func (c *Client) Status(address string) (map[string]any, error) {
	reqURL := fmt.Sprintf("https://%s/api/v1/status", address)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}
