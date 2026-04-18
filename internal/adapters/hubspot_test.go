package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

func TestHubSpotCreateContact(t *testing.T) {
	var seenAuth string
	var seenBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"501","properties":{"email":"alice@example.com"}}`))
	}))
	defer server.Close()

	adapter := NewHubSpotAdapter()
	adapter.client = server.Client()

	// Override the endpoint by having the test server respond at any path.
	// We patch the client transport to rewrite the host to the test server.
	adapter.client.Transport = rewriteHostTransport(server)

	result, err := adapter.Execute(context.Background(), engine.AdapterAction{
		Adapter:   "hubspot",
		Operation: "create_contact",
		Payload: map[string]any{
			"email":     "alice@example.com",
			"firstname": "Alice",
			"lastname":  "Smith",
		},
	}, stubSecrets{
		"hubspot_api_key": "hs-test-key",
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if seenAuth != "Bearer hs-test-key" {
		t.Fatalf("expected Bearer auth header, got %q", seenAuth)
	}
	props, ok := seenBody["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties in request body")
	}
	if props["email"] != "alice@example.com" {
		t.Fatalf("expected email in properties, got %v", props["email"])
	}
	if result.Status != "sent" {
		t.Fatalf("unexpected status %q", result.Status)
	}
	if result.Operation != "create_contact" {
		t.Fatalf("unexpected operation %q", result.Operation)
	}
}

func TestHubSpotUpdateContact(t *testing.T) {
	var seenAuth string
	var seenBody map[string]any
	var seenPath string
	var seenMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.Path
		seenMethod = r.Method
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"501","properties":{"email":"alice@newdomain.com"}}`))
	}))
	defer server.Close()

	adapter := NewHubSpotAdapter()
	adapter.client = server.Client()
	adapter.client.Transport = rewriteHostTransport(server)

	result, err := adapter.Execute(context.Background(), engine.AdapterAction{
		Adapter:   "hubspot",
		Operation: "update_contact",
		Payload: map[string]any{
			"id":    "501",
			"email": "alice@newdomain.com",
		},
	}, stubSecrets{
		"hubspot_api_key": "hs-test-key",
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if seenAuth != "Bearer hs-test-key" {
		t.Fatalf("expected Bearer auth header, got %q", seenAuth)
	}
	if seenMethod != http.MethodPatch {
		t.Fatalf("expected PATCH method, got %s", seenMethod)
	}
	if !strings.HasSuffix(seenPath, "/501") {
		t.Fatalf("expected path ending in /501, got %q", seenPath)
	}
	// The id field must not appear in the properties sent to the API.
	props, ok := seenBody["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties in request body")
	}
	if _, hasID := props["id"]; hasID {
		t.Fatalf("id should be excluded from properties payload")
	}
	if props["email"] != "alice@newdomain.com" {
		t.Fatalf("expected email in properties, got %v", props["email"])
	}
	if result.Status != "sent" {
		t.Fatalf("unexpected status %q", result.Status)
	}
	if result.Details["contact_id"] != "501" {
		t.Fatalf("expected contact_id in details")
	}
}

func TestHubSpotMissingAPIKey(t *testing.T) {
	adapter := NewHubSpotAdapter()

	_, err := adapter.Execute(context.Background(), engine.AdapterAction{
		Adapter:   "hubspot",
		Operation: "create_contact",
		Payload: map[string]any{
			"email": "bob@example.com",
		},
	}, stubSecrets{})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	if !strings.Contains(err.Error(), "hubspot_api_key") {
		t.Fatalf("expected error to mention hubspot_api_key, got %q", err.Error())
	}
}

func TestHubSpotUnknownOperation(t *testing.T) {
	adapter := NewHubSpotAdapter()

	_, err := adapter.Execute(context.Background(), engine.AdapterAction{
		Adapter:   "hubspot",
		Operation: "delete_everything",
		Payload:   map[string]any{},
	}, stubSecrets{
		"hubspot_api_key": "hs-test-key",
	})
	if err == nil {
		t.Fatal("expected error for unknown operation")
	}
	if !strings.Contains(err.Error(), "unsupported HubSpot operation") {
		t.Fatalf("expected unsupported operation error, got %q", err.Error())
	}
}

// rewriteHostTransport returns an http.RoundTripper that redirects all requests
// to the given test server, preserving the original path and query string.
func rewriteHostTransport(server *httptest.Server) http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// Replace scheme + host with the test server, keep path and query.
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return http.DefaultTransport.RoundTrip(req)
	})
}
