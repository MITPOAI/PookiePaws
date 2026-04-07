package brain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ProviderFailureCode string

const (
	ProviderFailureNone             ProviderFailureCode = ""
	ProviderFailureNotConfigured    ProviderFailureCode = "not_configured"
	ProviderFailureUnsupported      ProviderFailureCode = "unsupported_provider"
	ProviderFailureEndpointRejected ProviderFailureCode = "endpoint_rejected"
	ProviderFailureEndpointUnusable ProviderFailureCode = "endpoint_unreachable"
	ProviderFailureCredentials      ProviderFailureCode = "credentials_rejected"
	ProviderFailureModel            ProviderFailureCode = "model_rejected"
	ProviderFailureProviderResponse ProviderFailureCode = "provider_error"
)

type ProviderHealth struct {
	ConfigPresent       bool                `json:"config_present"`
	Provider            string              `json:"provider"`
	Mode                string              `json:"mode"`
	Model               string              `json:"model,omitempty"`
	BaseURL             string              `json:"base_url,omitempty"`
	EndpointReachable   bool                `json:"endpoint_reachable"`
	CredentialsAccepted bool                `json:"credentials_accepted"`
	ModelAccepted       bool                `json:"model_accepted"`
	FailureCode         ProviderFailureCode `json:"failure_code,omitempty"`
	Detail              string              `json:"detail,omitempty"`
	Error               string              `json:"error,omitempty"`
	CheckedAt           time.Time           `json:"checked_at"`
}

func (h ProviderHealth) Healthy() bool {
	return h.ConfigPresent && h.EndpointReachable && h.CredentialsAccepted && h.ModelAccepted
}

func CheckProviderHealth(ctx context.Context, secrets SecretReader) ProviderHealth {
	cfg, err := LoadProviderConfig(secrets)
	if err != nil {
		return ProviderHealth{
			ConfigPresent: false,
			Provider:      "OpenAI-compatible",
			Mode:          "disabled",
			FailureCode:   ProviderFailureNotConfigured,
			Detail:        "Provider configuration is incomplete.",
			Error:         err.Error(),
			CheckedAt:     time.Now().UTC(),
		}
	}
	return CheckProviderConfig(ctx, cfg, nil)
}

func CheckProviderConfig(ctx context.Context, cfg ProviderConfig, client *http.Client) ProviderHealth {
	status := cfg.Status()
	health := ProviderHealth{
		ConfigPresent: true,
		Provider:      status.Provider,
		Mode:          status.Mode,
		Model:         strings.TrimSpace(cfg.Model),
		BaseURL:       strings.TrimSpace(cfg.BaseURL),
		CheckedAt:     time.Now().UTC(),
	}

	switch cfg.Type {
	case providerOpenAICompatible:
		return checkOpenAICompatibleHealth(ctx, cfg, client, health)
	case providerMCP:
		health.FailureCode = ProviderFailureUnsupported
		health.Detail = "MCP provider validation is not yet available in doctor or smoke."
		health.Error = "provider health checks currently support OpenAI-compatible providers only"
		return health
	default:
		health.FailureCode = ProviderFailureUnsupported
		health.Detail = "Unsupported provider type."
		health.Error = fmt.Sprintf("unsupported provider %q", cfg.Type)
		return health
	}
}

func checkOpenAICompatibleHealth(ctx context.Context, cfg ProviderConfig, client *http.Client, health ProviderHealth) ProviderHealth {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Model) == "" {
		health.ConfigPresent = false
		health.FailureCode = ProviderFailureNotConfigured
		health.Detail = "OpenAI-compatible providers require both a chat-completions endpoint and a model ID."
		health.Error = ErrProviderNotConfigured.Error()
		return health
	}

	payload := map[string]any{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "respond with ok"},
			{"role": "user", "content": "ping"},
		},
		"max_tokens": 1,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		health.FailureCode = ProviderFailureProviderResponse
		health.Detail = "Could not prepare the provider health request."
		health.Error = err.Error()
		return health
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL, bytes.NewReader(body))
	if err != nil {
		health.FailureCode = ProviderFailureEndpointUnusable
		health.Detail = "The configured endpoint URL is invalid."
		health.Error = err.Error()
		return health
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey := strings.TrimSpace(cfg.APIKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("x-api-key", apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		health.FailureCode = ProviderFailureEndpointUnusable
		health.Detail = "The provider endpoint could not be reached."
		health.Error = err.Error()
		return health
	}
	defer resp.Body.Close()

	health.EndpointReachable = true
	rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	message := strings.TrimSpace(decodeProviderHealthError(rawBody))

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		health.FailureCode = ProviderFailureCredentials
		health.Detail = "The provider rejected the configured credentials."
		health.Error = firstNonEmpty(message, http.StatusText(resp.StatusCode))
		return health
	case resp.StatusCode == http.StatusNotFound:
		health.FailureCode = ProviderFailureEndpointRejected
		health.Detail = "The configured chat-completions endpoint was not found."
		health.Error = firstNonEmpty(message, http.StatusText(resp.StatusCode))
		return health
	case resp.StatusCode >= http.StatusBadRequest:
		health.CredentialsAccepted = true
		if looksLikeInvalidModel(message) {
			health.FailureCode = ProviderFailureModel
			health.Detail = "The provider is reachable, but the selected model ID was rejected."
			health.Error = firstNonEmpty(message, "invalid model")
			return health
		}
		health.FailureCode = ProviderFailureProviderResponse
		health.Detail = "The provider responded, but it did not accept the minimal completion request."
		health.Error = firstNonEmpty(message, fmt.Sprintf("HTTP %d", resp.StatusCode))
		return health
	default:
		health.CredentialsAccepted = true
		health.ModelAccepted = true
		health.Detail = "The provider accepted a minimal completion request for the configured model."
		return health
	}
}

func decodeProviderHealthError(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return ""
	}

	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if value := strings.TrimSpace(payload.Error.Message); value != "" {
			return value
		}
		if value := strings.TrimSpace(payload.Message); value != "" {
			return value
		}
	}
	return strings.TrimSpace(string(body))
}

func looksLikeInvalidModel(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(message, "invalid model"),
		strings.Contains(message, "model not found"),
		strings.Contains(message, "unknown model"),
		strings.Contains(message, "unsupported model"),
		strings.Contains(message, "not a valid model id"),
		strings.Contains(message, "model does not exist"):
		return true
	default:
		return false
	}
}
