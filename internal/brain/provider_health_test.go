package brain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckProviderConfigHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "mock-model",
			"choices": []map[string]any{
				{"message": map[string]string{"content": "ok"}},
			},
		})
	}))
	defer server.Close()

	health := CheckProviderConfig(context.Background(), ProviderConfig{
		Type:    providerOpenAICompatible,
		Model:   "mock-model",
		BaseURL: server.URL + "/v1/chat/completions",
		APIKey:  "sk-test",
	}, server.Client())

	if !health.Healthy() {
		t.Fatalf("expected healthy provider, got %+v", health)
	}
}

func TestCheckProviderConfigInvalidModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"message": "qwen/qwen3.5-plus is not a valid model ID"},
		})
	}))
	defer server.Close()

	health := CheckProviderConfig(context.Background(), ProviderConfig{
		Type:    providerOpenAICompatible,
		Model:   "qwen/qwen3.5-plus",
		BaseURL: server.URL + "/v1/chat/completions",
		APIKey:  "sk-test",
	}, server.Client())

	if health.FailureCode != ProviderFailureModel {
		t.Fatalf("expected model failure, got %+v", health)
	}
	if !health.EndpointReachable || !health.CredentialsAccepted || health.ModelAccepted {
		t.Fatalf("unexpected health states: %+v", health)
	}
}

func TestCheckProviderConfigRejectedCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"message": "invalid api key"},
		})
	}))
	defer server.Close()

	health := CheckProviderConfig(context.Background(), ProviderConfig{
		Type:    providerOpenAICompatible,
		Model:   "mock-model",
		BaseURL: server.URL + "/v1/chat/completions",
		APIKey:  "sk-test",
	}, server.Client())

	if health.FailureCode != ProviderFailureCredentials {
		t.Fatalf("expected credentials failure, got %+v", health)
	}
	if !health.EndpointReachable || health.CredentialsAccepted {
		t.Fatalf("unexpected health states: %+v", health)
	}
}
