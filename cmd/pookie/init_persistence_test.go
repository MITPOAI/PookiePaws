package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveSecretsPathUsesHomeOverride(t *testing.T) {
	root, secretsPath, err := resolveSecretsPath(filepath.Join("C:", "tmp", "pookie-home"))
	if err != nil {
		t.Fatalf("resolveSecretsPath: %v", err)
	}
	if root != filepath.Join("C:", "tmp", "pookie-home") {
		t.Fatalf("unexpected root %q", root)
	}
	if secretsPath != filepath.Join("C:", "tmp", "pookie-home", ".security.json") {
		t.Fatalf("unexpected secrets path %q", secretsPath)
	}
}

func TestSaveSecurityConfig(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".pookiepaws")
	secretsPath := filepath.Join(root, ".security.json")

	payload := map[string]string{
		"llm_provider": "openai-compatible",
		"llm_model":    "gpt-5.1",
	}

	if err := saveSecurityConfig(root, secretsPath, payload); err != nil {
		t.Fatalf("saveSecurityConfig: %v", err)
	}

	data, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if decoded["llm_model"] != "gpt-5.1" {
		t.Fatalf("expected llm_model to be persisted")
	}

	info, err := os.Stat(secretsPath)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("expected mode 0600, got %o", got)
		}
	}
}
