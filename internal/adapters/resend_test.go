package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

func TestResendAdapterExecute(t *testing.T) {
	var seenAuth string
	var seenBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seenAuth = request.Header.Get("Authorization")
		if err := json.NewDecoder(request.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"test-123"}`))
	}))
	defer server.Close()

	adapter := NewResendAdapter()
	adapter.client = server.Client()

	// Override the endpoint by pointing the client at the test server.
	// Since we cannot change the hardcoded URL, we use a transport override.
	adapter.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = server.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	result, err := adapter.Execute(context.Background(), engine.AdapterAction{
		Adapter:   "resend",
		Operation: "send_email",
		Payload: map[string]any{
			"from":    "noreply@example.com",
			"to":      "user@example.com",
			"subject": "Hello from PookiePaws",
			"html":    "<h1>Hello</h1>",
			"text":    "Hello",
		},
	}, stubSecrets{
		"resend_api_key": "re_test_key",
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if seenAuth != "Bearer re_test_key" {
		t.Fatalf("expected Bearer auth header, got %q", seenAuth)
	}
	if seenBody["to"] != "user@example.com" {
		t.Fatalf("expected recipient in body, got %v", seenBody["to"])
	}
	if seenBody["subject"] != "Hello from PookiePaws" {
		t.Fatalf("expected subject in body, got %v", seenBody["subject"])
	}
	if result.Status != "sent" {
		t.Fatalf("unexpected status %q", result.Status)
	}
	if result.Adapter != "resend" {
		t.Fatalf("unexpected adapter %q", result.Adapter)
	}
}

func TestResendAdapterMissingAPIKey(t *testing.T) {
	adapter := NewResendAdapter()

	_, err := adapter.Execute(context.Background(), engine.AdapterAction{
		Adapter:   "resend",
		Operation: "send_email",
		Payload: map[string]any{
			"from":    "noreply@example.com",
			"to":      "user@example.com",
			"subject": "Test",
			"html":    "<p>Test</p>",
		},
	}, stubSecrets{})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestResendAdapterErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = writer.Write([]byte(`{"message":"Invalid email address"}`))
	}))
	defer server.Close()

	adapter := NewResendAdapter()
	adapter.client = server.Client()
	adapter.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = server.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	_, err := adapter.Execute(context.Background(), engine.AdapterAction{
		Adapter:   "resend",
		Operation: "send_email",
		Payload: map[string]any{
			"from":    "noreply@example.com",
			"to":      "bad-address",
			"subject": "Test",
			"html":    "<p>Test</p>",
		},
	}, stubSecrets{
		"resend_api_key": "re_test_key",
	})
	if err == nil {
		t.Fatal("expected error for 422 response")
	}
}

// roundTripFunc allows using a plain function as an http.RoundTripper,
// which lets tests redirect requests from the production URL to the
// httptest server.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
