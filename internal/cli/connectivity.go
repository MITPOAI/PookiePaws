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
		return c.listModels(ctx, preset, apiKey)
	default:
		return ConnectivityResult{Endpoint: preset.BaseURL}, fmt.Errorf("unsupported connectivity check mode %q", preset.CheckMode)
	}
}

func (c *ConnectivityChecker) listModels(ctx context.Context, preset ProviderPreset, apiKey string) (ConnectivityResult, error) {
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
	return ConnectivityResult{
		Endpoint: endpoint,
		Message:  message,
	}, nil
}

func (c *ConnectivityChecker) chatPing(ctx context.Context, preset ProviderPreset, model, apiKey string) (ConnectivityResult, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ConnectivityResult{Endpoint: preset.BaseURL}, fmt.Errorf("%s requires a model before it can be verified", preset.Label)
	}

	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
		"max_tokens": 1,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ConnectivityResult{Endpoint: preset.BaseURL}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, preset.BaseURL, bytes.NewReader(body))
	if err != nil {
		return ConnectivityResult{Endpoint: preset.BaseURL}, err
	}
	req.Header.Set("Content-Type", "application/json")
	applyProviderHeaders(req, preset, apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return ConnectivityResult{Endpoint: preset.BaseURL}, fmt.Errorf("could not reach %s: %w", preset.Label, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return ConnectivityResult{Endpoint: preset.BaseURL}, providerHTTPError(preset, resp)
	}

	return ConnectivityResult{
		Endpoint: preset.BaseURL,
		Message:  fmt.Sprintf("%s accepted a minimal chat request.", preset.Label),
	}, nil
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
