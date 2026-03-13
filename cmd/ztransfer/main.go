package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/quantum-encoding/ztransfer/pkg/api"
	"github.com/quantum-encoding/ztransfer/pkg/auth"
	"github.com/quantum-encoding/ztransfer/pkg/client"
	"github.com/quantum-encoding/ztransfer/pkg/crypto"
	"github.com/quantum-encoding/ztransfer/pkg/nat"
	"github.com/quantum-encoding/ztransfer/pkg/remote"
	"github.com/quantum-encoding/ztransfer/pkg/server"
)

const version = "0.2.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		cmdServe(os.Args[2:])
	case "api":
		cmdAPI(os.Args[2:])
	case "pair":
		cmdPair(os.Args[2:])
	case "ls":
		cmdList(os.Args[2:])
	case "get":
		cmdGet(os.Args[2:])
	case "put":
		cmdPut(os.Args[2:])
	case "peers":
		cmdPeers()
	case "status":
		cmdStatus(os.Args[2:])
	case "remote":
		cmdRemote(os.Args[2:])
	case "version":
		fmt.Printf("ztransfer %s (quantum vault %s)\n", version, crypto.Version())
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`ztransfer %s — Secure file transfer & remote access with post-quantum authentication

Usage:
  ztransfer serve [--dir DIR] [--port PORT] [--name NAME]
  ztransfer api [--port PORT]
  ztransfer pair ADDRESS --token TOKEN [--name NAME]
  ztransfer ls PEER:/path/
  ztransfer get PEER:/path/file [LOCAL_DIR]
  ztransfer put LOCAL_FILE PEER:/path/
  ztransfer peers
  ztransfer status ADDRESS
  ztransfer remote host [--port PORT]
  ztransfer remote connect CODE [--host ADDRESS]
  ztransfer remote shell CODE [--host ADDRESS]
  ztransfer remote exec CODE COMMAND [ARGS...]
  ztransfer version

Commands:
  serve     Start the file server
  api       Start local REST API for Claude Code / programmatic access
  pair      Pair with a remote ztransfer server (one-time)
  ls        List files on a paired peer
  get       Download a file from a paired peer
  put       Upload a file to a paired peer
  peers     List paired peers
  status    Show local status + relay health, or check a remote server
  version   Show version info

Remote Access:
  remote host       Host a remote session (prints warp code for connector)
  remote connect    Connect to a hosted session
  remote shell      Open interactive shell on remote machine
  remote exec       Run a single command on remote machine

API Mode (for Claude Code):
  ztransfer api --port 9877
  curl http://localhost:9877/api/peers
  curl http://localhost:9877/api/ls?peer=NAME&path=/
  curl -X POST http://localhost:9877/api/get -d '{"peer":"NAME","remote_path":"/file","local_path":"/tmp/"}'
  curl 'http://localhost:9877/api/receive?peer=NAME&path=/file' > file
  curl -X POST http://localhost:9877/api/remote/exec -d '{"code":"warp-429-delta","command":"pacman -S brave-bin"}'
`, version)
}

// parsePeerPath splits "peer:/path" into (peer, path).
func parsePeerPath(s string) (string, string, error) {
	idx := strings.Index(s, ":")
	if idx < 1 {
		return "", "", fmt.Errorf("invalid format %q — use PEER:/path", s)
	}
	return s[:idx], s[idx+1:], nil
}

func getFlag(args []string, flag, defaultVal string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return defaultVal
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	// Strip domain suffix
	if idx := strings.Index(h, "."); idx > 0 {
		h = h[:idx]
	}
	return h
}

func cmdServe(args []string) {
	dir := getFlag(args, "--dir", ".")
	port := getFlag(args, "--port", "9876")
	name := getFlag(args, "--name", hostname())

	absDir, err := filepath.Abs(dir)
	if err != nil {
		fatal("resolve path: %v", err)
	}

	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		fatal("not a directory: %s", absDir)
	}

	identity, err := auth.LoadOrCreateIdentity(name)
	if err != nil {
		fatal("identity: %v", err)
	}

	peerStore, err := auth.LoadPeerStore()
	if err != nil {
		fatal("peer store: %v", err)
	}

	token, err := auth.GeneratePairToken()
	if err != nil {
		fatal("generate token: %v", err)
	}

	portNum := 9876
	fmt.Sscanf(port, "%d", &portNum)

	srv := &server.Server{
		RootDir:   absDir,
		Identity:  identity,
		PeerStore: peerStore,
		PairToken: token,
		Port:      portNum,
	}

	if err := srv.Start(); err != nil {
		fatal("server: %v", err)
	}
}

func cmdPair(args []string) {
	if len(args) < 1 {
		fatal("usage: ztransfer pair ADDRESS --token TOKEN")
	}

	address := args[0]
	token := getFlag(args, "--token", "")
	name := getFlag(args, "--name", hostname())

	if token == "" {
		fatal("--token required")
	}

	// Ensure address has port
	if !strings.Contains(address, ":") {
		address += ":9876"
	}

	identity, err := auth.LoadOrCreateIdentity(name)
	if err != nil {
		fatal("identity: %v", err)
	}

	peerStore, err := auth.LoadPeerStore()
	if err != nil {
		fatal("peer store: %v", err)
	}

	fmt.Printf("  Pairing with %s...\n", address)
	if err := auth.RequestPair(address, token, identity, peerStore); err != nil {
		fatal("pairing failed: %v", err)
	}
	fmt.Printf("  Pairing successful!\n")
}

func cmdList(args []string) {
	if len(args) < 1 {
		fatal("usage: ztransfer ls PEER:/path/")
	}

	peerName, remotePath, err := parsePeerPath(args[0])
	if err != nil {
		fatal("%v", err)
	}

	c := mustClient()
	files, err := c.List(peerName, remotePath)
	if err != nil {
		fatal("ls: %v", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, f := range files {
		sizeStr := formatBytes(f.Size)
		if f.IsDir {
			sizeStr = "-"
		}
		name := f.Name
		if f.IsDir {
			name += "/"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", f.Mode, sizeStr, f.ModTime[:10], name)
	}
	w.Flush()
}

func cmdGet(args []string) {
	if len(args) < 1 {
		fatal("usage: ztransfer get PEER:/path/file [LOCAL_DIR]")
	}

	peerName, remotePath, err := parsePeerPath(args[0])
	if err != nil {
		fatal("%v", err)
	}

	localDir := "."
	if len(args) > 1 {
		localDir = args[1]
	}

	c := mustClient()
	fmt.Printf("  Downloading %s from %s...\n", remotePath, peerName)
	written, err := c.Download(peerName, remotePath, localDir)
	if err != nil {
		fatal("download: %v", err)
	}
	fmt.Printf("  Downloaded %s (%s)\n", filepath.Base(remotePath), formatBytes(written))
}

func cmdPut(args []string) {
	if len(args) < 2 {
		fatal("usage: ztransfer put LOCAL_FILE PEER:/path/")
	}

	localPath := args[0]
	peerName, remotePath, err := parsePeerPath(args[1])
	if err != nil {
		fatal("%v", err)
	}

	c := mustClient()
	fmt.Printf("  Uploading %s to %s:%s...\n", localPath, peerName, remotePath)
	written, err := c.Upload(peerName, localPath, remotePath)
	if err != nil {
		fatal("upload: %v", err)
	}
	fmt.Printf("  Uploaded %s (%s)\n", filepath.Base(localPath), formatBytes(written))
}

func cmdPeers() {
	peerStore, err := auth.LoadPeerStore()
	if err != nil {
		fatal("peer store: %v", err)
	}

	peers := peerStore.ListPeers()
	if len(peers) == 0 {
		fmt.Println("  No paired peers. Use 'ztransfer pair' to add one.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tADDRESS\tFINGERPRINT\tPAIRED\n")
	for _, p := range peers {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name, p.Address, p.Fingerprint, p.PairedAt.Format("2006-01-02"))
	}
	w.Flush()
}

func cmdAPI(args []string) {
	port := getFlag(args, "--port", "9877")
	name := getFlag(args, "--name", hostname())

	identity, err := auth.LoadOrCreateIdentity(name)
	if err != nil {
		fatal("identity: %v", err)
	}

	peerStore, err := auth.LoadPeerStore()
	if err != nil {
		fatal("peer store: %v", err)
	}

	c := client.New(identity, peerStore)

	portNum := 9877
	fmt.Sscanf(port, "%d", &portNum)

	srv := &api.Server{
		Identity:    identity,
		PeerStore:   peerStore,
		Client:      c,
		DownloadDir: ".",
		Port:        portNum,
	}

	if err := srv.Start(); err != nil {
		fatal("api: %v", err)
	}
}

func cmdStatus(args []string) {
	if len(args) < 1 {
		// No address given — show local status including relay
		identity, err := auth.LoadOrCreateIdentity(hostname())
		if err != nil {
			fatal("identity: %v", err)
		}
		peerStore, err := auth.LoadPeerStore()
		if err != nil {
			fatal("peer store: %v", err)
		}

		fmt.Printf("\n  ztransfer %s\n", version)
		fmt.Printf("  %-14s %s\n", "Identity:", identity.Name)
		fmt.Printf("  %-14s %s\n", "Fingerprint:", identity.Fingerprint())
		fmt.Printf("  %-14s %d paired\n", "Peers:", len(peerStore.ListPeers()))

		// Relay status
		relayURL := os.Getenv("ZTRANSFER_RELAY_URL")
		if relayURL == "" {
			relayURL = nat.DefaultRelayURL
		}
		relayDisabled := strings.EqualFold(os.Getenv("ZTRANSFER_RELAY"), "off")

		fmt.Println()
		fmt.Printf("  Relay:\n")
		if relayDisabled {
			fmt.Printf("    %-12s disabled (ZTRANSFER_RELAY=off)\n", "Status:")
		} else {
			fmt.Printf("    %-12s %s\n", "URL:", relayURL)

			// Check relay health
			httpClient := &http.Client{Timeout: 5 * time.Second}
			resp, err := httpClient.Get(relayURL + "/health")
			if err != nil {
				fmt.Printf("    %-12s unreachable (%v)\n", "Health:", err)
			} else {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					fmt.Printf("    %-12s ok\n", "Health:")
				} else {
					fmt.Printf("    %-12s %s\n", "Health:", resp.Status)
				}
			}

			// Check token
			token := os.Getenv("ZTRANSFER_RELAY_TOKEN")
			if token != "" {
				fmt.Printf("    %-12s from ZTRANSFER_RELAY_TOKEN\n", "Auth:")
			} else {
				// Try gcloud
				cmd := exec.Command("gcloud", "auth", "print-identity-token",
					"--audiences="+relayURL)
				if out, err := cmd.Output(); err == nil && len(strings.TrimSpace(string(out))) > 0 {
					fmt.Printf("    %-12s gcloud identity token\n", "Auth:")
				} else {
					fmt.Printf("    %-12s none (set ZTRANSFER_RELAY_TOKEN or install gcloud)\n", "Auth:")
				}
			}
		}
		fmt.Println()
		return
	}

	address := args[0]
	if !strings.Contains(address, ":") {
		address += ":9876"
	}

	c := mustClient()
	status, err := c.Status(address)
	if err != nil {
		fatal("status: %v", err)
	}

	fmt.Printf("  Server: %s\n", status["name"])
	fmt.Printf("  Fingerprint: %s\n", status["fingerprint"])
	fmt.Printf("  Version: %s\n", status["version"])
	fmt.Printf("  Root: %s\n", status["root_dir"])
}

func cmdRemote(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, `Usage:
  ztransfer remote host [--port PORT]        Host a session (prints warp code)
  ztransfer remote connect CODE [--host ADDR] Connect to a hosted session
  ztransfer remote shell CODE [--host ADDR]   Interactive shell on remote
  ztransfer remote exec CODE COMMAND [ARGS]   Run command on remote
`)
		os.Exit(1)
	}

	identity, err := auth.LoadOrCreateIdentity(hostname())
	if err != nil {
		fatal("identity: %v", err)
	}

	switch args[0] {
	case "host":
		port := 9878
		if p := getFlag(args, "--port", ""); p != "" {
			fmt.Sscanf(p, "%d", &port)
		}
		session, err := remote.HostSession(identity, port)
		if err != nil {
			fatal("host: %v", err)
		}
		defer session.Close()

		fmt.Printf("\n  Remote session active\n")
		fmt.Printf("  Warp code: %s\n", session.WarpCode)
		fmt.Printf("  Waiting for connections...\n\n")

		// Serve incoming requests (blocks)
		if err := remote.ServeShell(session.Tunnel); err != nil {
			fatal("shell server: %v", err)
		}

	case "connect":
		if len(args) < 2 {
			fatal("usage: ztransfer remote connect CODE [--host ADDRESS]")
		}
		code := args[1]
		hostAddr := getFlag(args, "--host", "")

		session, err := remote.ConnectSession(identity, code, hostAddr)
		if err != nil {
			fatal("connect: %v", err)
		}
		defer session.Close()
		fmt.Printf("  Connected to %s\n", session.PeerName)

	case "shell":
		if len(args) < 2 {
			fatal("usage: ztransfer remote shell CODE [--host ADDRESS]")
		}
		code := args[1]
		hostAddr := getFlag(args, "--host", "")

		session, err := remote.ConnectSession(identity, code, hostAddr)
		if err != nil {
			fatal("connect: %v", err)
		}
		defer session.Close()

		fmt.Printf("  Connected — opening shell...\n\n")
		if err := remote.ConnectShell(session.Tunnel); err != nil {
			fatal("shell: %v", err)
		}

	case "exec":
		if len(args) < 3 {
			fatal("usage: ztransfer remote exec CODE COMMAND [ARGS...]")
		}
		code := args[1]
		command := args[2]
		cmdArgs := args[3:]
		hostAddr := getFlag(args, "--host", "")

		session, err := remote.ConnectSession(identity, code, hostAddr)
		if err != nil {
			fatal("connect: %v", err)
		}
		defer session.Close()

		resp, err := session.Exec(command, cmdArgs...)
		if err != nil {
			fatal("exec: %v", err)
		}

		if resp.Stdout != "" {
			fmt.Print(resp.Stdout)
		}
		if resp.Stderr != "" {
			fmt.Fprint(os.Stderr, resp.Stderr)
		}
		os.Exit(resp.ExitCode)

	default:
		fatal("unknown remote command: %s", args[0])
	}
}

func mustClient() *client.Client {
	identity, err := auth.LoadOrCreateIdentity(hostname())
	if err != nil {
		fatal("identity: %v", err)
	}
	peerStore, err := auth.LoadPeerStore()
	if err != nil {
		fatal("peer store: %v", err)
	}
	return client.New(identity, peerStore)
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

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "  error: "+format+"\n", args...)
	os.Exit(1)
}
