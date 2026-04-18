package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRunScenarioSmokeCreatesArtifact(t *testing.T) {
	env, err := newSmokeEnvironment()
	if err != nil {
		t.Fatalf("new smoke environment: %v", err)
	}
	defer env.Close()

	checks, result, ok := runScenarioSmoke(env.runtimeRoot)
	if !ok {
		t.Fatalf("expected scenario smoke to pass, got checks=%+v result=%+v", checks, result)
	}
	if result == nil || result.ArtifactPath == "" {
		t.Fatalf("expected artifact path, got %+v", result)
	}
	if _, err := os.Stat(result.ArtifactPath); err != nil {
		t.Fatalf("expected artifact to exist: %v", err)
	}
}

func TestExecuteSmokeReportAllIncludesScenario(t *testing.T) {
	env, err := newSmokeEnvironment()
	if err != nil {
		t.Fatalf("new smoke environment: %v", err)
	}
	defer env.Close()

	report := executeSmokeReport(env.runtimeRoot, smokeOptions{all: true})
	if !report.Passed {
		t.Fatalf("expected full smoke report to pass, got %+v", report)
	}
	if report.Scenario == nil {
		t.Fatalf("expected scenario metadata in full smoke report")
	}
	if report.ArtifactPath == "" {
		t.Fatalf("expected artifact path in full smoke report")
	}
}

func TestRunScenarioLiveSmokeCreatesArtifact(t *testing.T) {
	env, err := newSmokeEnvironment()
	if err != nil {
		t.Fatalf("new smoke environment: %v", err)
	}
	defer env.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/search":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"web": []map[string]any{
						{
							"title":       "OpenClaw Pricing",
							"description": "Operator pricing",
							"url":         "https://openclaw.example/pricing",
							"markdown":    "# Pricing\nPremium operator plan.",
						},
						{
							"title":       "OpenClaw Offers",
							"description": "Offer structure",
							"url":         "https://openclaw.example/offers",
							"markdown":    "# Offers\nBundle offer structure.",
						},
					},
				},
			})
		default:
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write([]byte("# Mock page"))
		}
	}))
	defer server.Close()

	t.Setenv("POOKIEPAWS_FIRECRAWL_BASE_URL", server.URL)
	t.Setenv("POOKIEPAWS_JINA_BASE_URL", server.URL)

	if err := saveSecurityConfig(env.runtimeRoot, filepath.Join(env.runtimeRoot, ".security.json"), map[string]string{
		"llm_provider":      "openai-compatible",
		"llm_base_url":      env.server.URL + "/v1/chat/completions",
		"llm_model":         "mock-model",
		"llm_api_key":       "sk-mock",
		"firecrawl_api_key": "fc-test",
	}); err != nil {
		t.Fatalf("save security config: %v", err)
	}

	checks, result, ok := runScenarioLiveSmoke(env.runtimeRoot)
	if !ok {
		t.Fatalf("expected live scenario smoke to pass, got checks=%+v result=%+v", checks, result)
	}
	if result == nil || result.ArtifactPath == "" {
		t.Fatalf("expected live artifact path, got %+v", result)
	}
	if result.Mode != "live" {
		t.Fatalf("expected live mode, got %+v", result)
	}
	if result.SourceCount == 0 {
		t.Fatalf("expected live source count, got %+v", result)
	}
	if _, err := os.Stat(result.ArtifactPath); err != nil {
		t.Fatalf("expected artifact to exist: %v", err)
	}
}
