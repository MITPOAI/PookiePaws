package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/brain"
)

type ConnectivityChecker struct {
	client *http.Client
}

type ConnectivityResult struct {
	Endpoint string
	Message  string
}

func NewConnectivityChecker(client *http.Client) *ConnectivityChecker {
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &ConnectivityChecker{client: client}
}

func (c *ConnectivityChecker) Check(ctx context.Context, preset ProviderPreset, model, apiKey string) (ConnectivityResult, error) {
	if strings.TrimSpace(preset.BaseURL) == "" {
		return ConnectivityResult{}, fmt.Errorf("%s is missing a base URL", preset.Label)
	}
	if preset.RequiresAPIKey && strings.TrimSpace(apiKey) == "" {
		return ConnectivityResult{Endpoint: preset.BaseURL}, fmt.Errorf("%s requires an API key before it can be verified", preset.Label)
	}

	switch preset.CheckMode {
	case CheckModeChatPing:
		return c.chatPing(ctx, preset, model, apiKey)
	case CheckModeListModels:
		return c.listModels(ctx, preset, model, apiKey)
	default:
		return ConnectivityResult{Endpoint: preset.BaseURL}, fmt.Errorf("unsupported connectivity check mode %q", preset.CheckMode)
	}
}

func (c *ConnectivityChecker) listModels(ctx context.Context, preset ProviderPreset, model, apiKey string) (ConnectivityResult, error) {
	endpoint := ModelsEndpoint(preset.BaseURL)
	if endpoint == "" {
		return ConnectivityResult{}, fmt.Errorf("could not derive a models endpoint from %s", preset.BaseURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ConnectivityResult{Endpoint: endpoint}, err
	}
	applyProviderHeaders(req, preset, apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return ConnectivityResult{Endpoint: endpoint}, fmt.Errorf("could not reach %s: %w", preset.Label, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return ConnectivityResult{Endpoint: endpoint}, providerHTTPError(preset, resp)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ConnectivityResult{Endpoint: endpoint}, fmt.Errorf("%s returned an unexpected models response", preset.Label)
	}

	message := fmt.Sprintf("%s responded successfully.", preset.Label)
	if count := len(payload.Data); count > 0 {
		message = fmt.Sprintf("%s returned %d model(s).", preset.Label, count)
	}
	result, err := c.chatPing(ctx, preset, model, apiKey)
	if err != nil {
		return ConnectivityResult{Endpoint: endpoint}, err
	}
	return ConnectivityResult{
		Endpoint: result.Endpoint,
		Message:  message + " " + result.Message,
	}, nil
}

func (c *ConnectivityChecker) chatPing(ctx context.Context, preset ProviderPreset, model, apiKey string) (ConnectivityResult, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ConnectivityResult{Endpoint: preset.BaseURL}, fmt.Errorf("%s requires a model before it can be verified", preset.Label)
	}
	health := brain.CheckProviderConfig(ctx, brain.ProviderConfig{
		Type:    "openai-compatible",
		Model:   model,
		BaseURL: preset.BaseURL,
		APIKey:  apiKey,
	}, c.client)
	if !health.Healthy() {
		return ConnectivityResult{Endpoint: health.BaseURL}, fmt.Errorf("%s", c.healthErrorMessage(preset, health))
	}
	return ConnectivityResult{Endpoint: health.BaseURL, Message: fmt.Sprintf("%s accepted a minimal chat request for %s.", preset.Label, model)}, nil
}

func (c *ConnectivityChecker) healthErrorMessage(preset ProviderPreset, health brain.ProviderHealth) string {
	switch health.FailureCode {
	case brain.ProviderFailureCredentials:
		return fmt.Sprintf("%s rejected the credentials: %s", preset.Label, health.Error)
	case brain.ProviderFailureModel:
		return fmt.Sprintf("%s rejected the selected model %q: %s", preset.Label, strings.TrimSpace(health.Model), health.Error)
	case brain.ProviderFailureEndpointRejected:
		return fmt.Sprintf("%s could not find that endpoint: %s", preset.Label, health.Error)
	case brain.ProviderFailureEndpointUnusable:
		return fmt.Sprintf("could not reach %s: %s", preset.Label, health.Error)
	case brain.ProviderFailureNotConfigured:
		return fmt.Sprintf("%s is not fully configured: %s", preset.Label, health.Error)
	default:
		message := strings.TrimSpace(health.Error)
		if message == "" {
			message = strings.TrimSpace(health.Detail)
		}
		if message == "" {
			message = "unknown error"
		}
		return fmt.Sprintf("%s returned an error: %s", preset.Label, message)
	}
}

func applyProviderHeaders(req *http.Request, preset ProviderPreset, apiKey string) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if preset.ID == "anthropic" && apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
}

func providerHTTPError(preset ProviderPreset, resp *http.Response) error {
	message := http.StatusText(resp.StatusCode)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err == nil {
		if decoded := decodeProviderError(body); decoded != "" {
			message = decoded
		}
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%s rejected the credentials: %s", preset.Label, message)
	case http.StatusNotFound:
		return fmt.Errorf("%s could not find that endpoint: %s", preset.Label, message)
	default:
		return fmt.Errorf("%s returned HTTP %d: %s", preset.Label, resp.StatusCode, message)
	}
}

func decodeProviderError(body []byte) string {
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
		if strings.TrimSpace(payload.Error.Message) != "" {
			return strings.TrimSpace(payload.Error.Message)
		}
		if strings.TrimSpace(payload.Message) != "" {
			return strings.TrimSpace(payload.Message)
		}
	}

	text := strings.TrimSpace(string(body))
	if len(text) > 200 {
		return text[:200]
	}
	return text
}
