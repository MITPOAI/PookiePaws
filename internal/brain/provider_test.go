package brain

import (
	"context"
	"fmt"
	"testing"
)

type stubSecrets map[string]string

func (s stubSecrets) Get(name string) (string, error) {
	value, ok := s[name]
	if !ok {
		return "", fmt.Errorf("secret %q not found", name)
	}
	return value, nil
}

func (s stubSecrets) RedactMap(payload map[string]any) map[string]any {
	return payload
}

type fakeProvider struct {
	response string
}

func (p fakeProvider) Complete(_ context.Context, request CompletionRequest) (CompletionResponse, error) {
	return CompletionResponse{
		Raw:        p.response,
		Model:      "test-model",
		PromptText: request.UserPrompt,
	}, nil
}

func (p fakeProvider) Status() Status {
	return Status{Enabled: true, Provider: "test", Mode: "test", Model: "test-model"}
}

func (p fakeProvider) Close() error {
	return nil
}

type fakeProviderFactory struct {
	provider Provider
}

func (f fakeProviderFactory) Available() bool {
	return f.provider != nil
}

func (f fakeProviderFactory) Status() Status {
	if f.provider == nil {
		return Status{Enabled: false, Provider: "test", Mode: "disabled"}
	}
	return f.provider.Status()
}

func (f fakeProviderFactory) New(context.Context) (Provider, error) {
	if f.provider == nil {
		return nil, ErrProviderNotConfigured
	}
	return f.provider, nil
}

func TestLoadProviderConfigOpenAICompatible(t *testing.T) {
	cfg, err := LoadProviderConfig(stubSecrets{
		"llm_base_url": "https://api.example.test/v1/chat/completions",
		"llm_model":    "gpt-5.4",
		"llm_api_key":  "secret",
	})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if cfg.Type != providerOpenAICompatible {
		t.Fatalf("unexpected provider type %q", cfg.Type)
	}
	if cfg.Model != "gpt-5.4" {
		t.Fatalf("unexpected model %q", cfg.Model)
	}
}

func TestLoadProviderConfigMCPResolvesSecretReferences(t *testing.T) {
	cfg, err := LoadProviderConfig(stubSecrets{
		"llm_provider":      "mcp",
		"llm_model":         "claude-sonnet-4.6",
		"llm_mcp_transport": "streamable-http",
		"llm_mcp_base_url":  "https://mcp.example.test",
		"llm_mcp_headers":   `{"Authorization":"$secret:llm_api_key"}`,
		"llm_api_key":       "Bearer secret-token",
	})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	if cfg.Type != providerMCP {
		t.Fatalf("unexpected provider type %q", cfg.Type)
	}
	if got := cfg.MCPHeaders["Authorization"]; got != "Bearer secret-token" {
		t.Fatalf("unexpected resolved header %q", got)
	}
}
