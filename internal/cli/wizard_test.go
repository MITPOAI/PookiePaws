package cli

import "testing"

func TestMenuModelMoveClamps(t *testing.T) {
	model := newMenuModel([]MenuItem{
		{Label: "one"},
		{Label: "two"},
		{Label: "three"},
	})

	model.Move(1)
	if model.Cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", model.Cursor)
	}

	model.Move(99)
	if model.Cursor != 2 {
		t.Fatalf("expected cursor to clamp to 2, got %d", model.Cursor)
	}

	model.Move(-99)
	if model.Cursor != 0 {
		t.Fatalf("expected cursor to clamp to 0, got %d", model.Cursor)
	}
}

func TestCheckboxModelToggleCurrentItem(t *testing.T) {
	model := newCheckboxModel([]CheckboxItem{
		{Label: "a"},
		{Label: "b"},
	})

	model.Move(1)
	model.Toggle()
	if !model.Items[1].Checked {
		t.Fatalf("expected second item to be checked")
	}
	if model.Items[0].Checked {
		t.Fatalf("expected first item to remain unchecked")
	}

	model.Toggle()
	if model.Items[1].Checked {
		t.Fatalf("expected second item to be unchecked after second toggle")
	}
}

func TestReadKeyParsesSequences(t *testing.T) {
	tests := []struct {
		name string
		buf  []byte
		want keyKind
	}{
		{name: "enter", buf: []byte{'\n'}, want: keyEnter},
		{name: "space", buf: []byte{' '}, want: keySpace},
		{name: "escape", buf: []byte{0x1b}, want: keyEscape},
		{name: "ctrlc", buf: []byte{3}, want: keyCtrlC},
		{name: "up", buf: []byte{0x1b, '[', 'A'}, want: keyUp},
		{name: "down", buf: []byte{0x1b, '[', 'B'}, want: keyDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyKey(tt.buf)
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
