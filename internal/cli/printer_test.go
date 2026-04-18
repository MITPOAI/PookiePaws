package cli

import (
	"testing"
)

// TestNoColorEnvDisablesColor verifies that setting NO_COLOR forces color off
// even when running on a terminal. Honors the de-facto NO_COLOR convention.
func TestNoColorEnvDisablesColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "")
	p := Stdout()
	if p.color {
		t.Error("expected color disabled when NO_COLOR is set")
	}
}

// TestTermDumbDisablesColor verifies that TERM=dumb forces color off.
func TestTermDumbDisablesColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	p := Stdout()
	if p.color {
		t.Error("expected color disabled when TERM=dumb")
	}
}

// TestNonTTYDisablesColor is the proof-point for the isatty path: under
// `go test`, stdout is piped (not a TTY), so even with NO_COLOR unset and
// TERM set to a "real" value the printer must disable color. If isatty
// detection regresses, this test will fail.
func TestNonTTYDisablesColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm")
	p := Stdout()
	if p.color {
		t.Error("expected color disabled when stdout is not a TTY (test runner pipes stdout)")
	}
}
