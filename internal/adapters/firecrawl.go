package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/conv"
	"github.com/mitpoai/pookiepaws/internal/engine"
)

// FirecrawlAdapter provides web scraping via the Firecrawl API with an
// automatic fallback to Jina Reader when no Firecrawl key is configured.
// It implements engine.MarketingChannel as a "research" kind adapter.
type FirecrawlAdapter struct {
	client *http.Client
}

var _ engine.MarketingChannel = (*FirecrawlAdapter)(nil)

// NewFirecrawlAdapter creates a FirecrawlAdapter with a standard HTTP client.
func NewFirecrawlAdapter() *FirecrawlAdapter {
	return &FirecrawlAdapter{
		client: newAdapterClient(),
	}
}

func (a *FirecrawlAdapter) Name() string {
	return "firecrawl"
}

func (a *FirecrawlAdapter) Kind() string {
	return "research"
}

func (a *FirecrawlAdapter) SecretKeys() []string {
	return []string{"firecrawl_api_key", "jina_api_key"}
}

func (a *FirecrawlAdapter) Status(secrets engine.SecretProvider) engine.ChannelProviderStatus {
	status := engine.ChannelProviderStatus{
		Provider:     a.Name(),
		Channel:      a.Kind(),
		Configured:   false,
		Healthy:      false,
		Capabilities: []string{"scrape"},
		Message:      "Firecrawl is not configured - provide firecrawl_api_key or jina_api_key.",
	}

	fcKey, _ := secrets.Get("firecrawl_api_key")
	jinaKey, _ := secrets.Get("jina_api_key")

	if strings.TrimSpace(fcKey) != "" {
		status.Configured = true
		status.Healthy = true
		status.Message = "Firecrawl API key configured."
		return status
	}
	if strings.TrimSpace(jinaKey) != "" {
		status.Configured = true
		status.Healthy = true
		status.Message = "Jina API key configured (fallback mode)."
		return status
	}

	return status
}

func (a *FirecrawlAdapter) Test(ctx context.Context, secrets engine.SecretProvider) (engine.ChannelProviderStatus, error) {
	status := a.Status(secrets)
	if !status.Configured {
		return status, fmt.Errorf(status.Message)
	}

	fcKey, _ := secrets.Get("firecrawl_api_key")
	if strings.TrimSpace(fcKey) != "" {
		return a.testFirecrawl(ctx, fcKey, status)
	}
	return a.testJina(ctx, secrets, status)
}

func (a *FirecrawlAdapter) testFirecrawl(ctx context.Context, apiKey string, status engine.ChannelProviderStatus) (engine.ChannelProviderStatus, error) {
	body := map[string]any{
		"url":     "https://example.com",
		"formats": []string{"markdown"},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return status, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.firecrawl.dev/v1/scrape", bytes.NewReader(payload))
	if err != nil {
		return status, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		status.Healthy = false
		status.Message = err.Error()
		return status, err
	}
	defer resp.Body.Close()

	_, err = readAdapterResponse(resp, "firecrawl")
	if err != nil {
		status.Healthy = false
		status.Message = err.Error()
		return status, err
	}

	status.Healthy = true
	status.Message = "Firecrawl API responded successfully."
	return status, nil
}

func (a *FirecrawlAdapter) testJina(ctx context.Context, secrets engine.SecretProvider, status engine.ChannelProviderStatus) (engine.ChannelProviderStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://r.jina.ai/https://example.com", nil)
	if err != nil {
		return status, err
	}
	req.Header.Set("Accept", "text/markdown")

	jinaKey, _ := secrets.Get("jina_api_key")
	if strings.TrimSpace(jinaKey) != "" {
		req.Header.Set("Authorization", "Bearer "+jinaKey)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		status.Healthy = false
		status.Message = err.Error()
		return status, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		status.Healthy = false
		status.Message = fmt.Sprintf("Jina API returned status %d", resp.StatusCode)
		return status, fmt.Errorf(status.Message)
	}

	status.Healthy = true
	status.Message = "Jina Reader API responded successfully."
	return status, nil
}

func (a *FirecrawlAdapter) Execute(ctx context.Context, action engine.AdapterAction, secrets engine.SecretProvider) (engine.AdapterResult, error) {
	if action.Operation != "scrape" {
		return engine.AdapterResult{}, fmt.Errorf("unsupported research operation %q", action.Operation)
	}

	rawURL := strings.TrimSpace(conv.AsString(action.Payload["url"]))
	if rawURL == "" {
		return engine.AdapterResult{}, fmt.Errorf("firecrawl scrape requires a url")
	}
	if err := validateScrapeURL(rawURL); err != nil {
		return engine.AdapterResult{}, err
	}

	fcKey, _ := secrets.Get("firecrawl_api_key")
	if strings.TrimSpace(fcKey) != "" {
		return a.scrapeFirecrawl(ctx, rawURL, fcKey)
	}
	return a.scrapeJina(ctx, rawURL, secrets)
}

func (a *FirecrawlAdapter) scrapeFirecrawl(ctx context.Context, targetURL, apiKey string) (engine.AdapterResult, error) {
	body := map[string]any{
		"url":     targetURL,
		"formats": []string{"markdown"},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return engine.AdapterResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.firecrawl.dev/v1/scrape", bytes.NewReader(payload))
	if err != nil {
		return engine.AdapterResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return engine.AdapterResult{}, err
	}
	defer resp.Body.Close()

	decoded, err := readAdapterResponse(resp, "firecrawl")
	if err != nil {
		return engine.AdapterResult{}, err
	}

	markdown := extractFirecrawlMarkdown(decoded)

	return engine.AdapterResult{
		Adapter:   a.Name(),
		Operation: "scrape",
		Status:    "scraped",
		Details: map[string]any{
			"markdown": markdown,
			"url":      targetURL,
		},
	}, nil
}

func (a *FirecrawlAdapter) scrapeJina(ctx context.Context, targetURL string, secrets engine.SecretProvider) (engine.AdapterResult, error) {
	jinaURL := "https://r.jina.ai/" + targetURL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jinaURL, nil)
	if err != nil {
		return engine.AdapterResult{}, err
	}
	req.Header.Set("Accept", "text/markdown")

	jinaKey, _ := secrets.Get("jina_api_key")
	if strings.TrimSpace(jinaKey) != "" {
		req.Header.Set("Authorization", "Bearer "+jinaKey)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return engine.AdapterResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return engine.AdapterResult{}, fmt.Errorf("jina API failed with status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return engine.AdapterResult{}, err
	}

	return engine.AdapterResult{
		Adapter:   a.Name(),
		Operation: "scrape",
		Status:    "scraped",
		Details: map[string]any{
			"markdown": string(raw),
			"url":      targetURL,
		},
	}, nil
}

// extractFirecrawlMarkdown navigates the Firecrawl JSON response to pull
// the markdown content from data.markdown.
func extractFirecrawlMarkdown(decoded map[string]any) string {
	if decoded == nil {
		return ""
	}
	data, ok := decoded["data"].(map[string]any)
	if !ok {
		return ""
	}
	return conv.AsString(data["markdown"])
}

// validateScrapeURL checks that the URL uses http or https and does not
// point to localhost or loopback addresses.
func validateScrapeURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("url scheme must be http or https, got %q", scheme)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0" {
		return fmt.Errorf("localhost urls are not allowed")
	}
	return nil
}
