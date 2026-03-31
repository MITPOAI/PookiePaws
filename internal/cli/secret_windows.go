//go:build windows

package cli

import (
	"bufio"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	getConsoleMode = kernel32.NewProc("GetConsoleMode")
	setConsoleMode = kernel32.NewProc("SetConsoleMode")
)

const enableEchoInput uint32 = 0x0004

// ReadSecret reads a line from stdin with terminal echo disabled using the
// Windows Console API via kernel32.dll. Falls back to a plain read when
// stdin is not attached to a console (e.g. piped input).
func ReadSecret() (string, error) {
	fd := os.Stdin.Fd()

	var mode uint32
	r, _, _ := getConsoleMode.Call(fd, uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		// Not a console — read normally.
		rdr := bufio.NewReader(os.Stdin)
		val, err := rdr.ReadString('\n')
		return strings.TrimRight(val, "\r\n"), err
	}

	// Disable echo.
	setConsoleMode.Call(fd, uintptr(mode&^enableEchoInput))
	defer setConsoleMode.Call(fd, uintptr(mode))

	rdr := bufio.NewReader(os.Stdin)
	val, err := rdr.ReadString('\n')
	return strings.TrimRight(val, "\r\n"), err
}
