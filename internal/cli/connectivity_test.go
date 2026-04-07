package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConnectivityCheckerListModels(t *testing.T) {
	var gotAuth string
	var gotPaths []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		gotAuth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "gpt-5.1"}},
			})
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": "gpt-5.1",
				"choices": []map[string]any{
					{"message": map[string]string{"content": "ok"}},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	checker := NewConnectivityChecker(server.Client())
	preset := ProviderPreset{
		ID:             "openai",
		Label:          "OpenAI",
		BaseURL:        server.URL + "/v1/chat/completions",
		RequiresAPIKey: true,
		CheckMode:      CheckModeListModels,
	}

	result, err := checker.Check(context.Background(), preset, "gpt-5.1", "sk-test")
	if err != nil {
		t.Fatalf("connectivity check failed: %v", err)
	}
	if len(gotPaths) != 2 || gotPaths[0] != "/v1/models" || gotPaths[1] != "/v1/chat/completions" {
		t.Fatalf("unexpected request sequence: %#v", gotPaths)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("expected bearer auth, got %q", gotAuth)
	}
	if result.Endpoint != server.URL+"/v1/chat/completions" {
		t.Fatalf("unexpected endpoint %q", result.Endpoint)
	}
}

func TestConnectivityCheckerChatPing(t *testing.T) {
	var gotAuth string
	var gotAPIKey string
	var gotPath string
	var gotModel string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		gotPath = r.URL.Path

		var body struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		gotModel = body.Model
		if body.MaxTokens != 1 {
			t.Fatalf("expected max_tokens=1, got %d", body.MaxTokens)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := NewConnectivityChecker(server.Client())
	preset := ProviderPreset{
		ID:             "anthropic",
		Label:          "Anthropic",
		BaseURL:        server.URL + "/v1/chat/completions",
		RequiresAPIKey: true,
		CheckMode:      CheckModeChatPing,
	}

	result, err := checker.Check(context.Background(), preset, "claude-sonnet-4-20250514", "sk-ant")
	if err != nil {
		t.Fatalf("connectivity check failed: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("expected path /v1/chat/completions, got %q", gotPath)
	}
	if gotAuth != "Bearer sk-ant" {
		t.Fatalf("expected bearer auth, got %q", gotAuth)
	}
	if gotAPIKey != "sk-ant" {
		t.Fatalf("expected x-api-key header, got %q", gotAPIKey)
	}
	if gotModel != "claude-sonnet-4-20250514" {
		t.Fatalf("unexpected model %q", gotModel)
	}
	if result.Endpoint != server.URL+"/v1/chat/completions" {
		t.Fatalf("unexpected endpoint %q", result.Endpoint)
	}
}

func TestConnectivityCheckerMissingAPIKey(t *testing.T) {
	checker := NewConnectivityChecker(nil)
	preset := ProviderPreset{
		ID:             "openai",
		Label:          "OpenAI",
		BaseURL:        "https://api.openai.com/v1/chat/completions",
		RequiresAPIKey: true,
		CheckMode:      CheckModeListModels,
	}

	if _, err := checker.Check(context.Background(), preset, "gpt-5.1", ""); err == nil {
		t.Fatalf("expected missing API key error")
	}
}
