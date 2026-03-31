package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type stubSecrets map[string]string

func (s stubSecrets) Get(name string) (string, error) {
	value, ok := s[name]
	if !ok {
		return "", engine.ErrNotFound
	}
	return value, nil
}

func (s stubSecrets) RedactMap(payload map[string]any) map[string]any {
	return payload
}

func TestSalesmanagoAdapterExecute(t *testing.T) {
	var seenHeader string
	var seenBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seenHeader = request.Header.Get("API-KEY")
		if err := json.NewDecoder(request.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"requestId":"abc"}`))
	}))
	defer server.Close()

	adapter := NewSalesmanagoAdapter()
	adapter.client = server.Client()
	result, err := adapter.Execute(context.Background(), engine.AdapterAction{
		Adapter:   "salesmanago",
		Operation: "route_lead",
		Payload: map[string]any{
			"email":       "lead@example.com",
			"route_queue": "priority-sales",
			"segment":     "vip",
			"priority":    "high",
		},
	}, stubSecrets{
		"salesmanago_api_key":  "sales-key",
		"salesmanago_base_url": server.URL,
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if seenHeader != "sales-key" {
		t.Fatalf("expected API-KEY header")
	}
	if result.Status != "sent" {
		t.Fatalf("unexpected status %q", result.Status)
	}
}

func TestMittoAdapterExecute(t *testing.T) {
	var seenHeader string
	var seenBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seenHeader = request.Header.Get("X-Mitto-API-Key")
		if err := json.NewDecoder(request.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"message_id":"sms-123","status":"accepted"}`))
	}))
	defer server.Close()

	adapter := NewMittoAdapter()
	adapter.client = server.Client()
	result, err := adapter.Execute(context.Background(), engine.AdapterAction{
		Adapter:   "mitto",
		Operation: "send_sms",
		Payload: map[string]any{
			"from":       "PookiePaws",
			"message":    "Hello",
			"recipients": []any{"+61400000000"},
		},
	}, stubSecrets{
		"mitto_api_key":  "mitto-key",
		"mitto_base_url": server.URL,
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if seenHeader != "mitto-key" {
		t.Fatalf("expected Mitto API key header")
	}
	if seenBody["text"] != "Hello" {
		t.Fatalf("expected SMS body")
	}
	if result.Status != "sent" {
		t.Fatalf("unexpected status %q", result.Status)
	}
}
