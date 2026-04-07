package brain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

var ErrProviderNotConfigured = errors.New("llm provider not configured")

type OpenAICompatibleClient struct {
	baseURL    string
	model      string
	apiKey     string
	httpClient *http.Client
}

func NewOpenAICompatibleClient(secrets engine.SecretProvider) (*OpenAICompatibleClient, error) {
	cfg, err := LoadProviderConfig(secrets)
	if err != nil {
		return nil, err
	}
	if cfg.Type != providerOpenAICompatible {
		return nil, ErrProviderNotConfigured
	}
	return NewOpenAICompatibleClientFromConfig(cfg)
}

func NewOpenAICompatibleClientFromConfig(cfg ProviderConfig) (*OpenAICompatibleClient, error) {
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.BaseURL == "" || cfg.Model == "" {
		return nil, ErrProviderNotConfigured
	}

	return &OpenAICompatibleClient{
		baseURL: cfg.BaseURL,
		model:   cfg.Model,
		apiKey:  strings.TrimSpace(cfg.APIKey),
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

func (c *OpenAICompatibleClient) Status() Status {
	if c == nil {
		return Status{Enabled: false, Provider: "OpenAI-compatible", Mode: "disabled"}
	}

	return Status{
		Enabled:  true,
		Provider: "OpenAI-compatible",
		Mode:     inferProviderMode(c.baseURL),
		Model:    c.model,
	}
}

func (c *OpenAICompatibleClient) Complete(ctx context.Context, request CompletionRequest) (CompletionResponse, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type payload struct {
		Model       string    `json:"model"`
		Messages    []message `json:"messages"`
		Temperature float64   `json:"temperature"`
	}
	type choice struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	type response struct {
		Model   string   `json:"model"`
		Choices []choice `json:"choices"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	body, err := json.Marshal(payload{
		Model: c.model,
		Messages: []message{
			{Role: "system", Content: request.SystemPrompt},
			{Role: "user", Content: request.UserPrompt},
		},
		Temperature: 0.1,
	})
	if err != nil {
		return CompletionResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return CompletionResponse{}, err
	}
	defer resp.Body.Close()

	var decoded response
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return CompletionResponse{}, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		if decoded.Error != nil && decoded.Error.Message != "" {
			return CompletionResponse{}, fmt.Errorf("llm request failed: %s", decoded.Error.Message)
		}
		return CompletionResponse{}, fmt.Errorf("llm request failed with status %d", resp.StatusCode)
	}
	if len(decoded.Choices) == 0 {
		return CompletionResponse{}, fmt.Errorf("llm response contained no choices")
	}

	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		return CompletionResponse{}, fmt.Errorf("llm response content was empty")
	}

	return CompletionResponse{
		Raw:        content,
		Model:      decoded.Model,
		PromptText: request.UserPrompt,
	}, nil
}

func isLocalURL(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(value, "127.0.0.1") || strings.Contains(value, "localhost")
}

func (c *OpenAICompatibleClient) Close() error {
	return nil
}
