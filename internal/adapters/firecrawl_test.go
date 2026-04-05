package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

// redirectTransport rewrites every outgoing request to point at the given
// test server while preserving the original path and headers. This lets
// tests intercept calls to hard-coded third-party URLs (Firecrawl, Jina).
func redirectTransport(server *httptest.Server) http.RoundTripper {
	target, _ := url.Parse(server.URL)
	return &rewriteTransport{target: target, base: server.Client().Transport}
}

type rewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	return t.base.RoundTrip(req)
}

func TestFirecrawlAdapterScrapeFirecrawlPrimary(t *testing.T) {
	var seenAuth string
	var seenBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"# Hello World\nSome content."}}`))
	}))
	defer server.Close()

	adapter := NewFirecrawlAdapter()
	adapter.client = &http.Client{Transport: redirectTransport(server)}

	result, err := adapter.Execute(context.Background(), engine.AdapterAction{
		Adapter:   "firecrawl",
		Operation: "scrape",
		Payload: map[string]any{
			"url": "https://example.com/test-page",
		},
	}, stubSecrets{
		"firecrawl_api_key": "fc-test-key",
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if seenAuth != "Bearer fc-test-key" {
		t.Fatalf("expected bearer auth header, got %q", seenAuth)
	}
	if result.Status != "scraped" {
		t.Fatalf("expected status scraped, got %q", result.Status)
	}
	md, _ := result.Details["markdown"].(string)
	if !strings.Contains(md, "Hello World") {
		t.Fatalf("expected markdown content, got %q", md)
	}
	gotURL, _ := result.Details["url"].(string)
	if gotURL != "https://example.com/test-page" {
		t.Fatalf("expected url in details, got %q", gotURL)
	}

	// Verify request body contained the target URL.
	if seenBody["url"] != "https://example.com/test-page" {
		t.Fatalf("expected url in request body, got %v", seenBody["url"])
	}

	// Test extractFirecrawlMarkdown parsing.
	decoded := map[string]any{
		"success": true,
		"data": map[string]any{
			"markdown": "# Parsed Content",
		},
	}
	if extractFirecrawlMarkdown(decoded) != "# Parsed Content" {
		t.Fatal("expected parsed markdown from nested structure")
	}
	if extractFirecrawlMarkdown(nil) != "" {
		t.Fatal("expected empty string for nil decoded")
	}
	if extractFirecrawlMarkdown(map[string]any{"data": "not-a-map"}) != "" {
		t.Fatal("expected empty string for non-map data")
	}
}

func TestFirecrawlAdapterScrapeJinaFallback(t *testing.T) {
	var seenAccept string
	var seenAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAccept = r.Header.Get("Accept")
		seenAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# Jina Scraped\nFallback content."))
	}))
	defer server.Close()

	adapter := NewFirecrawlAdapter()
	adapter.client = &http.Client{Transport: redirectTransport(server)}

	secrets := stubSecrets{
		"jina_api_key": "jina-test-key",
	}

	// Verify Status reports Jina fallback mode.
	status := adapter.Status(secrets)
	if !status.Configured {
		t.Fatal("expected configured status with jina key")
	}
	if !status.Healthy {
		t.Fatal("expected healthy status with jina key")
	}
	if !strings.Contains(status.Message, "Jina") {
		t.Fatalf("expected Jina in message, got %q", status.Message)
	}

	// Execute the scrape via the Jina fallback (no firecrawl_api_key present).
	result, err := adapter.Execute(context.Background(), engine.AdapterAction{
		Adapter:   "firecrawl",
		Operation: "scrape",
		Payload: map[string]any{
			"url": "https://example.com/some-page",
		},
	}, secrets)
	if err != nil {
		t.Fatalf("execute jina fallback failed: %v", err)
	}
	if result.Status != "scraped" {
		t.Fatalf("expected status scraped, got %q", result.Status)
	}
	md, ok := result.Details["markdown"].(string)
	if !ok || !strings.Contains(md, "Jina Scraped") {
		t.Fatalf("expected jina markdown content, got %q", md)
	}
	gotURL, _ := result.Details["url"].(string)
	if gotURL != "https://example.com/some-page" {
		t.Fatalf("expected url in details, got %q", gotURL)
	}
	if seenAccept != "text/markdown" {
		t.Fatalf("expected Accept: text/markdown, got %q", seenAccept)
	}
	if seenAuth != "Bearer jina-test-key" {
		t.Fatalf("expected jina bearer auth, got %q", seenAuth)
	}
}

func TestFirecrawlAdapterMissingURL(t *testing.T) {
	adapter := NewFirecrawlAdapter()

	_, err := adapter.Execute(context.Background(), engine.AdapterAction{
		Adapter:   "firecrawl",
		Operation: "scrape",
		Payload:   map[string]any{},
	}, stubSecrets{
		"firecrawl_api_key": "fc-key",
	})
	if err == nil {
		t.Fatal("expected error for missing url")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Fatalf("expected url-related error, got %q", err.Error())
	}
}

func TestFirecrawlAdapterEmptySecretsNotConfigured(t *testing.T) {
	adapter := NewFirecrawlAdapter()

	status := adapter.Status(stubSecrets{})
	if status.Configured {
		t.Fatal("expected not configured with empty secrets")
	}
	if status.Healthy {
		t.Fatal("expected not healthy with empty secrets")
	}
}

func TestFirecrawlAdapterUnsupportedOperation(t *testing.T) {
	adapter := NewFirecrawlAdapter()

	_, err := adapter.Execute(context.Background(), engine.AdapterAction{
		Adapter:   "firecrawl",
		Operation: "crawl",
		Payload:   map[string]any{"url": "https://example.com"},
	}, stubSecrets{
		"firecrawl_api_key": "fc-key",
	})
	if err == nil {
		t.Fatal("expected error for unsupported operation")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported error, got %q", err.Error())
	}
}

func TestFirecrawlAdapterValidateScrapeURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "valid https", url: "https://example.com", wantErr: false},
		{name: "valid http", url: "http://example.com/page", wantErr: false},
		{name: "localhost rejected", url: "http://localhost:8080/path", wantErr: true},
		{name: "loopback rejected", url: "http://127.0.0.1/path", wantErr: true},
		{name: "ftp rejected", url: "ftp://example.com/file", wantErr: true},
		{name: "no scheme rejected", url: "example.com", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateScrapeURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateScrapeURL(%q): got err=%v, wantErr=%v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestFirecrawlAdapterTestFirecrawlPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fc-test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"# Test"}}`))
	}))
	defer server.Close()

	adapter := NewFirecrawlAdapter()
	adapter.client = &http.Client{Transport: redirectTransport(server)}

	secrets := stubSecrets{
		"firecrawl_api_key": "fc-test-key",
	}
	result, err := adapter.Test(context.Background(), secrets)
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
	if !result.Healthy {
		t.Fatalf("expected healthy, got message: %q", result.Message)
	}
	if !strings.Contains(result.Message, "Firecrawl") {
		t.Fatalf("expected Firecrawl in message, got %q", result.Message)
	}
}

func TestFirecrawlAdapterTestJinaPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/markdown" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# Example"))
	}))
	defer server.Close()

	adapter := NewFirecrawlAdapter()
	adapter.client = &http.Client{Transport: redirectTransport(server)}

	secrets := stubSecrets{"jina_api_key": "jina-key"}
	result, err := adapter.Test(context.Background(), secrets)
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
	if !result.Healthy {
		t.Fatalf("expected healthy, got message: %q", result.Message)
	}
	if !strings.Contains(result.Message, "Jina") {
		t.Fatalf("expected Jina in message, got %q", result.Message)
	}
}

func TestFirecrawlAdapterNameAndKind(t *testing.T) {
	adapter := NewFirecrawlAdapter()

	if adapter.Name() != "firecrawl" {
		t.Fatalf("expected name firecrawl, got %q", adapter.Name())
	}
	if adapter.Kind() != "research" {
		t.Fatalf("expected kind research, got %q", adapter.Kind())
	}

	keys := adapter.SecretKeys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 secret keys, got %d", len(keys))
	}
	if keys[0] != "firecrawl_api_key" || keys[1] != "jina_api_key" {
		t.Fatalf("unexpected secret keys: %v", keys)
	}
}

func TestFirecrawlAdapterScrapeFirecrawlViaServer(t *testing.T) {
	var seenAuth string
	var seenBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"# Server Content\nParagraph."}}`))
	}))
	defer server.Close()

	adapter := NewFirecrawlAdapter()
	adapter.client = &http.Client{Transport: redirectTransport(server)}

	result, err := adapter.scrapeFirecrawl(context.Background(), "https://example.com/article", "fc-direct-key")
	if err != nil {
		t.Fatalf("scrapeFirecrawl failed: %v", err)
	}
	if seenAuth != "Bearer fc-direct-key" {
		t.Fatalf("expected bearer auth, got %q", seenAuth)
	}
	md, _ := result.Details["markdown"].(string)
	if !strings.Contains(md, "Server Content") {
		t.Fatalf("expected markdown content, got %q", md)
	}
	if result.Status != "scraped" {
		t.Fatalf("expected status scraped, got %q", result.Status)
	}
}
