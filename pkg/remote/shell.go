//go:build darwin || linux

package remote

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/quantum-encoding/ztransfer-public/pkg/nat"
	"golang.org/x/sys/unix"
)

// ShellServer runs on the remote machine, spawning a PTY-backed shell.
type ShellServer struct {
	tunnel *nat.Tunnel
	cmd    *exec.Cmd
	ptmx   *os.File // master PTY fd
}

// ShellClient runs on the local machine, bridging the local terminal to
// the remote PTY.
type ShellClient struct {
	tunnel *nat.Tunnel
}

// openPTY opens a pseudo-terminal pair and returns the master and slave file
// descriptors. Works on both macOS and Linux using /dev/ptmx. The
// platform-specific grantpt, unlockpt, and ptsname functions are in
// pty_darwin.go and pty_linux.go.
func openPTY() (master *os.File, slave *os.File, err error) {
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}

	fd := int(ptmx.Fd())

	if err := grantpt(fd); err != nil {
		ptmx.Close()
		return nil, nil, fmt.Errorf("grantpt: %w", err)
	}
	if err := unlockpt(fd); err != nil {
		ptmx.Close()
		return nil, nil, fmt.Errorf("unlockpt: %w", err)
	}

	slavePath, err := ptsname(fd)
	if err != nil {
		ptmx.Close()
		return nil, nil, fmt.Errorf("ptsname: %w", err)
	}

	pts, err := os.OpenFile(slavePath, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		ptmx.Close()
		return nil, nil, fmt.Errorf("open slave %s: %w", slavePath, err)
	}

	return ptmx, pts, nil
}

// defaultShell returns the user's preferred shell.
func defaultShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/bash"
}

// ServeShell starts a shell server over the tunnel. It spawns the user's
// default shell attached to a PTY and bridges I/O through the tunnel.
// This function blocks until the shell exits or the tunnel closes.
func ServeShell(tunnel *nat.Tunnel) error {
	master, slave, err := openPTY()
	if err != nil {
		return fmt.Errorf("serve shell: %w", err)
	}
	defer master.Close()

	shell := defaultShell()
	cmd := exec.Command(shell)
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}

	if err := cmd.Start(); err != nil {
		slave.Close()
		return fmt.Errorf("start shell: %w", err)
	}
	// Close slave in parent; the child owns it now.
	slave.Close()

	_ = &ShellServer{
		tunnel: tunnel,
		cmd:    cmd,
		ptmx:   master,
	}

	router := NewRouter(tunnel)

	// Handle exec requests alongside shell traffic.
	HandleExecRequests(router, tunnel)

	// Handle stdin from the client: write to the PTY master.
	router.Handle(MsgStdin, func(payload []byte) {
		master.Write(payload)
	})

	// Handle resize requests from the client.
	router.Handle(MsgResize, func(payload []byte) {
		if len(payload) >= 4 {
			cols := binary.BigEndian.Uint16(payload[0:2])
			rows := binary.BigEndian.Uint16(payload[2:4])
			setWinSize(master, cols, rows)
		}
	})

	// Handle ping with pong.
	router.Handle(MsgPing, func(payload []byte) {
		router.Send(MsgPong, payload)
	})

	// Forward PTY output to the client.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, readErr := master.Read(buf)
			if n > 0 {
				if sendErr := router.Send(MsgStdout, buf[:n]); sendErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	// Wait for the shell to exit.
	exitCode := 0
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	// Send the exit code to the client.
	exitPayload := make([]byte, 4)
	binary.BigEndian.PutUint32(exitPayload, uint32(exitCode))
	router.Send(MsgExit, exitPayload)
	router.Stop()

	return nil
}

// setWinSize sets the terminal window size on the PTY master.
func setWinSize(f *os.File, cols, rows uint16) {
	ws := unix.Winsize{
		Row: rows,
		Col: cols,
	}
	unix.IoctlSetWinsize(int(f.Fd()), unix.TIOCSWINSZ, &ws)
}

// ConnectShell connects to a remote shell server and bridges the local
// terminal. It puts the local terminal in raw mode and forwards all I/O
// through the tunnel. Terminal state is restored on exit, even on panic.
func ConnectShell(tunnel *nat.Tunnel) error {
	stdinFd := int(os.Stdin.Fd())

	// Save and restore terminal state.
	origTermios, err := getTermios(stdinFd)
	if err != nil {
		return fmt.Errorf("get terminal state: %w", err)
	}
	defer restoreTermios(stdinFd, origTermios)

	// Also restore on panic.
	defer func() {
		if r := recover(); r != nil {
			restoreTermios(stdinFd, origTermios)
			panic(r) // re-panic after restoring
		}
	}()

	// Set raw mode.
	if err := setRawMode(stdinFd); err != nil {
		return fmt.Errorf("set raw mode: %w", err)
	}

	router := NewRouter(tunnel)
	exitCh := make(chan int, 1)

	// Handle stdout from the remote shell.
	router.Handle(MsgStdout, func(payload []byte) {
		os.Stdout.Write(payload)
	})

	// Handle remote shell exit.
	router.Handle(MsgExit, func(payload []byte) {
		code := 0
		if len(payload) >= 4 {
			code = int(binary.BigEndian.Uint32(payload))
		}
		exitCh <- code
		router.Stop()
	})

	// Handle error messages from the server.
	router.Handle(MsgError, func(payload []byte) {
		fmt.Fprintf(os.Stderr, "\r\nremote error: %s\r\n", string(payload))
	})

	// Handle pong (keepalive response).
	router.Handle(MsgPong, func(payload []byte) {
		// Keepalive acknowledged; nothing to do.
	})

	// Forward local stdin to the remote shell.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := os.Stdin.Read(buf)
			if n > 0 {
				if sendErr := router.Send(MsgStdin, buf[:n]); sendErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	// Send initial terminal size, then watch for SIGWINCH.
	sendTermSize(router, stdinFd)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			sendTermSize(router, stdinFd)
		}
	}()
	defer signal.Stop(sigCh)

	// Start routing messages (blocks until done).
	routerErr := make(chan error, 1)
	go func() {
		routerErr <- router.Start()
	}()

	// Wait for exit or router error.
	select {
	case code := <-exitCh:
		if code != 0 {
			return fmt.Errorf("remote shell exited with code %d", code)
		}
		return nil
	case err := <-routerErr:
		return err
	}
}

// sendTermSize reads the current terminal dimensions and sends a resize
// message to the remote shell server.
func sendTermSize(router *MessageRouter, fd int) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		return
	}
	payload := make([]byte, 4)
	binary.BigEndian.PutUint16(payload[0:2], ws.Col)
	binary.BigEndian.PutUint16(payload[2:4], ws.Row)
	router.Send(MsgResize, payload)
}

// readFromReader reads from an io.Reader into a channel of byte slices.
// Closes the channel on EOF or error.
func readFromReader(r io.Reader, ch chan<- []byte) {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			ch <- data
		}
		if err != nil {
			close(ch)
			return
		}
	}
}
