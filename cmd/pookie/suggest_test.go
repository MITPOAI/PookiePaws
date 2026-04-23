package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mitpoai/pookiepaws/internal/cli"
)

func TestSuggestCommandMatchesCommonTypos(t *testing.T) {
	tests := []struct {
		input   string
		choices []string
		want    string
	}{
		{input: "reseach", choices: topLevelCommands, want: "research"},
		{input: "docter", choices: topLevelCommands, want: "doctor"},
		{input: "analze", choices: researchSubcommands, want: "analyze"},
	}
	for _, tt := range tests {
		got := suggestCommand(tt.input, tt.choices)
		if got != tt.want {
			t.Fatalf("suggestCommand(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSuggestCommandRejectsDistantStrings(t *testing.T) {
	if got := suggestCommand("zzz", topLevelCommands); got != "" {
		t.Fatalf("suggestCommand returned %q for distant input", got)
	}
}

func TestPrintUsageBodyHighlightsResearchAndCommandSkillSplit(t *testing.T) {
	var buf bytes.Buffer
	printUsageBody(cli.New(&buf))
	text := buf.String()
	if !strings.Contains(text, "research <sub>") {
		t.Fatalf("expected research command in usage, got %q", text)
	}
	if !strings.Contains(text, "pookie research analyze") {
		t.Fatalf("expected research quick-start in usage, got %q", text)
	}
	if !strings.Contains(text, "`pookie list` shows installed skills") {
		t.Fatalf("expected command-vs-skill guidance in usage, got %q", text)
	}
}

func TestPrintResearchUsageIncludesAnalyseAlias(t *testing.T) {
	var buf bytes.Buffer
	printResearchUsage(&buf)
	text := buf.String()
	if !strings.Contains(text, "analyse --company") {
		t.Fatalf("expected analyse alias in research usage, got %q", text)
	}
	if !strings.Contains(text, "Examples:") {
		t.Fatalf("expected examples section in research usage, got %q", text)
	}
}
