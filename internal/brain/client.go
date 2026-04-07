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

// CompleteNative sends a native tool-calling request to the LLM API.
// It implements NativeClient. Local payload/response types are defined inline
// following the same pattern as Complete().
func (c *OpenAICompatibleClient) CompleteNative(ctx context.Context, messages []ChatMessage, tools []ToolDefinition) (NativeCompletionResponse, error) {
	type nativePayload struct {
		Model       string           `json:"model"`
		Messages    []ChatMessage    `json:"messages"`
		Tools       []ToolDefinition `json:"tools,omitempty"`
		ToolChoice  string           `json:"tool_choice,omitempty"`
		Temperature float64          `json:"temperature"`
	}
	type tcFunc struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	type tc struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function tcFunc `json:"function"`
	}
	type respMessage struct {
		Role      string `json:"role"`
		Content   string `json:"content"`
		ToolCalls []tc   `json:"tool_calls,omitempty"`
	}
	type choice struct {
		Message      respMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	}
	type apiResponse struct {
		Model   string   `json:"model"`
		Choices []choice `json:"choices"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	toolChoice := ""
	if len(tools) > 0 {
		toolChoice = "auto"
	}

	body, err := json.Marshal(nativePayload{
		Model:       c.model,
		Messages:    messages,
		Tools:       tools,
		ToolChoice:  toolChoice,
		Temperature: 0.1,
	})
	if err != nil {
		return NativeCompletionResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return NativeCompletionResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return NativeCompletionResponse{}, err
	}
	defer resp.Body.Close()

	var decoded apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return NativeCompletionResponse{}, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		if decoded.Error != nil && decoded.Error.Message != "" {
			return NativeCompletionResponse{}, fmt.Errorf("llm request failed: %s", decoded.Error.Message)
		}
		return NativeCompletionResponse{}, fmt.Errorf("llm request failed with status %d", resp.StatusCode)
	}
	if len(decoded.Choices) == 0 {
		return NativeCompletionResponse{}, fmt.Errorf("llm response contained no choices")
	}

	ch := decoded.Choices[0]
	msg := ChatMessage{
		Role:    ch.Message.Role,
		Content: strings.TrimSpace(ch.Message.Content),
	}
	for _, call := range ch.Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			ID:   call.ID,
			Type: call.Type,
			Function: ToolCallFunc{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}

	return NativeCompletionResponse{
		Message:      msg,
		FinishReason: ch.FinishReason,
		Model:        decoded.Model,
	}, nil
}

var _ CompletionClient = (*OpenAICompatibleClient)(nil)
var _ NativeClient = (*OpenAICompatibleClient)(nil)
