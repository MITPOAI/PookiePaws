package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

const (
	providerOpenAICompatible = "openai-compatible"
	providerMCP              = "mcp"
	mcpTransportStdio        = "stdio"
	mcpTransportHTTP         = "streamable-http"
)

type Provider interface {
	CompletionClient
	Status() Status
	Close() error
}

type ProviderFactory interface {
	Available() bool
	Status() Status
	New(ctx context.Context) (Provider, error)
}

type ProviderConfig struct {
	Type         string
	Model        string
	BaseURL      string
	APIKey       string
	MCPTransport string
	MCPCommand   string
	MCPArgs      []string
	MCPEnv       map[string]string
	MCPBaseURL   string
	MCPHeaders   map[string]string
	MCPMethod    string
}

type SecretBackedProviderFactory struct {
	secrets engine.SecretProvider
}

func NewSecretBackedProviderFactory(secrets engine.SecretProvider) *SecretBackedProviderFactory {
	return &SecretBackedProviderFactory{secrets: secrets}
}

func (f *SecretBackedProviderFactory) Available() bool {
	_, err := LoadProviderConfig(f.secrets)
	return err == nil
}

func (f *SecretBackedProviderFactory) Status() Status {
	cfg, err := LoadProviderConfig(f.secrets)
	if err != nil {
		return Status{
			Enabled:  false,
			Provider: "OpenAI-compatible",
			Mode:     "disabled",
		}
	}
	return cfg.Status()
}

func (f *SecretBackedProviderFactory) New(ctx context.Context) (Provider, error) {
	cfg, err := LoadProviderConfig(f.secrets)
	if err != nil {
		return nil, err
	}

	switch cfg.Type {
	case providerOpenAICompatible:
		return NewOpenAICompatibleClientFromConfig(cfg)
	case providerMCP:
		return NewMCPProvider(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported llm provider %q", cfg.Type)
	}
}

func LoadProviderConfig(secrets engine.SecretProvider) (ProviderConfig, error) {
	if secrets == nil {
		return ProviderConfig{}, ErrProviderNotConfigured
	}

	provider := optionalSecret(secrets, "llm_provider")
	if provider == "" {
		provider = providerOpenAICompatible
	}
	provider = strings.ToLower(strings.TrimSpace(provider))

	cfg := ProviderConfig{
		Type:         provider,
		Model:        optionalSecret(secrets, "llm_model"),
		BaseURL:      optionalSecret(secrets, "llm_base_url"),
		APIKey:       optionalSecret(secrets, "llm_api_key"),
		MCPTransport: strings.ToLower(optionalSecret(secrets, "llm_mcp_transport")),
		MCPCommand:   optionalSecret(secrets, "llm_mcp_command"),
		MCPBaseURL:   optionalSecret(secrets, "llm_mcp_base_url"),
		MCPMethod:    optionalSecret(secrets, "llm_mcp_method"),
	}
	if cfg.MCPMethod == "" {
		cfg.MCPMethod = "llm.complete"
	}

	var err error
	if cfg.MCPArgs, err = parseStringList(optionalSecret(secrets, "llm_mcp_args")); err != nil {
		return ProviderConfig{}, fmt.Errorf("parse llm_mcp_args: %w", err)
	}
	if cfg.MCPEnv, err = parseStringMap(optionalSecret(secrets, "llm_mcp_env")); err != nil {
		return ProviderConfig{}, fmt.Errorf("parse llm_mcp_env: %w", err)
	}
	if cfg.MCPHeaders, err = parseStringMap(optionalSecret(secrets, "llm_mcp_headers")); err != nil {
		return ProviderConfig{}, fmt.Errorf("parse llm_mcp_headers: %w", err)
	}
	if cfg.MCPEnv, err = resolveSecretMap(cfg.MCPEnv, secrets); err != nil {
		return ProviderConfig{}, fmt.Errorf("resolve llm_mcp_env: %w", err)
	}
	if cfg.MCPHeaders, err = resolveSecretMap(cfg.MCPHeaders, secrets); err != nil {
		return ProviderConfig{}, fmt.Errorf("resolve llm_mcp_headers: %w", err)
	}

	switch cfg.Type {
	case providerOpenAICompatible:
		if cfg.BaseURL == "" || cfg.Model == "" {
			return ProviderConfig{}, ErrProviderNotConfigured
		}
	case providerMCP:
		if cfg.Model == "" {
			return ProviderConfig{}, ErrProviderNotConfigured
		}
		if cfg.MCPTransport == "" {
			cfg.MCPTransport = mcpTransportHTTP
		}
		switch cfg.MCPTransport {
		case mcpTransportStdio:
			if cfg.MCPCommand == "" {
				return ProviderConfig{}, ErrProviderNotConfigured
			}
		case mcpTransportHTTP:
			if cfg.MCPBaseURL == "" {
				if cfg.BaseURL == "" {
					return ProviderConfig{}, ErrProviderNotConfigured
				}
				cfg.MCPBaseURL = cfg.BaseURL
			}
		default:
			return ProviderConfig{}, fmt.Errorf("unsupported llm_mcp_transport %q", cfg.MCPTransport)
		}
	default:
		return ProviderConfig{}, fmt.Errorf("unsupported llm provider %q", cfg.Type)
	}

	return cfg, nil
}

func (c ProviderConfig) Status() Status {
	switch c.Type {
	case providerMCP:
		mode := c.MCPTransport
		if mode == "" {
			mode = "bridge"
		}
		return Status{
			Enabled:  true,
			Provider: "MCP bridge",
			Mode:     mode,
			Model:    c.Model,
		}
	default:
		return Status{
			Enabled:  true,
			Provider: "OpenAI-compatible",
			Mode:     inferProviderMode(c.BaseURL),
			Model:    c.Model,
		}
	}
}

func optionalSecret(secrets engine.SecretProvider, key string) string {
	value, err := secrets.Get(key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func parseStringList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "[") {
		var items []string
		if err := json.Unmarshal([]byte(raw), &items); err != nil {
			return nil, err
		}
		return items, nil
	}

	parts := strings.Split(raw, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items, nil
}

func parseStringMap(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}, nil
	}

	parsed := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func resolveSecretMap(values map[string]string, secrets engine.SecretProvider) (map[string]string, error) {
	if len(values) == 0 {
		return values, nil
	}

	resolved := make(map[string]string, len(values))
	for key, value := range values {
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "$secret:") {
			secretName := strings.TrimSpace(strings.TrimPrefix(value, "$secret:"))
			if secretName == "" {
				return nil, fmt.Errorf("empty secret reference for %q", key)
			}
			secretValue, err := secrets.Get(secretName)
			if err != nil {
				return nil, err
			}
			resolved[key] = secretValue
			continue
		}
		resolved[key] = value
	}
	return resolved, nil
}

func inferProviderMode(baseURL string) string {
	baseURL = strings.ToLower(strings.TrimSpace(baseURL))
	switch {
	case baseURL == "":
		return "disabled"
	case strings.Contains(baseURL, "127.0.0.1"), strings.Contains(baseURL, "localhost"), strings.Contains(baseURL, "[::1]"):
		return "local"
	default:
		return "hosted"
	}
}
