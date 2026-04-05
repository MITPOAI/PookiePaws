//go:build !windows

package cli

import (
	"os"
	"syscall"
	"unsafe"
)

type terminalState struct {
	fd      uintptr
	origios syscall.Termios
}

func enableRawMode() (*terminalState, error) {
	fd := os.Stdin.Fd()

	var orig syscall.Termios
	if _, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL, fd,
		ioctlReadTermios, uintptr(unsafe.Pointer(&orig)),
		0, 0, 0,
	); errno != 0 {
		return nil, errno
	}

	raw := orig
	// Disable canonical mode (ICANON) and echo (ECHO) so we get individual
	// keystrokes without waiting for Enter and without printing them.
	raw.Lflag &^= syscall.ICANON | syscall.ECHO
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0

	if _, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL, fd,
		ioctlWriteTermios, uintptr(unsafe.Pointer(&raw)),
		0, 0, 0,
	); errno != 0 {
		return nil, errno
	}

	return &terminalState{fd: fd, origios: orig}, nil
}

func restoreMode(state *terminalState) {
	if state == nil {
		return
	}
	syscall.Syscall6(
		syscall.SYS_IOCTL, state.fd,
		ioctlWriteTermios, uintptr(unsafe.Pointer(&state.origios)),
		0, 0, 0,
	)
}

// isTerminal reports whether the given file is attached to a terminal.
func isTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
