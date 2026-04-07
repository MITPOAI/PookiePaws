package brain

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultJinaBase = "https://r.jina.ai"

// JinaScraperTool fetches a URL via the Jina AI Reader API and returns
// clean Markdown content. It replaces WebSearchTool in the native tool path.
type JinaScraperTool struct {
	// jinaBase overrides the Jina base URL; zero value uses defaultJinaBase.
	// This field exists for test injection only.
	jinaBase string
}

var _ Tool = (*JinaScraperTool)(nil)

func (t *JinaScraperTool) Name() string { return "web_search" }
func (t *JinaScraperTool) Description() string {
	return "Fetch a public URL and return clean Markdown content for research. Use for competitor pages, pricing, or industry data. Never guess URLs — look them up."
}
func (t *JinaScraperTool) ParameterSchema() string {
	return `{"url": "string (required) - the public HTTPS URL to fetch"}`
}

func (t *JinaScraperTool) Definition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: JSONSchema{
				Type: "object",
				Properties: map[string]SchemaProperty{
					"url": {Type: "string", Description: "The public HTTPS URL to fetch and read"},
				},
				Required: []string{"url"},
			},
		},
	}
}

func (t *JinaScraperTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	rawURL := strings.TrimSpace(asString(input["url"]))
	if rawURL == "" {
		return nil, fmt.Errorf("url is required")
	}

	base := t.jinaBase
	if base == "" {
		base = defaultJinaBase
	}
	jinaURL := base + "/" + rawURL

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jinaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "text/markdown")
	req.Header.Set("User-Agent", "PookiePaws/1.1 (marketing-research-bot)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jina fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("jina returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 500*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	content := strings.TrimSpace(string(body))
	if len(content) > 8000 {
		content = content[:8000] + "\n...(truncated)"
	}

	return map[string]any{
		"url":     rawURL,
		"content": content,
	}, nil
}
