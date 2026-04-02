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
