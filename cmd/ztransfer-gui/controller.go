package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"fyne.io/fyne/v2"

	"github.com/quantum-encoding/ztransfer/pkg/auth"
	"github.com/quantum-encoding/ztransfer/pkg/client"
	"github.com/quantum-encoding/ztransfer/pkg/crypto"
	"github.com/quantum-encoding/ztransfer/pkg/server"
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
	selectedPeer    string
	downloadDir     string
	statusText      string
	whitelistedDirs []string

	// Drop handler — set by transfer tab, called by window OnDropped
	onDrop func([]fyne.URI)
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

	// Load whitelisted dirs from config or default to common locations
	defaultDirs := []string{}
	if docDir := filepath.Join(home, "Documents"); dirExists(docDir) {
		defaultDirs = append(defaultDirs, docDir)
	}
	if dlDir := filepath.Join(home, "Downloads"); dirExists(dlDir) {
		defaultDirs = append(defaultDirs, dlDir)
	}

	return &Controller{
		identity:        identity,
		peerStore:       peerStore,
		client:          client.New(identity, peerStore),
		serverPort:      9876,
		serverDir:       home,
		downloadDir:     downloadDir,
		statusText:      "Ready",
		whitelistedDirs: defaultDirs,
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

// HandleDrop is called when files are dropped onto the window.
func (c *Controller) HandleDrop(_ fyne.Position, uris []fyne.URI) {
	c.mu.RLock()
	handler := c.onDrop
	c.mu.RUnlock()
	if handler != nil {
		handler(uris)
	}
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

// AddWhitelistedDir adds a directory to the discoverable whitelist.
func (c *Controller) AddWhitelistedDir(dir string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, d := range c.whitelistedDirs {
		if d == dir {
			return // already exists
		}
	}
	c.whitelistedDirs = append(c.whitelistedDirs, dir)
}

// RemoveWhitelistedDir removes a directory from the discoverable whitelist.
func (c *Controller) RemoveWhitelistedDir(dir string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, d := range c.whitelistedDirs {
		if d == dir {
			c.whitelistedDirs = append(c.whitelistedDirs[:i], c.whitelistedDirs[i+1:]...)
			return
		}
	}
}

// WhitelistedDirs returns the current discoverable directories.
func (c *Controller) WhitelistedDirs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]string, len(c.whitelistedDirs))
	copy(result, c.whitelistedDirs)
	return result
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// openInFileManager reveals a file or folder in the OS file manager.
func openInFileManager(path string) {
	dir := path
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}

	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", "-R", path).Start()
	case "linux":
		exec.Command("xdg-open", dir).Start()
	case "windows":
		exec.Command("explorer", "/select,", path).Start()
	}
}

// Unused import guard — these are used in other files in this package.
var (
	_ = server.Server{}
	_ = crypto.Version
)
