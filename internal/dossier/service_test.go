package dossier

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/research"
)

func newDossierServiceForTest(t *testing.T) *Service {
	t.Helper()
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("new dossier service: %v", err)
	}
	return svc
}

type stubSecrets map[string]string

func (s stubSecrets) Get(name string) (string, error) {
	return s[name], nil
}

func (s stubSecrets) RedactMap(payload map[string]any) map[string]any {
	return payload
}

func TestGenerateDossierInternalAndDiff(t *testing.T) {
	var pricingBody = `<html><title>Pricing</title><body>Premium operator plan with tracked bundles.</body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body><a href="https://openclaw.example/pricing">OpenClaw Pricing</a></body></html>`))
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
		case "/pricing":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(pricingBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("POOKIEPAWS_INTERNAL_SEARCH_BASE_URL", server.URL+"/search")

	service, err := NewServiceWithResearch(t.TempDir(), research.NewService().WithHTTPClient(&http.Client{Transport: redirectTransport(server)}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	first, err := service.GenerateDossier(context.Background(), GenerateRequest{
		Name:        "OpenClaw core watchlist",
		Topic:       "OpenClaw",
		Company:     "PookiePaws",
		Competitors: []string{"OpenClaw"},
		Pages:       []string{"https://openclaw.example/pricing"},
		Market:      "AU pet gifting",
		FocusAreas:  []string{"pricing", "positioning"},
	}, stubSecrets{})
	if err != nil {
		t.Fatalf("generate first dossier: %v", err)
	}
	if first.Dossier.Provider != "internal" {
		t.Fatalf("expected internal provider, got %q", first.Dossier.Provider)
	}
	if len(first.Evidence) == 0 {
		t.Fatal("expected evidence to be persisted")
	}
	if len(first.Recommendations) == 0 {
		t.Fatal("expected recommendations to be generated")
	}

	pricingBody = `<html><title>Pricing</title><body>Premium operator plan with updated bundle pricing and revised offer stack.</body></html>`
	second, err := service.GenerateDossier(context.Background(), GenerateRequest{
		WatchlistID: first.Watchlist.ID,
		Name:        first.Watchlist.Name,
		Topic:       first.Watchlist.Topic,
		Company:     first.Watchlist.Company,
		Competitors: first.Watchlist.Competitors,
		Pages:       []string{"https://openclaw.example/pricing"},
		Market:      first.Watchlist.Market,
		FocusAreas:  first.Watchlist.FocusAreas,
	}, stubSecrets{})
	if err != nil {
		t.Fatalf("generate second dossier: %v", err)
	}
	if len(second.Changes) == 0 {
		t.Fatal("expected change records after content changed")
	}

	diff, err := service.DiffLatest(context.Background(), first.Watchlist.ID)
	if err != nil {
		t.Fatalf("diff latest: %v", err)
	}
	if diff.Summary == "" {
		t.Fatal("expected diff summary")
	}
}

func redirectTransport(server *httptest.Server) http.RoundTripper {
	target, _ := url.Parse(server.URL)
	return &rewriteTransport{target: target, base: server.Client().Transport}
}

type rewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	return t.base.RoundTrip(req)
}

var _ engine.SecretProvider = stubSecrets{}

func TestGetWatchlistFound(t *testing.T) {
	svc := newDossierServiceForTest(t)
	saved, err := svc.SaveWatchlists(context.Background(), []Watchlist{{ID: "wl-1", Name: "alpha"}})
	if err != nil {
		t.Fatalf("SaveWatchlists: %v", err)
	}
	got, err := svc.GetWatchlist(context.Background(), saved[0].ID)
	if err != nil {
		t.Fatalf("GetWatchlist: %v", err)
	}
	if got.Name != "alpha" {
		t.Errorf("Name = %q, want %q", got.Name, "alpha")
	}
}

func TestGetWatchlistMissing(t *testing.T) {
	svc := newDossierServiceForTest(t)
	if _, err := svc.GetWatchlist(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for missing watchlist")
	}
}

func TestGetWatchlistEmptyID(t *testing.T) {
	svc := newDossierServiceForTest(t)
	if _, err := svc.GetWatchlist(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestDeleteWatchlistRemoves(t *testing.T) {
	svc := newDossierServiceForTest(t)
	saved, err := svc.SaveWatchlists(context.Background(), []Watchlist{{ID: "wl-1", Name: "alpha"}})
	if err != nil {
		t.Fatalf("SaveWatchlists: %v", err)
	}
	if err := svc.DeleteWatchlist(context.Background(), saved[0].ID); err != nil {
		t.Fatalf("DeleteWatchlist: %v", err)
	}
	all, err := svc.ListWatchlists(context.Background())
	if err != nil {
		t.Fatalf("ListWatchlists: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 watchlists after delete, got %d", len(all))
	}
}

func TestDeleteWatchlistMissingIsNoop(t *testing.T) {
	svc := newDossierServiceForTest(t)
	if err := svc.DeleteWatchlist(context.Background(), "nope"); err != nil {
		t.Fatalf("expected nil error for missing delete, got %v", err)
	}
}

func TestDeleteWatchlistEmptyID(t *testing.T) {
	svc := newDossierServiceForTest(t)
	if err := svc.DeleteWatchlist(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestMaxLastRunAt(t *testing.T) {
	svc := newDossierServiceForTest(t)
	t1 := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	if _, err := svc.SaveWatchlists(context.Background(), []Watchlist{
		{ID: "a", Name: "a", LastRunAt: &t1},
		{ID: "b", Name: "b", LastRunAt: &t2},
		{ID: "c", Name: "c"},
	}); err != nil {
		t.Fatalf("SaveWatchlists: %v", err)
	}
	got, err := svc.MaxLastRunAt(context.Background())
	if err != nil {
		t.Fatalf("MaxLastRunAt: %v", err)
	}
	if got == nil || !got.Equal(t2) {
		t.Fatalf("MaxLastRunAt = %v, want %v", got, t2)
	}
}

func TestMaxLastRunAtNoneRun(t *testing.T) {
	svc := newDossierServiceForTest(t)
	if _, err := svc.SaveWatchlists(context.Background(), []Watchlist{{ID: "a", Name: "a"}}); err != nil {
		t.Fatalf("SaveWatchlists: %v", err)
	}
	got, err := svc.MaxLastRunAt(context.Background())
	if err != nil {
		t.Fatalf("MaxLastRunAt: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil when no watchlist has run, got %v", got)
	}
}
