package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/quantum-encoding/ztransfer-public/pkg/auth"
	"github.com/quantum-encoding/ztransfer-public/pkg/client"
	"github.com/quantum-encoding/ztransfer-public/pkg/crypto"
	"github.com/quantum-encoding/ztransfer-public/pkg/server"
)

// Controller manages application state and coordinates between tabs.
type Controller struct {
	mu sync.RWMutex

	identity  *auth.Identity
	peerStore *auth.PeerStore
	client    *client.Client

	// Server state
	serverRunning bool
	serverCancel  func()
	serverPort    int
	serverDir     string
	pairToken     string

	// UI state
	selectedPeer string
	downloadDir  string
	statusText   string
}

func NewController() *Controller {
	name := hostname()
	identity, err := auth.LoadOrCreateIdentity(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "identity error: %v\n", err)
		os.Exit(1)
	}

	peerStore, err := auth.LoadPeerStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "peer store error: %v\n", err)
		os.Exit(1)
	}

	home, _ := os.UserHomeDir()
	downloadDir := home + "/Downloads"

	return &Controller{
		identity:    identity,
		peerStore:   peerStore,
		client:      client.New(identity, peerStore),
		serverPort:  9876,
		serverDir:   home,
		downloadDir: downloadDir,
		statusText:  "Ready",
	}
}

func (c *Controller) SetStatus(msg string) {
	c.mu.Lock()
	c.statusText = msg
	c.mu.Unlock()
}

func (c *Controller) GetStatus() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.statusText
}

func (c *Controller) IsServerRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.serverRunning
}

func (c *Controller) Shutdown() {
	c.mu.Lock()
	if c.serverCancel != nil {
		c.serverCancel()
	}
	c.mu.Unlock()
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	if idx := strings.Index(h, "."); idx > 0 {
		h = h[:idx]
	}
	return h
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// Unused import guard — these are used in other files in this package.
var (
	_ = server.Server{}
	_ = crypto.Version
)
