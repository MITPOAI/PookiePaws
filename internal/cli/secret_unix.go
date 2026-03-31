//go:build !windows

package cli

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
)

// ReadSecret reads a line from stdin with terminal echo disabled.
// Falls back to a normal read when stdin is not a TTY.
func ReadSecret() (string, error) {
	disable := exec.Command("stty", "-echo")
	disable.Stdin = os.Stdin
	if err := disable.Run(); err != nil {
		// Not a TTY (piped input). Read normally.
		r := bufio.NewReader(os.Stdin)
		val, err := r.ReadString('\n')
		return strings.TrimRight(val, "\r\n"), err
	}

	restore := exec.Command("stty", "echo")
	restore.Stdin = os.Stdin
	defer restore.Run() //nolint:errcheck

	r := bufio.NewReader(os.Stdin)
	val, err := r.ReadString('\n')
	return strings.TrimRight(val, "\r\n"), err
}
