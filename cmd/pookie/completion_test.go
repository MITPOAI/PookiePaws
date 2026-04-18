package main

import (
	"strings"
	"testing"
)

func TestCompletionScriptsContainKeyCommands(t *testing.T) {
	scripts := map[string]string{
		"bash":       bashCompletion,
		"zsh":        zshCompletion,
		"fish":       fishCompletion,
		"powershell": powershellCompletion,
	}
	// Every shell's script must mention the top-level commands and the
	// research subcommand families. This catches drift if commands are
	// added without updating completion.
	mustContain := []string{
		"start", "status", "research", "watchlists", "dossier", "recommendations",
		"completion", "version", "doctor",
	}
	for shell, script := range scripts {
		for _, tok := range mustContain {
			if !strings.Contains(script, tok) {
				t.Errorf("%s completion missing %q", shell, tok)
			}
		}
	}
}

func TestCompletionScriptsHaveShellMarkers(t *testing.T) {
	if !strings.Contains(bashCompletion, "complete -F _pookie pookie") {
		t.Error("bash script missing complete registration")
	}
	if !strings.Contains(zshCompletion, "compdef _pookie pookie") {
		t.Error("zsh script missing compdef")
	}
	if !strings.Contains(fishCompletion, "__fish_use_subcommand") {
		t.Error("fish script missing fish helpers")
	}
	if !strings.Contains(powershellCompletion, "Register-ArgumentCompleter") {
		t.Error("powershell script missing Register-ArgumentCompleter")
	}
}
