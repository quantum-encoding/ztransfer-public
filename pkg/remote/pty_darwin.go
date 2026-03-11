//go:build darwin

package remote

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// grantpt grants access to the slave PTY on macOS using TIOCPTYGRANT.
func grantpt(fd int) error {
	return unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0)
}

// unlockpt unlocks the slave PTY on macOS using TIOCPTYUNLK.
func unlockpt(fd int) error {
	return unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0)
}

// ptsname returns the path to the slave PTY device on macOS.
// Uses the TIOCPTYGNAME ioctl which writes a null-terminated path into a buffer.
func ptsname(fd int) (string, error) {
	buf := make([]byte, 128)
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(unix.TIOCPTYGNAME),
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if errno != 0 {
		return "", fmt.Errorf("TIOCPTYGNAME: %w", errno)
	}
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i]), nil
		}
	}
	return string(buf), nil
}

// getTermios retrieves the current terminal attributes on macOS.
func getTermios(fd int) (*unix.Termios, error) {
	return unix.IoctlGetTermios(fd, unix.TIOCGETA)
}

// restoreTermios restores terminal attributes on macOS.
func restoreTermios(fd int, termios *unix.Termios) {
	unix.IoctlSetTermios(fd, unix.TIOCSETA, termios)
}

// setRawMode puts the terminal into raw mode on macOS.
func setRawMode(fd int) error {
	termios, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return err
	}

	termios.Iflag &^= unix.BRKINT | unix.ICRNL | unix.INPCK | unix.ISTRIP | unix.IXON
	termios.Oflag &^= unix.OPOST
	termios.Cflag &^= unix.CSIZE | unix.PARENB
	termios.Cflag |= unix.CS8
	termios.Lflag &^= unix.ECHO | unix.ICANON | unix.ISIG | unix.IEXTEN
	termios.Cc[unix.VMIN] = 1
	termios.Cc[unix.VTIME] = 0

	return unix.IoctlSetTermios(fd, unix.TIOCSETA, termios)
}
