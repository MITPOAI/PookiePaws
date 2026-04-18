package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintJSONIndents(t *testing.T) {
	var buf bytes.Buffer
	err := printJSON(&buf, map[string]any{"k": "v", "n": 1})
	if err != nil {
		t.Fatalf("printJSON: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"k": "v"`) {
		t.Errorf("missing key/value: %q", out)
	}
	if !strings.Contains(out, "  ") {
		t.Errorf("expected indentation: %q", out)
	}
}
