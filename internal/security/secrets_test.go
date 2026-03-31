package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONSecretProviderAcceptsUTF8BOM(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".security.json")
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"llm_model":"gpt-oss:20b"}`)...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write secret file: %v", err)
	}

	provider, err := NewJSONSecretProvider(root)
	if err != nil {
		t.Fatalf("create secret provider: %v", err)
	}
	value, err := provider.Get("llm_model")
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if value != "gpt-oss:20b" {
		t.Fatalf("unexpected secret value %q", value)
	}
}

func TestJSONSecretProviderUpdatePersistsValues(t *testing.T) {
	root := t.TempDir()
	provider, err := NewJSONSecretProvider(root)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	if err := provider.Update(map[string]string{
		"llm_base_url": "http://localhost:11434/v1/chat/completions",
		"llm_model":    "gpt-oss:20b",
	}); err != nil {
		t.Fatalf("update provider: %v", err)
	}

	reloaded, err := NewJSONSecretProvider(root)
	if err != nil {
		t.Fatalf("reload provider: %v", err)
	}
	value, err := reloaded.Get("llm_model")
	if err != nil {
		t.Fatalf("get reloaded value: %v", err)
	}
	if value != "gpt-oss:20b" {
		t.Fatalf("unexpected persisted value %q", value)
	}
}
