package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type promptActionKind int

const (
	promptActionContinue promptActionKind = iota
	promptActionSubmit
	promptActionCancel
	promptActionAbort
)

type promptBuffer struct {
	value []byte
}

func (w *Wizard) PromptSecret(label, hint string, hasCurrent bool) (string, bool) {
	return w.promptInput(label, hint, hasCurrent, true)
}

func (w *Wizard) promptInput(label, hint string, hasCurrent bool, secret bool) (string, bool) {
	if w == nil {
		w = NewWizard(nil)
	}

	if strings.TrimSpace(label) != "" {
		w.p.Accent("%s", label)
	}
	if strings.TrimSpace(hint) != "" {
		w.p.Dim("%s", hint)
	}
	if hasCurrent {
		w.p.Dim("Press Enter on an empty line to keep the current value.")
	}
	fmt.Fprint(w.p.out, "  > ")

	if !InteractiveAvailable() {
		return readPromptFallback(secret)
	}

	state, err := enableRawMode()
	if err != nil {
		return readPromptFallback(secret)
	}
	defer restoreMode(state)

	buffer := promptBuffer{}
	for {
		chunk, err := readInputChunk()
		if err != nil {
			fmt.Fprintln(w.p.out)
			return "", false
		}

		action := buffer.ApplyChunk(chunk)
		switch action {
		case promptActionAbort, promptActionCancel:
			fmt.Fprintln(w.p.out)
			return "", false
		case promptActionSubmit:
			fmt.Fprintln(w.p.out)
			return strings.TrimSpace(buffer.String()), true
		default:
			rendered := buffer.String()
			if secret {
				rendered = strings.Repeat("*", len(buffer.value))
			}
			fmt.Fprintf(w.p.out, "\r\033[2K  > %s", rendered)
		}
	}
}

func readPromptFallback(secret bool) (string, bool) {
	if secret {
		value, err := ReadSecret()
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(value), true
	}

	reader := bufio.NewReader(os.Stdin)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func readInputChunk() ([]byte, error) {
	buf := make([]byte, 64)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (b *promptBuffer) ApplyChunk(chunk []byte) promptActionKind {
	if len(chunk) == 0 {
		return promptActionContinue
	}
	if isEscapeSequence(chunk) {
		return promptActionContinue
	}

	for index := 0; index < len(chunk); index++ {
		switch chunk[index] {
		case 3:
			return promptActionAbort
		case '\r', '\n':
			return promptActionSubmit
		case 0x1b:
			if len(chunk) == 1 {
				return promptActionCancel
			}
			if isEscapeSequence(chunk[index:]) {
				return promptActionContinue
			}
		case 0x08, 0x7f:
			if len(b.value) > 0 {
				b.value = b.value[:len(b.value)-1]
			}
		default:
			if chunk[index] >= 32 && chunk[index] != 127 {
				b.value = append(b.value, chunk[index])
			}
		}
	}
	return promptActionContinue
}

func (b *promptBuffer) String() string {
	return string(b.value)
}

func isEscapeSequence(chunk []byte) bool {
	if len(chunk) < 2 || chunk[0] != 0x1b {
		return false
	}
	switch chunk[1] {
	case '[',
		'O':
		return true
	default:
		return false
	}
}
