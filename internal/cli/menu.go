package cli

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
	menuItems := make([]MenuItem, 0, len(items))
	for _, item := range items {
		menuItems = append(menuItems, MenuItem{Label: item})
	}
	index, _ := NewWizard(p).Select(title, "", menuItems, len(items)-1)
	return index
}
