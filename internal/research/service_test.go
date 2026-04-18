package research

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

type stubSecrets map[string]string

func (s stubSecrets) Get(name string) (string, error) {
	return s[name], nil
}

func (s stubSecrets) RedactMap(payload map[string]any) map[string]any {
	return payload
}

func TestServiceAnalyzeLiveBoundedSelection(t *testing.T) {
	firecrawl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/search" {
			http.NotFound(w, r)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		query, _ := payload["query"].(string)
		if strings.Contains(query, "about") {
			http.Error(w, "temporary failure", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"web": []map[string]any{
					{
						"title":       "OpenClaw Pricing",
						"description": "Premium plans for operators",
						"url":         "https://openclaw.example/pricing",
						"markdown":    "# Pricing\nPremium plan with operator controls.",
					},
					{
						"title":       "OpenClaw FAQ",
						"description": "Common questions",
						"url":         "https://openclaw.example/faq",
						"markdown":    "# FAQ\nAnswers for operator workflows.",
					},
					{
						"title":       "OpenClaw Collections",
						"description": "Offer bundles",
						"url":         "https://openclaw.example/collections",
						"markdown":    "",
					},
					{
						"title":       "OpenClaw Login",
						"description": "Login",
						"url":         "https://openclaw.example/login",
						"markdown":    "# Login",
					},
					{
						"title":       "Third-party review",
						"description": "Review commentary",
						"url":         "https://review.example/openclaw-review",
						"markdown":    "# Review\nCommentary.",
					},
				},
			},
		})
	}))
	defer firecrawl.Close()

	jina := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# Collections\nBundle offer structure with curated add-ons."))
	}))
	defer jina.Close()

	service := &Service{
		client:           firecrawl.Client(),
		firecrawlBaseURL: firecrawl.URL,
		jinaBaseURL:      jina.URL,
	}

	result, err := service.Analyze(context.Background(), AnalyzeRequest{
		Company:     "PookiePaws Reserve",
		Competitors: []string{"OpenClaw"},
		Market:      "AU pet gifting",
		FocusAreas:  []string{"pricing", "positioning", "offer structure"},
	}, stubSecrets{"firecrawl_api_key": "fc-test"})
	if err != nil {
		t.Fatalf("analyze live: %v", err)
	}
	if result.Coverage.Mode != "live" {
		t.Fatalf("expected live mode, got %+v", result.Coverage)
	}
	if result.Coverage.Kept == 0 || result.Coverage.Kept > 6 {
		t.Fatalf("expected bounded kept count, got %+v", result.Coverage)
	}
	if result.Coverage.Scraped == 0 {
		t.Fatalf("expected scraped pages, got %+v", result.Coverage)
	}
	hostCount := map[string]int{}
	for _, source := range result.Sources {
		hostCount[source.Host]++
		if hostCount[source.Host] > 2 {
			t.Fatalf("expected host cap of 2, got %v", hostCount)
		}
		if source.Markdown != "" {
			t.Fatalf("expected raw markdown to be omitted by default")
		}
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected partial-failure warnings, got none")
	}
	if len(result.CompetitorNotes) == 0 {
		t.Fatalf("expected competitor notes, got %+v", result)
	}
}

func TestServiceAnalyzeSeedDomainsFallback(t *testing.T) {
	jina := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# Home\nPositioning for premium pet gifting."))
	}))
	defer jina.Close()

	service := &Service{
		client:           jina.Client(),
		firecrawlBaseURL: "http://unused.example",
		jinaBaseURL:      jina.URL,
	}

	result, err := service.Analyze(context.Background(), AnalyzeRequest{
		Company:    "PookiePaws Reserve",
		Domains:    []string{"openclaw.test"},
		FocusAreas: []string{"positioning"},
	}, stubSecrets{})
	if err != nil {
		t.Fatalf("analyze fallback: %v", err)
	}
	if result.Coverage.Mode != "seed_domains" {
		t.Fatalf("expected seed-domain mode, got %+v", result.Coverage)
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected degraded-coverage warning")
	}
}

func TestServiceAnalyzeRequiresConfig(t *testing.T) {
	service := &Service{
		client:           &http.Client{},
		firecrawlBaseURL: "http://unused.example",
		jinaBaseURL:      "http://unused.example",
	}
	_, err := service.Analyze(context.Background(), AnalyzeRequest{
		Company: "PookiePaws Reserve",
	}, stubSecrets{})
	if err == nil {
		t.Fatal("expected configuration error")
	}
}

func TestServiceAnalyzeInternalProviderPreferred(t *testing.T) {
	search := `
<html><body>
  <a href="https://openclaw.example/pricing">OpenClaw Pricing</a>
  <a href="https://openclaw.example/about">OpenClaw About</a>
</body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(search))
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
		case "/pricing":
			_, _ = w.Write([]byte("<html><title>Pricing</title><body>Premium gifting bundles and pricing.</body></html>"))
		case "/about":
			_, _ = w.Write([]byte("<html><title>About</title><body>Positioning for premium pet gifting.</body></html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := &Service{
		client:        &http.Client{Transport: redirectTransport(server)},
		searchBaseURL: server.URL + "/search",
	}

	result, err := service.Analyze(context.Background(), AnalyzeRequest{
		Company:     "PookiePaws Reserve",
		Competitors: []string{"OpenClaw"},
		Market:      "AU pet gifting",
	}, stubSecrets{})
	if err != nil {
		t.Fatalf("analyze internal: %v", err)
	}
	if result.Provider != researchProviderInternal {
		t.Fatalf("expected internal provider, got %q", result.Provider)
	}
	if result.Coverage.Mode != "internal" {
		t.Fatalf("expected internal coverage mode, got %+v", result.Coverage)
	}
	if result.FallbackReason != "" {
		t.Fatalf("expected no fallback reason, got %q", result.FallbackReason)
	}
	if result.Coverage.Kept == 0 {
		t.Fatalf("expected internal sources, got %+v", result.Coverage)
	}
}

func TestServiceAnalyzeInternalFallbacksToFirecrawl(t *testing.T) {
	firecrawlCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body><a href="https://openclaw.example/pricing">OpenClaw Pricing</a></body></html>`))
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
		case "/pricing":
			http.Error(w, "temporary failure", http.StatusBadGateway)
		case "/v2/search":
			firecrawlCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"web": []map[string]any{
						{
							"title":       "OpenClaw Pricing",
							"description": "Premium plans for operators",
							"url":         "https://openclaw.example/pricing",
							"markdown":    "# Pricing\nPremium plan with operator controls.",
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := &Service{
		client:           &http.Client{Transport: redirectTransport(server)},
		firecrawlBaseURL: server.URL,
		searchBaseURL:    server.URL + "/search",
	}

	result, err := service.Analyze(context.Background(), AnalyzeRequest{
		Company:     "PookiePaws Reserve",
		Competitors: []string{"OpenClaw"},
		Market:      "AU pet gifting",
	}, stubSecrets{"firecrawl_api_key": "fc-test"})
	if err != nil {
		t.Fatalf("analyze with fallback: %v", err)
	}
	if firecrawlCalls == 0 {
		t.Fatal("expected firecrawl fallback to be used")
	}
	if result.Provider != researchProviderFirecrawl {
		t.Fatalf("expected firecrawl provider, got %q", result.Provider)
	}
	if result.FallbackReason == "" {
		t.Fatal("expected fallback reason to be populated")
	}
}

func TestServiceAnalyzeExplicitInternalFailsClosed(t *testing.T) {
	firecrawlCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body><a href="https://openclaw.example/pricing">OpenClaw Pricing</a></body></html>`))
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
		case "/pricing":
			http.Error(w, "temporary failure", http.StatusBadGateway)
		case "/v2/search":
			firecrawlCalls++
			http.Error(w, "should not be called", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := &Service{
		client:           &http.Client{Transport: redirectTransport(server)},
		firecrawlBaseURL: server.URL,
		searchBaseURL:    server.URL + "/search",
	}

	_, err := service.Analyze(context.Background(), AnalyzeRequest{
		Company:     "PookiePaws Reserve",
		Competitors: []string{"OpenClaw"},
		Market:      "AU pet gifting",
		Provider:    researchProviderInternal,
	}, stubSecrets{"firecrawl_api_key": "fc-test"})
	if err == nil {
		t.Fatal("expected explicit internal mode to fail without fallback")
	}
	if firecrawlCalls != 0 {
		t.Fatalf("expected no firecrawl fallback call, got %d", firecrawlCalls)
	}
}

func TestValidatePublicURLRejectsPrivateTargets(t *testing.T) {
	cases := []string{
		"http://localhost:8080",
		"http://127.0.0.1/path",
		"http://10.0.0.8/path",
		"http://192.168.0.1/path",
	}
	for _, rawURL := range cases {
		if err := validatePublicURL(rawURL); err == nil {
			t.Fatalf("expected %s to be rejected", rawURL)
		}
	}
}

var _ engine.SecretProvider = stubSecrets{}

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
