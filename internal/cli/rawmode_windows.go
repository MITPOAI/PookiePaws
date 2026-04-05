//go:build windows

package cli

import (
	"os"
	"unsafe"
)

// Windows console mode flags.
const (
	enableProcessedInput       uint32 = 0x0001
	enableLineInput            uint32 = 0x0002
	enableEchoInputMenu        uint32 = 0x0004
	enableVirtualTerminalInput uint32 = 0x0200
)

type terminalState struct {
	fd       uintptr
	origMode uint32
}

func enableRawMode() (*terminalState, error) {
	fd := os.Stdin.Fd()

	var origMode uint32
	r, _, err := getConsoleMode.Call(fd, uintptr(unsafe.Pointer(&origMode)))
	if r == 0 {
		return nil, err
	}

	// Disable line input and echo; enable virtual terminal input so that
	// ANSI escape sequences (arrow keys) arrive as escape byte sequences
	// rather than Windows-specific virtual-key records.
	raw := (origMode &^ (enableLineInput | enableEchoInputMenu)) | enableVirtualTerminalInput
	r, _, err = setConsoleMode.Call(fd, uintptr(raw))
	if r == 0 {
		return nil, err
	}

	return &terminalState{fd: fd, origMode: origMode}, nil
}

func restoreMode(state *terminalState) {
	if state == nil {
		return
	}
	setConsoleMode.Call(state.fd, uintptr(state.origMode))
}

// isTerminal reports whether the given file is a Windows console handle.
// Uses GetConsoleMode from kernel32.dll which returns 0 for non-console
// handles (pipes, NUL, files) and nonzero for real consoles.
func isTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	var mode uint32
	r, _, _ := getConsoleMode.Call(file.Fd(), uintptr(unsafe.Pointer(&mode)))
	return r != 0
}
