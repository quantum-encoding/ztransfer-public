//go:build linux

package remote

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// grantpt grants access to the slave PTY on Linux.
// On modern Linux with devpts, this is typically a no-op.
func grantpt(fd int) error {
	return nil
}

// unlockpt unlocks the slave PTY on Linux using TIOCSPTLCK.
func unlockpt(fd int) error {
	return unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0)
}

// ptsname returns the path to the slave PTY device on Linux.
// Uses the TIOCGPTN ioctl to get the PTY number.
func ptsname(fd int) (string, error) {
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		return "", fmt.Errorf("TIOCGPTN: %w", err)
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}

// getTermios retrieves the current terminal attributes on Linux.
func getTermios(fd int) (*unix.Termios, error) {
	return unix.IoctlGetTermios(fd, unix.TCGETS)
}

// restoreTermios restores terminal attributes on Linux.
func restoreTermios(fd int, termios *unix.Termios) {
	unix.IoctlSetTermios(fd, unix.TCSETS, termios)
}

// setRawMode puts the terminal into raw mode on Linux.
func setRawMode(fd int) error {
	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
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

	return unix.IoctlSetTermios(fd, unix.TCSETS, termios)
}
