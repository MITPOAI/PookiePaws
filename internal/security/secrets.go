package security

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type JSONSecretProvider struct {
	path   string
	mu     sync.RWMutex
	values map[string]string
}

var _ engine.SecretProvider = (*JSONSecretProvider)(nil)

func NewJSONSecretProvider(runtimeRoot string) (*JSONSecretProvider, error) {
	path := filepath.Join(runtimeRoot, ".security.json")
	provider := &JSONSecretProvider{
		path:   path,
		values: map[string]string{},
	}

	if data, err := os.ReadFile(path); err == nil {
		if len(data) > 0 {
			data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
			if err := json.Unmarshal(data, &provider.values); err != nil {
				return nil, err
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	return provider, nil
}

func (p *JSONSecretProvider) Get(name string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	value, ok := p.values[name]
	if !ok {
		return "", fmt.Errorf("secret %q not found", name)
	}
	return value, nil
}

func (p *JSONSecretProvider) Update(values map[string]string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	next := make(map[string]string, len(p.values))
	for key, value := range p.values {
		next[key] = value
	}
	for key, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			delete(next, key)
			continue
		}
		next[key] = value
	}

	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return err
	}

	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, p.path); err != nil {
		return err
	}
	p.values = next
	return nil
}

func (p *JSONSecretProvider) RedactMap(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	redacted := make(map[string]any, len(payload))
	for key, value := range payload {
		lower := strings.ToLower(key)
		switch {
		case strings.Contains(lower, "secret"),
			strings.Contains(lower, "token"),
			strings.Contains(lower, "key"),
			strings.Contains(lower, "password"):
			redacted[key] = "[REDACTED]"
		default:
			redacted[key] = value
		}
	}
	return redacted
}
