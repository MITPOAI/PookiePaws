package cli

import (
	"fmt"
	"os"
)

// RunMenu displays an interactive arrow-key selection menu and returns the
// zero-based index of the chosen item. The caller is responsible for printing
// any banner or preamble before invoking this function.
//
// Controls:
//
//	Up / Down arrow  — move selection
//	Enter            — confirm
//	q / Ctrl+C       — select the last item (conventionally "Exit")
func RunMenu(p *Printer, title string, items []string) int {
	if len(items) == 0 {
		return -1
	}

	// If stdin is not a terminal (piped input), fall back to the last option.
	state, err := enableRawMode()
	if err != nil {
		return len(items) - 1
	}
	defer restoreMode(state)

	cursor := 0
	draw := func() {
		// Move to column 1 and clear from the cursor down, then redraw.
		// We print (1 title line + len(items) lines), so move up that many
		// lines before redrawing — except on the first draw when there is
		// nothing to erase yet.
		if title != "" {
			p.Accent("%s", title)
		}
		p.Blank()
		for i, item := range items {
			if i == cursor {
				fmt.Fprintf(p.out, "  %s  > %s%s\n",
					ansiBoldMagenta, item, ansiReset)
			} else {
				fmt.Fprintf(p.out, "      %s\n", item)
			}
		}
	}

	// Number of lines that draw() outputs (title + blank + items).
	lineCount := len(items)
	if title != "" {
		lineCount += 2
	}

	// Clear the drawn block by moving up and erasing lines.
	clear := func() {
		for i := 0; i < lineCount; i++ {
			fmt.Fprint(p.out, "\033[A") // move up one line
			fmt.Fprint(p.out, "\033[2K") // erase entire line
		}
	}

	draw()

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return len(items) - 1
		}

		switch {
		case n == 1 && (buf[0] == '\r' || buf[0] == '\n'):
			// Enter — confirm selection.
			return cursor

		case n == 1 && (buf[0] == 'q' || buf[0] == 3): // 3 = Ctrl+C
			// Select the last item (Exit).
			clear()
			cursor = len(items) - 1
			draw()
			return cursor

		case n == 3 && buf[0] == 0x1b && buf[1] == '[':
			prev := cursor
			switch buf[2] {
			case 'A': // Up arrow
				if cursor > 0 {
					cursor--
				}
			case 'B': // Down arrow
				if cursor < len(items)-1 {
					cursor++
				}
			}
			if cursor != prev {
				clear()
				draw()
			}
		}
	}
}
