package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mitpoai/pookiepaws/internal/dossier"
)

func newTestDossierService(t *testing.T) *dossier.Service {
	t.Helper()
	svc, err := dossier.NewService(t.TempDir())
	if err != nil {
		t.Fatalf("dossier.NewService: %v", err)
	}
	return svc
}

func TestResearchWatchlistsListEmpty(t *testing.T) {
	svc := newTestDossierService(t)
	var buf bytes.Buffer
	if err := runResearchWatchlistsList(context.Background(), svc, &buf); err != nil {
		t.Fatalf("runResearchWatchlistsList: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "no watchlists configured") {
		t.Fatalf("expected empty-state message, got: %q", got)
	}
}

func TestResearchWatchlistsListPopulated(t *testing.T) {
	svc := newTestDossierService(t)
	if _, err := svc.SaveWatchlists(context.Background(), []dossier.Watchlist{
		{Name: "Acme competitor watch", Topic: "pricing", Company: "acme"},
	}); err != nil {
		t.Fatalf("SaveWatchlists: %v", err)
	}

	var buf bytes.Buffer
	if err := runResearchWatchlistsList(context.Background(), svc, &buf); err != nil {
		t.Fatalf("runResearchWatchlistsList: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Acme competitor watch") {
		t.Fatalf("expected watchlist name in output, got: %q", out)
	}
	if !strings.Contains(out, "ID") || !strings.Contains(out, "NAME") {
		t.Fatalf("expected table headers in output, got: %q", out)
	}
}

func TestResearchWatchlistsApplyFromFile(t *testing.T) {
	svc := newTestDossierService(t)

	payload := []dossier.Watchlist{
		{Name: "Pricing watch", Topic: "pricing", Company: "acme"},
		{Name: "Positioning watch", Topic: "positioning", Company: "globex"},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(t.TempDir(), "watchlists.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var buf bytes.Buffer
	if err := runResearchWatchlistsApply(context.Background(), svc, path, nil, &buf); err != nil {
		t.Fatalf("runResearchWatchlistsApply: %v", err)
	}
	if !strings.Contains(buf.String(), "applied 2 watchlist") {
		t.Fatalf("expected applied count in output, got: %q", buf.String())
	}

	all, err := svc.ListWatchlists(context.Background())
	if err != nil {
		t.Fatalf("ListWatchlists: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 watchlists persisted, got %d", len(all))
	}
}

func TestResearchWatchlistsApplyFromStdin(t *testing.T) {
	svc := newTestDossierService(t)

	payload := []dossier.Watchlist{
		{Name: "Stdin watch", Topic: "offers", Company: "umbrella"},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var buf bytes.Buffer
	if err := runResearchWatchlistsApply(context.Background(), svc, "", strings.NewReader(string(data)), &buf); err != nil {
		t.Fatalf("runResearchWatchlistsApply: %v", err)
	}
	if !strings.Contains(buf.String(), "applied 1 watchlist") {
		t.Fatalf("expected applied count, got: %q", buf.String())
	}

	all, err := svc.ListWatchlists(context.Background())
	if err != nil {
		t.Fatalf("ListWatchlists: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 watchlist persisted, got %d", len(all))
	}
}

func TestResearchWatchlistsApplyInvalidJSON(t *testing.T) {
	svc := newTestDossierService(t)

	var buf bytes.Buffer
	err := runResearchWatchlistsApply(context.Background(), svc, "", strings.NewReader("not json"), &buf)
	if err == nil {
		t.Fatalf("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse json") {
		t.Fatalf("expected parse json error, got: %v", err)
	}
}

func TestResearchWatchlistsApplyRequiresInput(t *testing.T) {
	svc := newTestDossierService(t)

	var buf bytes.Buffer
	err := runResearchWatchlistsApply(context.Background(), svc, "", nil, &buf)
	if err == nil {
		t.Fatalf("expected error when neither file nor stdin supplied")
	}
}
