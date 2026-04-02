// Package cli provides lightweight terminal output primitives using standard
// ANSI escape codes. No third-party dependencies are required.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// ── Colour constants ────────────────────────────────────────────────────────

const (
	ansiReset       = "\033[0m"
	ansiDim         = "\033[2m"
	ansiBoldGreen   = "\033[1;32m"
	ansiBoldRed     = "\033[1;31m"
	ansiBoldYellow  = "\033[1;33m"
	ansiBoldCyan    = "\033[1;36m"
	ansiBoldMagenta = "\033[1;35m" // mitpo.io accent
	ansiBoldWhite   = "\033[1;37m"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// noColor returns true when the environment requests plain output.
func noColor() bool {
	return os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"
}

// ── Printer ─────────────────────────────────────────────────────────────────

// Printer writes styled terminal output to an [io.Writer].
type Printer struct {
	out   io.Writer
	color bool
}

// New returns a Printer that writes to w.
// Colour is automatically disabled when NO_COLOR is set or TERM=dumb.
func New(w io.Writer) *Printer { return &Printer{out: w, color: !noColor()} }

// Stdout returns a Printer writing to os.Stdout.
func Stdout() *Printer { return New(os.Stdout) }

// Stderr returns a Printer writing to os.Stderr.
func Stderr() *Printer { return New(os.Stderr) }

// IsColor reports whether the printer uses colour output.
func (p *Printer) IsColor() bool { return p.color }

func (p *Printer) paint(code, s string) string {
	if !p.color {
		return s
	}
	return code + s + ansiReset
}

func (p *Printer) emit(code, prefix, format string, args []any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(p.out, p.paint(code, "  "+prefix)+" "+msg)
}

// Success prints a green "✓" line.
func (p *Printer) Success(format string, args ...any) { p.emit(ansiBoldGreen, "✓", format, args) }

// Error prints a red "✗" line.
func (p *Printer) Error(format string, args ...any) { p.emit(ansiBoldRed, "✗", format, args) }

// Warning prints a yellow "⚠" line.
func (p *Printer) Warning(format string, args ...any) { p.emit(ansiBoldYellow, "⚠", format, args) }

// Info prints a cyan "→" line.
func (p *Printer) Info(format string, args ...any) { p.emit(ansiBoldCyan, "→", format, args) }

// Blank prints an empty line.
func (p *Printer) Blank() { fmt.Fprintln(p.out) }

// Plain prints an indented unstyled line.
func (p *Printer) Plain(format string, args ...any) {
	fmt.Fprintf(p.out, "  "+format+"\n", args...)
}

// Accent prints text in the mitpo.io accent colour (bold magenta).
func (p *Printer) Accent(format string, args ...any) {
	fmt.Fprintln(p.out, p.paint(ansiBoldMagenta, fmt.Sprintf("  "+format, args...)))
}

// Dim prints faded text.
func (p *Printer) Dim(format string, args ...any) {
	fmt.Fprintln(p.out, p.paint(ansiDim, fmt.Sprintf("  "+format, args...)))
}

// Rule prints a horizontal divider with an optional label.
func (p *Printer) Rule(label string) {
	const total = 50
	var s string
	if label == "" {
		s = "  " + strings.Repeat("─", total)
	} else {
		prefix := "  ─── " + label + " "
		n := total - len(prefix) + 2
		if n < 0 {
			n = 0
		}
		s = prefix + strings.Repeat("─", n)
	}
	fmt.Fprintln(p.out, p.paint(ansiDim, s))
}

// Banner prints the PookiePaws identity header.
func (p *Printer) Banner() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, p.paint(ansiBoldMagenta, "  PookiePaws"))
	fmt.Fprintln(p.out, p.paint(ansiDim, "  Local-first marketing operations runtime · mitpo.io"))
	fmt.Fprintln(p.out)
}

// ── Box ─────────────────────────────────────────────────────────────────────

// boxW is the number of visible characters between the two side-border glyphs.
const boxW = 54

// Box renders a bordered summary panel.
// rows is a slice of [key, value] pairs displayed in aligned columns.
func (p *Printer) Box(title string, rows [][2]string) {
	// Measure the longest key for column alignment.
	maxKey := 0
	for _, r := range rows {
		if l := len(r[0]); l > maxKey {
			maxKey = l
		}
	}

	// Top border: ╭─ Title ──...──╮
	hdr := "─ " + title + " "
	topFill := boxW - len(hdr)
	if topFill < 0 {
		topFill = 0
	}
	p.emitDim("  ╭" + hdr + strings.Repeat("─", topFill) + "╮")

	// Content rows.
	// Inner layout: "  " + key + gap + val + trailing = boxW chars
	for _, r := range rows {
		key, val := r[0], r[1]
		gap := maxKey - len(key) + 2
		trailing := boxW - 2 - maxKey - 2 - len(val)
		if trailing < 0 {
			avail := boxW - 4 - maxKey
			if avail > 1 && len(val) > avail {
				val = val[:avail-1] + "…"
			}
			trailing = 0
		}
		if p.color {
			fmt.Fprintf(p.out, "  │  %s%s%s%s%s%s│\n",
				ansiBoldWhite, key, ansiReset,
				strings.Repeat(" ", gap), val,
				strings.Repeat(" ", trailing),
			)
		} else {
			fmt.Fprintf(p.out, "  │  %s%s%s%s│\n",
				key, strings.Repeat(" ", gap), val,
				strings.Repeat(" ", trailing),
			)
		}
	}

	// Bottom border: ╰──...──╯
	p.emitDim("  ╰" + strings.Repeat("─", boxW) + "╯")
}

func (p *Printer) emitDim(s string) {
	if p.color {
		fmt.Fprintln(p.out, ansiDim+s+ansiReset)
	} else {
		fmt.Fprintln(p.out, s)
	}
}

// ── Spinner ──────────────────────────────────────────────────────────────────

// Spinner shows a braille-dot animation while work is in progress.
type Spinner struct {
	p      *Printer
	mu     sync.Mutex
	label  string
	active bool
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewSpinner creates a Spinner attached to p with the given label.
func (p *Printer) NewSpinner(label string) *Spinner {
	return &Spinner{
		p:      p,
		label:  label,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start begins the spinner animation in a background goroutine.
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return
	}
	s.active = true
	s.mu.Unlock()

	go func() {
		defer close(s.doneCh)
		i := 0
		tick := time.NewTicker(80 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-s.stopCh:
				fmt.Fprint(s.p.out, "\r\033[K") // erase spinner line
				return
			case <-tick.C:
				s.mu.Lock()
				lbl := s.label
				s.mu.Unlock()
				frame := spinnerFrames[i%len(spinnerFrames)]
				if s.p.color {
					fmt.Fprintf(s.p.out, "\r  %s%s%s  %s",
						ansiBoldMagenta, frame, ansiReset, lbl)
				} else {
					fmt.Fprintf(s.p.out, "\r  %s  %s", frame, lbl)
				}
				i++
			}
		}
	}()
}

// UpdateLabel changes the spinner label while it is running.
func (s *Spinner) UpdateLabel(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
}

// Stop halts the spinner and prints a final success or error line.
func (s *Spinner) Stop(ok bool, msg string) {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	close(s.stopCh)
	s.mu.Unlock()
	<-s.doneCh
	if ok {
		s.p.Success(msg)
	} else {
		s.p.Error(msg)
	}
}
