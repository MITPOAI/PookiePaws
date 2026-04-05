package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

type MenuItem struct {
	Label string
	Hint  string
}

type CheckboxItem struct {
	Label   string
	Hint    string
	Checked bool
}

type Wizard struct {
	p *Printer
}

type keyKind int

const (
	keyUnknown keyKind = iota
	keyEnter
	keyUp
	keyDown
	keySpace
	keyEscape
	keyCtrlC
)

type menuModel struct {
	Items  []MenuItem
	Cursor int
}

type checkboxModel struct {
	Items  []CheckboxItem
	Cursor int
}

func NewWizard(p *Printer) *Wizard {
	if p == nil {
		p = Stdout()
	}
	return &Wizard{p: p}
}

func InteractiveAvailable() bool {
	return isTerminal(os.Stdin) && isTerminal(os.Stdout)
}

func (w *Wizard) Splash(title string, detail []string) {
	w.p.Banner()
	if strings.TrimSpace(title) != "" {
		w.p.Accent("%s", title)
	}
	for _, line := range detail {
		if strings.TrimSpace(line) == "" {
			w.p.Blank()
			continue
		}
		w.p.Dim("%s", line)
	}
	if len(detail) > 0 {
		w.p.Blank()
	}
}

func (w *Wizard) Select(title string, help string, items []MenuItem, fallback int) (int, bool) {
	if len(items) == 0 {
		return -1, false
	}
	if fallback < 0 || fallback >= len(items) {
		fallback = len(items) - 1
	}
	if !InteractiveAvailable() {
		return fallback, true
	}

	state, err := enableRawMode()
	if err != nil {
		return fallback, true
	}
	defer restoreMode(state)

	model := newMenuModel(items)
	lines := 0
	draw := func() {
		clearDrawnBlock(w.p.out, lines)
		lines = drawMenuFrame(w.p, title, help, model)
	}

	draw()
	for {
		key, readErr := readKey()
		if readErr != nil {
			return fallback, true
		}

		switch key {
		case keyEnter:
			return model.Cursor, true
		case keyCtrlC:
			return fallback, false
		case keyEscape:
			return fallback, false
		case keyUp:
			model.Move(-1)
			draw()
		case keyDown:
			model.Move(1)
			draw()
		}
	}
}

func (w *Wizard) MultiSelect(title string, help string, items []CheckboxItem) ([]CheckboxItem, bool) {
	if len(items) == 0 {
		return nil, true
	}
	if !InteractiveAvailable() {
		return items, true
	}

	state, err := enableRawMode()
	if err != nil {
		return items, true
	}
	defer restoreMode(state)

	model := newCheckboxModel(items)
	lines := 0
	draw := func() {
		clearDrawnBlock(w.p.out, lines)
		lines = drawCheckboxFrame(w.p, title, help, model)
	}

	draw()
	for {
		key, readErr := readKey()
		if readErr != nil {
			return items, true
		}

		switch key {
		case keyEnter:
			return model.Items, true
		case keyCtrlC, keyEscape:
			return model.Items, false
		case keyUp:
			model.Move(-1)
			draw()
		case keyDown:
			model.Move(1)
			draw()
		case keySpace:
			model.Toggle()
			draw()
		}
	}
}

func newMenuModel(items []MenuItem) menuModel {
	return menuModel{Items: append([]MenuItem(nil), items...)}
}

func (m *menuModel) Move(delta int) {
	if len(m.Items) == 0 {
		m.Cursor = 0
		return
	}
	next := m.Cursor + delta
	switch {
	case next < 0:
		m.Cursor = 0
	case next >= len(m.Items):
		m.Cursor = len(m.Items) - 1
	default:
		m.Cursor = next
	}
}

func newCheckboxModel(items []CheckboxItem) checkboxModel {
	return checkboxModel{Items: append([]CheckboxItem(nil), items...)}
}

func (m *checkboxModel) Move(delta int) {
	if len(m.Items) == 0 {
		m.Cursor = 0
		return
	}
	next := m.Cursor + delta
	switch {
	case next < 0:
		m.Cursor = 0
	case next >= len(m.Items):
		m.Cursor = len(m.Items) - 1
	default:
		m.Cursor = next
	}
}

func (m *checkboxModel) Toggle() {
	if len(m.Items) == 0 || m.Cursor < 0 || m.Cursor >= len(m.Items) {
		return
	}
	m.Items[m.Cursor].Checked = !m.Items[m.Cursor].Checked
}

func drawMenuFrame(p *Printer, title string, help string, model menuModel) int {
	lines := 0
	if strings.TrimSpace(title) != "" {
		p.Accent("%s", title)
		lines++
	}
	if strings.TrimSpace(help) != "" {
		p.Dim("%s", help)
		lines++
	}
	p.Blank()
	lines++
	for index, item := range model.Items {
		prefix := "[ ]"
		labelCode := ""
		if index == model.Cursor {
			prefix = "[›]"
			labelCode = ansiSelection
		}
		if p.color && labelCode != "" {
			fmt.Fprintf(p.out, "  %s%s%s %s%s%s\n", ansiSelection, prefix, ansiReset, labelCode, item.Label, ansiReset)
		} else {
			fmt.Fprintf(p.out, "  %s %s\n", prefix, item.Label)
		}
		lines++
		if strings.TrimSpace(item.Hint) != "" {
			p.Dim("    %s", item.Hint)
			lines++
		}
	}
	p.Blank()
	lines++
	p.Dim("[↑/↓] Move  [Enter] Confirm  [Esc] Back  [Ctrl+C] Exit")
	lines++
	return lines
}

func drawCheckboxFrame(p *Printer, title string, help string, model checkboxModel) int {
	lines := 0
	if strings.TrimSpace(title) != "" {
		p.Accent("%s", title)
		lines++
	}
	if strings.TrimSpace(help) != "" {
		p.Dim("%s", help)
		lines++
	}
	p.Blank()
	lines++
	for index, item := range model.Items {
		check := "[ ]"
		if item.Checked {
			check = "[x]"
		}
		cursor := "   "
		if index == model.Cursor {
			cursor = "[›]"
		}
		if p.color && index == model.Cursor {
			fmt.Fprintf(p.out, "  %s%s%s %s %s%s%s\n", ansiSelection, cursor, ansiReset, check, ansiSelection, item.Label, ansiReset)
		} else {
			fmt.Fprintf(p.out, "  %s %s %s\n", cursor, check, item.Label)
		}
		lines++
		if strings.TrimSpace(item.Hint) != "" {
			p.Dim("      %s", item.Hint)
			lines++
		}
	}
	p.Blank()
	lines++
	p.Dim("[↑/↓] Move  [Space] Toggle  [Enter] Confirm  [Esc] Back  [Ctrl+C] Exit")
	lines++
	return lines
}

func clearDrawnBlock(out io.Writer, lines int) {
	if lines <= 0 {
		return
	}
	for index := 0; index < lines; index++ {
		fmt.Fprint(out, "\033[A")
		fmt.Fprint(out, "\033[2K")
	}
}

func readKey() (keyKind, error) {
	buf := make([]byte, 8)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		return keyUnknown, err
	}
	return classifyKey(buf[:n]), nil
}

func classifyKey(buf []byte) keyKind {
	if len(buf) == 0 {
		return keyUnknown
	}

	switch {
	case len(buf) == 1 && (buf[0] == '\r' || buf[0] == '\n'):
		return keyEnter
	case len(buf) == 1 && buf[0] == ' ':
		return keySpace
	case len(buf) == 1 && buf[0] == 0x1b:
		return keyEscape
	case len(buf) == 1 && buf[0] == 3:
		return keyCtrlC
	case len(buf) >= 3 && buf[0] == 0x1b && buf[1] == '[':
		switch buf[2] {
		case 'A':
			return keyUp
		case 'B':
			return keyDown
		}
	}
	return keyUnknown
}

// isTerminal is defined in rawmode_windows.go and rawmode_unix.go using
// platform-specific detection (GetConsoleMode on Windows, ModeCharDevice
// on Unix). This ensures InteractiveAvailable() works correctly on all
// platforms.
