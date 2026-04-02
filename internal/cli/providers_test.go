package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFindProviderPreset(t *testing.T) {
	preset, ok := FindProviderPreset("openrouter")
	if !ok {
		t.Fatalf("expected openrouter preset")
	}
	if preset.BaseURL != "https://openrouter.ai/api/v1/chat/completions" {
		t.Fatalf("unexpected base URL %q", preset.BaseURL)
	}
	if preset.ProviderKind != "openai-compatible" {
		t.Fatalf("unexpected provider kind %q", preset.ProviderKind)
	}
	if len(preset.Models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(preset.Models))
	}
}

func TestDefaultProviderPresets(t *testing.T) {
	tests := []struct {
		id      string
		baseURL string
		models  []string
	}{
		{
			id:      "openai",
			baseURL: "https://api.openai.com/v1/chat/completions",
			models:  []string{"gpt-5.1", "o3"},
		},
		{
			id:      "anthropic",
			baseURL: "https://api.anthropic.com/v1/chat/completions",
			models:  []string{"claude-opus-4-1-20250805", "claude-sonnet-4-20250514"},
		},
		{
			id:      "google",
			baseURL: "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
			models:  []string{"gemini-2.5-pro", "gemini-2.5-flash"},
		},
		{
			id:      "openrouter",
			baseURL: "https://openrouter.ai/api/v1/chat/completions",
			models:  []string{"deepseek/deepseek-r1-0528", "qwen/qwen3.5-plus-02-15", "z-ai/glm-5"},
		},
		{
			id:      "ollama",
			baseURL: "http://127.0.0.1:11434/v1/chat/completions",
			models:  []string{"gpt-oss:20b", "gpt-oss:120b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			preset, ok := FindProviderPreset(tt.id)
			if !ok {
				t.Fatalf("expected preset %q", tt.id)
			}
			if preset.BaseURL != tt.baseURL {
				t.Fatalf("expected base URL %q, got %q", tt.baseURL, preset.BaseURL)
			}
			if len(preset.Models) != len(tt.models) {
				t.Fatalf("expected %d models, got %d", len(tt.models), len(preset.Models))
			}
			for index, modelID := range tt.models {
				if preset.Models[index].ID != modelID {
					t.Fatalf("expected model %q at index %d, got %q", modelID, index, preset.Models[index].ID)
				}
			}
		})
	}
}

func TestModelsEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{
			name:    "chat completions",
			baseURL: "https://api.openai.com/v1/chat/completions",
			want:    "https://api.openai.com/v1/models",
		},
		{
			name:    "nested openrouter",
			baseURL: "https://openrouter.ai/api/v1/chat/completions",
			want:    "https://openrouter.ai/api/v1/models",
		},
		{
			name:    "root fallback",
			baseURL: "http://127.0.0.1:1234",
			want:    "http://127.0.0.1:1234/v1/models",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ModelsEndpoint(tt.baseURL)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestDiscoverOpenAICompatibleModels(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"local-alpha"},{"id":"local-beta"}]}`))
	}))
	defer server.Close()

	models, err := DiscoverOpenAICompatibleModels(context.Background(), server.URL+"/v1/chat/completions")
	if err != nil {
		t.Fatalf("discover models: %v", err)
	}
	if gotPath != "/v1/models" {
		t.Fatalf("expected request path /v1/models, got %q", gotPath)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "local-alpha" || models[1].ID != "local-beta" {
		t.Fatalf("unexpected models: %#v", models)
	}
}
