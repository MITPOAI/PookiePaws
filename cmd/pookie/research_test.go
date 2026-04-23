package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/dossier"
	"github.com/mitpoai/pookiepaws/internal/scheduler"
)

func newTestDossierService(t *testing.T) *dossier.Service {
	t.Helper()
	svc, _ := newTestDossierServiceWithRoot(t)
	return svc
}

// newTestDossierServiceWithRoot returns the service along with the runtime
// root directory it was initialized against, so tests can write fixture
// records (e.g. recommendations) directly to disk under
// <root>/state/research/<kind>/<id>.json.
func newTestDossierServiceWithRoot(t *testing.T) (*dossier.Service, string) {
	t.Helper()
	root := t.TempDir()
	svc, err := dossier.NewService(root)
	if err != nil {
		t.Fatalf("dossier.NewService: %v", err)
	}
	return svc, root
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

// --- watchlists delete ---

func TestResearchWatchlistsDeleteMissingIsIdempotent(t *testing.T) {
	svc := newTestDossierService(t)

	var buf bytes.Buffer
	if err := runResearchWatchlistsDelete(context.Background(), svc, "does-not-exist", &buf); err != nil {
		t.Fatalf("expected no error on missing id, got: %v", err)
	}
	if !strings.Contains(buf.String(), "deleted watchlist") {
		t.Fatalf("expected delete confirmation, got: %q", buf.String())
	}
}

func TestResearchWatchlistsDeleteRemovesExisting(t *testing.T) {
	svc := newTestDossierService(t)
	saved, err := svc.SaveWatchlists(context.Background(), []dossier.Watchlist{
		{Name: "Delete me", Topic: "tmp", Company: "tmp"},
	})
	if err != nil {
		t.Fatalf("SaveWatchlists: %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("expected 1 saved watchlist, got %d", len(saved))
	}
	id := saved[0].ID

	var buf bytes.Buffer
	if err := runResearchWatchlistsDelete(context.Background(), svc, id, &buf); err != nil {
		t.Fatalf("runResearchWatchlistsDelete: %v", err)
	}

	all, err := svc.ListWatchlists(context.Background())
	if err != nil {
		t.Fatalf("ListWatchlists: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected watchlist to be removed, still have %d", len(all))
	}
}

func TestResearchWatchlistsDeleteRequiresID(t *testing.T) {
	svc := newTestDossierService(t)
	var buf bytes.Buffer
	if err := runResearchWatchlistsDelete(context.Background(), svc, "", &buf); err == nil {
		t.Fatalf("expected error when id is empty")
	}
}

// --- watchlists show ---

func TestResearchWatchlistsShowRoundTrip(t *testing.T) {
	svc := newTestDossierService(t)
	saved, err := svc.SaveWatchlists(context.Background(), []dossier.Watchlist{
		{Name: "Show me", Topic: "topic-x", Company: "co-x"},
	})
	if err != nil {
		t.Fatalf("SaveWatchlists: %v", err)
	}
	id := saved[0].ID

	var buf bytes.Buffer
	if err := runResearchWatchlistsShow(context.Background(), svc, id, &buf); err != nil {
		t.Fatalf("runResearchWatchlistsShow: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Show me") {
		t.Fatalf("expected name in output, got: %q", out)
	}
	if !strings.Contains(out, "topic-x") {
		t.Fatalf("expected topic in output, got: %q", out)
	}
}

func TestResearchWatchlistsShowMissing(t *testing.T) {
	svc := newTestDossierService(t)
	var buf bytes.Buffer
	err := runResearchWatchlistsShow(context.Background(), svc, "no-such-id", &buf)
	if err == nil {
		t.Fatalf("expected error for missing id")
	}
}

// --- dossier list ---

func TestResearchDossierListEmpty(t *testing.T) {
	svc := newTestDossierService(t)
	var buf bytes.Buffer
	if err := runResearchDossierList(context.Background(), svc, 50, &buf); err != nil {
		t.Fatalf("runResearchDossierList: %v", err)
	}
	if !strings.Contains(buf.String(), "no dossiers") {
		t.Fatalf("expected empty-state message, got: %q", buf.String())
	}
}

// --- dossier show (missing) ---

func TestResearchDossierShowMissing(t *testing.T) {
	svc := newTestDossierService(t)
	var buf bytes.Buffer
	err := runResearchDossierShow(context.Background(), svc, "no-such-dossier", &buf)
	if err == nil {
		t.Fatalf("expected error for missing dossier")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got: %v", err)
	}
}

// --- dossier evidence empty ---

func TestResearchDossierEvidenceEmpty(t *testing.T) {
	svc := newTestDossierService(t)
	var buf bytes.Buffer
	if err := runResearchDossierEvidence(context.Background(), svc, "no-such-dossier", 50, &buf); err != nil {
		t.Fatalf("runResearchDossierEvidence: %v", err)
	}
	if !strings.Contains(buf.String(), "no evidence") {
		t.Fatalf("expected empty-state message, got: %q", buf.String())
	}
}

// --- recommendations show / queue / discard ---

// seedRecommendation persists a minimal recommendation directly into the
// dossier service's on-disk store so we can exercise the show/queue/discard
// paths without spinning up the full research pipeline.
func seedRecommendation(t *testing.T, root string, rec dossier.Recommendation) dossier.Recommendation {
	t.Helper()
	if rec.ID == "" {
		rec.ID = "rec-test-001"
	}
	if rec.Status == "" {
		rec.Status = dossier.RecommendationDraft
	}
	dir := filepath.Join(root, "state", "research", "recommendations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir recommendations: %v", err)
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatalf("marshal rec: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, rec.ID+".json"), data, 0o644); err != nil {
		t.Fatalf("write rec: %v", err)
	}
	return rec
}

func TestResearchRecommendationsShowRoundTrip(t *testing.T) {
	svc, root := newTestDossierServiceWithRoot(t)
	seed := seedRecommendation(t, root, dossier.Recommendation{
		ID:        "rec-show-001",
		DossierID: "dossier-x",
		Title:     "Show me a rec",
		Summary:   "details",
		Status:    dossier.RecommendationDraft,
	})

	var buf bytes.Buffer
	if err := runResearchRecommendationsShow(context.Background(), svc, seed.ID, &buf); err != nil {
		t.Fatalf("runResearchRecommendationsShow: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Show me a rec") {
		t.Fatalf("expected title in output, got: %q", out)
	}
	if !strings.Contains(out, "draft") {
		t.Fatalf("expected status in output, got: %q", out)
	}
}

func TestResearchRecommendationsShowMissing(t *testing.T) {
	svc := newTestDossierService(t)
	var buf bytes.Buffer
	if err := runResearchRecommendationsShow(context.Background(), svc, "missing-rec", &buf); err == nil {
		t.Fatalf("expected error for missing rec")
	}
}

func TestResearchRecommendationsQueueRoundTrip(t *testing.T) {
	svc, root := newTestDossierServiceWithRoot(t)
	seed := seedRecommendation(t, root, dossier.Recommendation{
		ID:        "rec-queue-001",
		DossierID: "dossier-x",
		Title:     "Queue me",
		Status:    dossier.RecommendationDraft,
	})

	var buf bytes.Buffer
	if err := runResearchRecommendationsQueue(context.Background(), svc, seed.ID, "wf-123", &buf); err != nil {
		t.Fatalf("runResearchRecommendationsQueue: %v", err)
	}
	updated, err := svc.GetRecommendation(context.Background(), seed.ID)
	if err != nil {
		t.Fatalf("GetRecommendation: %v", err)
	}
	// Note: MarkRecommendationQueued sets status to Submitted (per service code)
	if updated.Status != dossier.RecommendationSubmitted {
		t.Fatalf("expected status submitted (per service impl), got: %q", updated.Status)
	}
	if updated.QueuedWorkflowID != "wf-123" {
		t.Fatalf("expected workflow id to be set, got: %q", updated.QueuedWorkflowID)
	}
}

func TestResearchRecommendationsQueueRequiresWorkflow(t *testing.T) {
	svc, root := newTestDossierServiceWithRoot(t)
	seed := seedRecommendation(t, root, dossier.Recommendation{
		ID:        "rec-queue-002",
		DossierID: "dossier-x",
		Title:     "needs workflow",
		Status:    dossier.RecommendationDraft,
	})
	var buf bytes.Buffer
	if err := runResearchRecommendationsQueue(context.Background(), svc, seed.ID, "", &buf); err == nil {
		t.Fatalf("expected error when workflow id is empty")
	}
}

// --- status hint ---

func TestBuildResearchStatusPayloadEmptyAddsHint(t *testing.T) {
	p := buildResearchStatusPayload(scheduler.State{}, 0, nil, nil)
	if p.Hint == "" {
		t.Fatalf("expected hint when scheduler state is empty")
	}
	if !strings.Contains(p.Hint, "pookie start") {
		t.Fatalf("expected hint to mention 'pookie start', got: %q", p.Hint)
	}
	if p.Watchlists == nil || *p.Watchlists != 0 {
		t.Fatalf("expected watchlists=0 to be encoded as a pointer to 0, got: %v", p.Watchlists)
	}
}

func TestBuildResearchStatusPayloadWithScheduleNoHint(t *testing.T) {
	st := scheduler.State{Schedule: "hourly"}
	p := buildResearchStatusPayload(st, 2, nil, nil)
	if p.Hint != "" {
		t.Fatalf("expected no hint when schedule is set, got: %q", p.Hint)
	}
}

func TestBuildResearchStatusPayloadWithLastTickNoHint(t *testing.T) {
	st := scheduler.State{LastTickAt: time.Now()}
	p := buildResearchStatusPayload(st, 0, nil, nil)
	if p.Hint != "" {
		t.Fatalf("expected no hint when daemon has ticked, got: %q", p.Hint)
	}
}

func TestBuildResearchStatusPayloadWithWatchlistsError(t *testing.T) {
	p := buildResearchStatusPayload(scheduler.State{}, 0, errors.New("boom"), nil)
	if p.WatchlistsError != "boom" {
		t.Fatalf("expected watchlists_error=boom, got: %q", p.WatchlistsError)
	}
	if p.Watchlists != nil {
		t.Fatalf("expected nil watchlists count when error is present, got: %v", *p.Watchlists)
	}
	if p.Hint == "" {
		t.Fatalf("expected hint when scheduler state is empty even with watchlist error")
	}
}

func TestResearchRecommendationsDiscardRoundTrip(t *testing.T) {
	svc, root := newTestDossierServiceWithRoot(t)
	seed := seedRecommendation(t, root, dossier.Recommendation{
		ID:        "rec-discard-001",
		DossierID: "dossier-x",
		Title:     "Discard me",
		Status:    dossier.RecommendationDraft,
	})

	var buf bytes.Buffer
	if err := runResearchRecommendationsDiscard(context.Background(), svc, seed.ID, &buf); err != nil {
		t.Fatalf("runResearchRecommendationsDiscard: %v", err)
	}
	updated, err := svc.GetRecommendation(context.Background(), seed.ID)
	if err != nil {
		t.Fatalf("GetRecommendation: %v", err)
	}
	if updated.Status != dossier.RecommendationDiscarded {
		t.Fatalf("expected status discarded, got: %q", updated.Status)
	}
}

type analyzeTestSecrets map[string]string

func (s analyzeTestSecrets) Get(name string) (string, error) {
	return s[name], nil
}

func (s analyzeTestSecrets) RedactMap(payload map[string]any) map[string]any {
	return payload
}

func TestParseResearchAnalyzeArgsParsesListsAndDefaults(t *testing.T) {
	opts, err := parseResearchAnalyzeArgs([]string{
		"--company", "PookiePaws",
		"--competitors", "OpenClaw, PetBox",
		"--domains", "openclaw.example,petbox.example",
		"--focus-areas", "pricing, offer structure",
		"--schedule", "hourly",
	})
	if err != nil {
		t.Fatalf("parseResearchAnalyzeArgs: %v", err)
	}
	if opts.Company != "PookiePaws" {
		t.Fatalf("Company = %q", opts.Company)
	}
	if len(opts.Competitors) != 2 || opts.Competitors[1] != "PetBox" {
		t.Fatalf("Competitors = %#v", opts.Competitors)
	}
	if len(opts.FocusAreas) != 2 || opts.FocusAreas[1] != "offer structure" {
		t.Fatalf("FocusAreas = %#v", opts.FocusAreas)
	}
	if opts.Schedule != scheduler.ModeHourly {
		t.Fatalf("Schedule = %q", opts.Schedule)
	}
	if opts.MaxSources != 0 {
		t.Fatalf("expected zero max sources before normalization, got %d", opts.MaxSources)
	}
}

func TestParseResearchAnalyzeArgsRequiresCompanyOrCompetitors(t *testing.T) {
	_, err := parseResearchAnalyzeArgs([]string{"--domains", "openclaw.example"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "company or competitors is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseResearchAnalyzeArgsHelpReturnsFlagErrHelp(t *testing.T) {
	_, err := parseResearchAnalyzeArgs([]string{"--help"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}
}

func TestPrintResearchAnalyzeUsageIncludesExamples(t *testing.T) {
	var buf bytes.Buffer
	printResearchAnalyzeUsage(&buf)
	text := buf.String()
	if !strings.Contains(text, "pookie research analyze [flags]") {
		t.Fatalf("expected analyze usage header, got %q", text)
	}
	if !strings.Contains(text, "--schedule <mode>") {
		t.Fatalf("expected schedule flag help, got %q", text)
	}
	if !strings.Contains(text, "Recurring schedules only run while \"pookie start\" is running.") {
		t.Fatalf("expected scheduler note, got %q", text)
	}
}

func TestRunResearchAnalyzeWritesMarkdownAndSchedule(t *testing.T) {
	svc, root := newTestDossierServiceWithRoot(t)
	var scheduled string
	var buf bytes.Buffer

	err := runResearchAnalyze(context.Background(), svc, analyzeTestSecrets{}, researchAnalyzeOptions{
		Name:        "OpenClaw fixture watchlist",
		Topic:       "OpenClaw",
		Company:     "PookiePaws",
		Competitors: []string{"OpenClaw"},
		Domains:     []string{"openclaw.example"},
		FocusAreas:  []string{"pricing", "positioning"},
		Schedule:    scheduler.ModeHourly,
	}, func(mode string) error {
		scheduled = mode
		return nil
	}, root, &buf)
	if err != nil {
		t.Fatalf("runResearchAnalyze: %v", err)
	}
	if scheduled != scheduler.ModeHourly {
		t.Fatalf("expected hourly schedule to be persisted, got %q", scheduled)
	}
	if !strings.Contains(buf.String(), "Competitor analysis saved locally") {
		t.Fatalf("expected success output, got %q", buf.String())
	}
	dossiers, err := svc.ListDossiers(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListDossiers: %v", err)
	}
	if len(dossiers) != 1 || dossiers[0].MarkdownPath == "" {
		t.Fatalf("expected latest dossier markdown path, got %+v", dossiers)
	}
	if _, err := os.Stat(dossiers[0].MarkdownPath); err != nil {
		t.Fatalf("expected markdown export to exist, got %v", err)
	}
}

func TestRunResearchAnalyzeNoExportSkipsMarkdown(t *testing.T) {
	svc, root := newTestDossierServiceWithRoot(t)
	var buf bytes.Buffer

	err := runResearchAnalyze(context.Background(), svc, analyzeTestSecrets{}, researchAnalyzeOptions{
		Name:        "OpenClaw fixture watchlist",
		Topic:       "OpenClaw",
		Company:     "PookiePaws",
		Competitors: []string{"OpenClaw"},
		Domains:     []string{"openclaw.example"},
		NoExport:    true,
		Schedule:    scheduler.ModeManual,
	}, func(string) error {
		t.Fatal("manual schedule should not write schedule")
		return nil
	}, root, &buf)
	if err != nil {
		t.Fatalf("runResearchAnalyze: %v", err)
	}
	dossiers, err := svc.ListDossiers(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListDossiers: %v", err)
	}
	if len(dossiers) != 1 {
		t.Fatalf("expected one dossier, got %d", len(dossiers))
	}
	if dossiers[0].MarkdownPath != "" {
		t.Fatalf("expected no markdown path when export disabled, got %q", dossiers[0].MarkdownPath)
	}
	if !strings.Contains(buf.String(), "disabled") {
		t.Fatalf("expected disabled export message, got %q", buf.String())
	}
}
