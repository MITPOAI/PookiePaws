//go:build !windows

package cli

import (
	"bufio"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// ReadSecret reads a line from stdin with terminal echo disabled.
// Falls back to a normal read when stdin is not a TTY.
func ReadSecret() (string, error) {
	fd := os.Stdin.Fd()

	var orig syscall.Termios
	if _, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL, fd,
		ioctlReadTermios, uintptr(unsafe.Pointer(&orig)),
		0, 0, 0,
	); errno != 0 {
		r := bufio.NewReader(os.Stdin)
		val, err := r.ReadString('\n')
		return strings.TrimRight(val, "\r\n"), err
	}

	masked := orig
	masked.Lflag &^= syscall.ECHO

	if _, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL, fd,
		ioctlWriteTermios, uintptr(unsafe.Pointer(&masked)),
		0, 0, 0,
	); errno != 0 {
		r := bufio.NewReader(os.Stdin)
		val, err := r.ReadString('\n')
		return strings.TrimRight(val, "\r\n"), err
	}
	defer syscall.Syscall6(
		syscall.SYS_IOCTL, fd,
		ioctlWriteTermios, uintptr(unsafe.Pointer(&orig)),
		0, 0, 0,
	)

	r := bufio.NewReader(os.Stdin)
	val, err := r.ReadString('\n')
	return strings.TrimRight(val, "\r\n"), err
}
