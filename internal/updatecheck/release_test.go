package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchLatestSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); !strings.Contains(got, "pookiepaws-update-check") {
			t.Errorf("User-Agent = %q, want pookiepaws-update-check prefix", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.6.0","html_url":"https://example/release","name":"0.6.0","draft":false,"prerelease":false,"published_at":"2026-04-15T10:00:00Z"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "0.5.2", 2*time.Second)
	rel, err := c.FetchLatest(context.Background())
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if rel.TagName != "v0.6.0" {
		t.Errorf("TagName = %q", rel.TagName)
	}
	if rel.HTMLURL != "https://example/release" {
		t.Errorf("HTMLURL = %q", rel.HTMLURL)
	}
}

func TestFetchLatestSkipsDraftsAndPrereleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.6.0","draft":true,"prerelease":false}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "0.5.2", 2*time.Second)
	_, err := c.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error for draft release")
	}
}

func TestFetchLatestNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"rate limit"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "0.5.2", 2*time.Second)
	_, err := c.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

func TestFetchLatestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"tag_name":"v0.6.0"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "0.5.2", 50*time.Millisecond)
	_, err := c.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
