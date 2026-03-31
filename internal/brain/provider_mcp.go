package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/mcp"
)

type MCPProvider struct {
	client        *mcp.Client
	transport     mcp.Transport
	model         string
	method        string
	transportMode string
}

func NewMCPProvider(ctx context.Context, cfg ProviderConfig) (*MCPProvider, error) {
	transport, mode, err := buildMCPTransport(cfg)
	if err != nil {
		return nil, err
	}

	client := mcp.NewClient(transport)
	if err := client.Connect(ctx); err != nil {
		_ = transport.Close()
		return nil, err
	}

	return &MCPProvider{
		client:        client,
		transport:     transport,
		model:         cfg.Model,
		method:        cfg.MCPMethod,
		transportMode: mode,
	}, nil
}

func (p *MCPProvider) Status() Status {
	if p == nil {
		return Status{Enabled: false, Provider: "MCP bridge", Mode: "disabled"}
	}
	return Status{
		Enabled:  true,
		Provider: "MCP bridge",
		Mode:     p.transportMode,
		Model:    p.model,
	}
}

func (p *MCPProvider) Complete(ctx context.Context, request CompletionRequest) (CompletionResponse, error) {
	if p == nil || p.client == nil {
		return CompletionResponse{}, ErrProviderNotConfigured
	}

	params, err := json.Marshal(map[string]any{
		"model": p.model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": request.SystemPrompt,
			},
			{
				"role":    "user",
				"content": request.UserPrompt,
			},
		},
		"temperature": 0.1,
	})
	if err != nil {
		return CompletionResponse{}, err
	}

	requestID := fmt.Sprintf("llm_%d", time.Now().UnixNano())
	if err := p.client.Send(ctx, mcp.JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  p.method,
		Params:  params,
	}); err != nil {
		return CompletionResponse{}, err
	}

	for {
		message, err := p.client.Receive(ctx)
		if err != nil {
			return CompletionResponse{}, err
		}
		if message.Error != nil {
			return CompletionResponse{}, fmt.Errorf("mcp completion failed: %s", message.Error.Message)
		}
		if message.ID != nil && fmt.Sprint(message.ID) != requestID {
			continue
		}
		if len(message.Result) == 0 {
			continue
		}

		text, model, err := decodeMCPCompletionResult(message.Result)
		if err != nil {
			return CompletionResponse{}, err
		}
		if model == "" {
			model = p.model
		}
		return CompletionResponse{
			Raw:        text,
			Model:      model,
			PromptText: request.UserPrompt,
		}, nil
	}
}

func (p *MCPProvider) Close() error {
	if p == nil || p.transport == nil {
		return nil
	}
	return p.transport.Close()
}

func buildMCPTransport(cfg ProviderConfig) (mcp.Transport, string, error) {
	switch cfg.MCPTransport {
	case mcpTransportStdio:
		transport := mcp.NewStdioTransport(cfg.MCPCommand, cfg.MCPArgs...)
		if len(cfg.MCPEnv) > 0 {
			env := make([]string, 0, len(cfg.MCPEnv))
			for key, value := range cfg.MCPEnv {
				env = append(env, key+"="+value)
			}
			transport.Env = env
		}
		return transport, mcpTransportStdio, nil
	case mcpTransportHTTP:
		return mcp.NewStreamableHTTPTransport(cfg.MCPBaseURL, cfg.MCPHeaders), mcpTransportHTTP, nil
	default:
		return nil, "", fmt.Errorf("unsupported llm_mcp_transport %q", cfg.MCPTransport)
	}
}

func decodeMCPCompletionResult(raw json.RawMessage) (string, string, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", err
	}

	model := flattenText(payload["model"])
	candidates := []any{
		payload["output_text"],
		payload["content"],
		payload["text"],
		payload["message"],
		payload["messages"],
		payload["choices"],
	}
	for _, candidate := range candidates {
		if text := strings.TrimSpace(flattenText(candidate)); text != "" {
			return text, model, nil
		}
	}

	return "", model, fmt.Errorf("mcp completion result did not contain text")
}

func flattenText(value any) string {
	switch cast := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(cast)
	case []any:
		parts := make([]string, 0, len(cast))
		for _, item := range cast {
			if text := flattenText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case map[string]any:
		for _, key := range []string{"text", "content", "output_text"} {
			if text := flattenText(cast[key]); text != "" {
				return text
			}
		}
		if message, ok := cast["message"]; ok {
			if text := flattenText(message); text != "" {
				return text
			}
		}
		for _, key := range []string{"choices", "messages", "parts"} {
			if text := flattenText(cast[key]); text != "" {
				return text
			}
		}
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}
