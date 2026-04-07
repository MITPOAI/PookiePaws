package brain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJinaScraperToolMissingURL(t *testing.T) {
	tool := &JinaScraperTool{}
	_, err := tool.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing url")
	}
}

func TestJinaScraperToolHTTPMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("# Hello\nThis is markdown content."))
	}))
	defer srv.Close()

	tool := &JinaScraperTool{jinaBase: srv.URL}
	result, err := tool.Execute(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	content, ok := result["content"].(string)
	if !ok || !strings.Contains(content, "Hello") {
		t.Errorf("unexpected content: %v", result["content"])
	}
}

func TestJinaScraperToolDefinition(t *testing.T) {
	def := (&JinaScraperTool{}).Definition()
	if def.Type != "function" {
		t.Errorf("want type=function got %s", def.Type)
	}
	if def.Function.Name != "web_search" {
		t.Errorf("want name=web_search got %s", def.Function.Name)
	}
	if _, ok := def.Function.Parameters.Properties["url"]; !ok {
		t.Error("missing url property in schema")
	}
}

func TestJinaScraperToolTruncation(t *testing.T) {
	bigContent := strings.Repeat("x", 10000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(bigContent))
	}))
	defer srv.Close()

	tool := &JinaScraperTool{jinaBase: srv.URL}
	result, err := tool.Execute(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	content := result["content"].(string)
	if len(content) > 8100 {
		t.Errorf("content not truncated: len=%d", len(content))
	}
}

func TestJinaScraperToolHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tool := &JinaScraperTool{jinaBase: srv.URL}
	_, err := tool.Execute(context.Background(), map[string]any{"url": "https://example.com"})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}
