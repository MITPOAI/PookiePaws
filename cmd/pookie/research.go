package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mitpoai/pookiepaws/internal/cli"
	"github.com/mitpoai/pookiepaws/internal/dossier"
	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/research"
	"github.com/mitpoai/pookiepaws/internal/scheduler"
	"github.com/mitpoai/pookiepaws/internal/security"
)

// cmdResearch is the top-level dispatcher for `pookie research <sub>`.
// Each subcommand wraps a small `runXxx` helper that takes injected
// dependencies so the helpers stay testable in isolation.
func cmdResearch(args []string) {
	if len(args) == 0 {
		printResearchUsage(os.Stderr)
		os.Exit(2)
	}
	switch args[0] {
	case "analyze":
		cmdResearchAnalyze(args[1:])
	case "watchlists":
		cmdResearchWatchlists(args[1:])
	case "refresh":
		cmdResearchRefresh(args[1:])
	case "schedule":
		cmdResearchSchedule(args[1:])
	case "status":
		cmdResearchStatus(args[1:])
	case "recommendations":
		cmdResearchRecommendations(args[1:])
	case "dossier":
		cmdResearchDossier(args[1:])
	case "help", "--help", "-h":
		printResearchUsage(os.Stdout)
	default:
		slog.Error("unknown research subcommand", "subcommand", args[0])
		printResearchUsage(os.Stderr)
		os.Exit(2)
	}
}

func printResearchUsage(w io.Writer) {
	fmt.Fprint(w, `pookie research <subcommand>

  analyze --company <name> [flags]            Check competitors online and save a local dossier
  watchlists list                            Print configured watchlists
  watchlists apply --file <json>             Replace watchlists from JSON file (or --stdin)
  watchlists show <id>                       Show a single watchlist
  watchlists delete <id>                     Delete a watchlist (idempotent)
  dossier list [--limit N]                   List recent dossiers
  dossier show <id>                          Show a single dossier
  dossier diff <watchlist-id>                Show latest dossier diff for a watchlist
  dossier evidence <dossier-id> [--limit N]  List evidence records for a dossier
  refresh                                    Submit a watchlist refresh workflow now
  schedule --mode <m>                        Set research schedule (manual|hourly|daily)
  status                                     Show scheduler state
  recommendations [--status s]               List recommendations (draft|queued|submitted|discarded)
  recommendations show <id>                  Show a single recommendation
  recommendations queue <id> --workflow <wf> Mark a recommendation as queued for a workflow
  recommendations discard <id>               Mark a recommendation as discarded
`)
}

type researchAnalyzeOptions struct {
	Name       string
	Topic      string
	Company    string
	Market     string
	Country    string
	Location   string
	Provider   string
	Schedule   string
	Debug      bool
	NoExport   bool
	MaxSources int

	Competitors []string
	Domains     []string
	Pages       []string
	FocusAreas  []string
}

type latestResearchStatus struct {
	DossierID     string    `json:"dossier_id,omitempty"`
	WatchlistID   string    `json:"watchlist_id,omitempty"`
	Topic         string    `json:"topic,omitempty"`
	Company       string    `json:"company,omitempty"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
	MarkdownPath  string    `json:"markdown_path,omitempty"`
	MarkdownSaved bool      `json:"markdown_saved"`
}

func cmdResearchAnalyze(args []string) {
	p := cli.Stdout()
	p.Banner()

	opts, err := parseResearchAnalyzeArgs(args)
	if err != nil {
		p.Error("%v", err)
		p.Blank()
		printResearchUsage(os.Stderr)
		os.Exit(2)
	}

	runtimeRoot := resolveRuntimeRoot()
	svc, err := dossier.NewService(runtimeRoot)
	if err != nil {
		p.Error("init dossier service: %v", err)
		os.Exit(1)
	}
	secrets, err := security.NewJSONSecretProvider(runtimeRoot)
	if err != nil {
		p.Error("open secrets vault: %v", err)
		os.Exit(1)
	}

	err = runResearchAnalyze(
		context.Background(),
		svc,
		secrets,
		opts,
		func(mode string) error {
			if mode == scheduler.ModeManual {
				return nil
			}
			return writeVaultSecret("research_schedule", mode)
		},
		runtimeRoot,
		os.Stdout,
	)
	if err != nil {
		p.Error("research analyze failed: %v", err)
		if hint := researchAnalyzeHint(err); hint != "" {
			p.Info("%s", hint)
		}
		os.Exit(1)
	}

	if opts.Schedule != scheduler.ModeManual {
		p.Info("Recurring research is now set to %s. Keep `pookie start` running so the daemon can continue checking online.", opts.Schedule)
		p.Blank()
	}
}

func parseResearchAnalyzeArgs(args []string) (researchAnalyzeOptions, error) {
	var opts researchAnalyzeOptions

	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	var stderr bytes.Buffer
	fs.SetOutput(&stderr)

	name := fs.String("name", "", "Watchlist display name")
	topic := fs.String("topic", "", "Topic label for the dossier")
	company := fs.String("company", "", "Target brand or company name")
	competitors := fs.String("competitors", "", "Comma-separated competitors to compare")
	domains := fs.String("domains", "", "Comma-separated public domains to prioritize")
	pages := fs.String("pages", "", "Comma-separated public pages to observe directly")
	focusAreas := fs.String("focus-areas", "", "Comma-separated focus areas such as pricing, positioning, offers")
	market := fs.String("market", "", "Market context")
	country := fs.String("country", "", "Country code for search geo")
	location := fs.String("location", "", "Location string for search geo")
	provider := fs.String("provider", "", "Research provider (auto|internal|firecrawl|jina)")
	maxSources := fs.Int("max-sources", 0, "Maximum public sources to keep (default 6)")
	schedule := fs.String("schedule", scheduler.ModeManual, "Run once or enable recurring refreshes (manual|hourly|daily)")
	debug := fs.Bool("debug", false, "Include debug-level research output")
	noExport := fs.Bool("no-export", false, "Skip the automatic local Markdown brief")

	if err := fs.Parse(args); err != nil {
		return opts, fmt.Errorf("invalid analyze flags: %w", err)
	}
	if len(fs.Args()) > 0 {
		return opts, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}

	opts = researchAnalyzeOptions{
		Name:        strings.TrimSpace(*name),
		Topic:       strings.TrimSpace(*topic),
		Company:     strings.TrimSpace(*company),
		Market:      strings.TrimSpace(*market),
		Country:     strings.TrimSpace(*country),
		Location:    strings.TrimSpace(*location),
		Provider:    strings.TrimSpace(*provider),
		Schedule:    strings.TrimSpace(*schedule),
		Debug:       *debug,
		NoExport:    *noExport,
		MaxSources:  *maxSources,
		Competitors: splitCSVArgs(*competitors),
		Domains:     splitCSVArgs(*domains),
		Pages:       splitCSVArgs(*pages),
		FocusAreas:  splitCSVArgs(*focusAreas),
	}

	if opts.Schedule == "" {
		opts.Schedule = scheduler.ModeManual
	}
	switch opts.Schedule {
	case scheduler.ModeManual, scheduler.ModeHourly, scheduler.ModeDaily:
	default:
		return opts, fmt.Errorf("schedule must be manual, hourly, or daily")
	}
	if opts.Company == "" && len(opts.Competitors) == 0 {
		return opts, fmt.Errorf("company or competitors is required")
	}
	if opts.Provider != "" {
		switch strings.ToLower(opts.Provider) {
		case "auto", "internal", "firecrawl", "jina":
		default:
			return opts, fmt.Errorf("provider must be auto, internal, firecrawl, or jina")
		}
	}
	if opts.MaxSources < 0 {
		return opts, fmt.Errorf("max-sources must be zero or greater")
	}
	return opts, nil
}

func runResearchAnalyze(ctx context.Context, svc *dossier.Service, secrets engine.SecretProvider, opts researchAnalyzeOptions, setSchedule func(string) error, runtimeRoot string, out io.Writer) error {
	p := cli.New(out)
	stateDir := filepath.Join(runtimeRoot, "state", "research")
	p.Box("Analyze Request", [][2]string{
		{"company", emptyDash(opts.Company)},
		{"competitors", emptyDash(strings.Join(opts.Competitors, ", "))},
		{"domains", emptyDash(strings.Join(opts.Domains, ", "))},
		{"pages", emptyDash(strings.Join(opts.Pages, ", "))},
		{"focus", emptyDash(strings.Join(opts.FocusAreas, ", "))},
		{"market", emptyDash(opts.Market)},
		{"geo", emptyDash(strings.TrimSpace(strings.TrimSpace(opts.Country + " " + opts.Location)))},
		{"provider", firstNonEmpty(strings.ToLower(opts.Provider), "default")},
		{"max sources", fmt.Sprintf("%d", normalizeAnalyzeMaxSources(opts.MaxSources))},
		{"schedule", opts.Schedule},
	})
	p.Blank()
	p.Info("Checking bounded public sources online and saving a local dossier under %s", stateDir)
	p.Blank()

	generated, err := svc.GenerateDossier(ctx, dossier.GenerateRequest{
		Name:           opts.Name,
		Topic:          opts.Topic,
		Company:        opts.Company,
		Competitors:    opts.Competitors,
		Domains:        opts.Domains,
		Pages:          opts.Pages,
		Market:         opts.Market,
		Country:        opts.Country,
		Location:       opts.Location,
		MaxSources:     opts.MaxSources,
		FocusAreas:     opts.FocusAreas,
		Provider:       opts.Provider,
		Debug:          opts.Debug,
		SkipExport:     opts.NoExport,
		TrustedDomains: nil,
	}, secrets)
	if err != nil {
		return err
	}

	if opts.Schedule != scheduler.ModeManual && setSchedule != nil {
		if err := setSchedule(opts.Schedule); err != nil {
			return fmt.Errorf("save research schedule: %w", err)
		}
	}

	p.Success("Competitor analysis saved locally")
	p.Blank()
	p.Box("Research Result", [][2]string{
		{"watchlist", generated.Watchlist.ID},
		{"dossier", generated.Dossier.ID},
		{"topic", emptyDash(generated.Dossier.Topic)},
		{"provider", emptyDash(generated.Dossier.Provider)},
		{"coverage", formatCoverage(generated.Dossier.Coverage)},
		{"evidence", fmt.Sprintf("%d", len(generated.Evidence))},
		{"changes", fmt.Sprintf("%d", len(generated.Changes))},
		{"recommendations", fmt.Sprintf("%d", len(generated.Recommendations))},
	})
	p.Blank()
	p.Box("Saved Output", [][2]string{
		{"state dir", stateDir},
		{"watchlist json", filepath.Join(stateDir, "watchlists", generated.Watchlist.ID+".json")},
		{"dossier json", filepath.Join(stateDir, "dossiers", generated.Dossier.ID+".json")},
		{"markdown brief", firstNonEmpty(generated.Dossier.MarkdownPath, "disabled")},
	})
	if len(generated.Dossier.Warnings) > 0 {
		p.Blank()
		for _, warning := range generated.Dossier.Warnings {
			p.Warning("%s", warning)
		}
	}
	p.Blank()
	return nil
}

func researchAnalyzeHint(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "company or competitors is required"),
		strings.Contains(text, "bounded research requires at least one competitor or company"):
		return "Provide --company or --competitors. Example: pookie research analyze --company \"PookiePaws\" --competitors \"OpenClaw,PetBox\" --domains \"openclaw.com,petbox.com\"."
	case strings.Contains(text, "firecrawl_api_key"):
		return "Configure firecrawl_api_key in your local settings or use --provider internal/auto."
	case strings.Contains(text, "research_provider=jina requires explicit domains"):
		return "Add --domains when using --provider jina so the run has public pages to seed from."
	default:
		return ""
	}
}

func splitCSVArgs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := strings.ToLower(part)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, part)
	}
	return out
}

func normalizeAnalyzeMaxSources(value int) int {
	if value <= 0 || value > 6 {
		return 6
	}
	return value
}

// --- watchlists subcommand ---

func cmdResearchWatchlists(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: pookie research watchlists <list|apply|show|delete>")
		os.Exit(2)
	}
	svc := mustDossierService()
	switch args[0] {
	case "list":
		if err := runResearchWatchlistsList(context.Background(), svc, os.Stdout); err != nil {
			slog.Error("list watchlists failed", "err", err)
			os.Exit(1)
		}
	case "apply":
		fs := flag.NewFlagSet("apply", flag.ExitOnError)
		file := fs.String("file", "", "JSON file containing a watchlist array")
		stdin := fs.Bool("stdin", false, "Read watchlists from stdin")
		_ = fs.Parse(args[1:])
		var input io.Reader
		if *stdin {
			input = os.Stdin
		}
		if err := runResearchWatchlistsApply(context.Background(), svc, *file, input, os.Stdout); err != nil {
			slog.Error("apply watchlists failed", "err", err)
			os.Exit(1)
		}
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: pookie research watchlists show <id>")
			os.Exit(2)
		}
		if err := runResearchWatchlistsShow(context.Background(), svc, args[1], os.Stdout); err != nil {
			slog.Error("show watchlist failed", "err", err)
			os.Exit(1)
		}
	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: pookie research watchlists delete <id>")
			os.Exit(2)
		}
		if err := runResearchWatchlistsDelete(context.Background(), svc, args[1], os.Stdout); err != nil {
			slog.Error("delete watchlist failed", "err", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: pookie research watchlists <list|apply|show|delete>")
		os.Exit(2)
	}
}

func runResearchWatchlistsList(ctx context.Context, svc *dossier.Service, out io.Writer) error {
	all, err := svc.ListWatchlists(ctx)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		fmt.Fprintln(out, "no watchlists configured")
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tTOPIC\tLAST RUN")
	for _, wl := range all {
		last := "-"
		if wl.LastRunAt != nil {
			last = wl.LastRunAt.Format(time.RFC3339)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", wl.ID, wl.Name, wl.Topic, last)
	}
	return tw.Flush()
}

func runResearchWatchlistsApply(ctx context.Context, svc *dossier.Service, file string, stdin io.Reader, out io.Writer) error {
	var data []byte
	var err error
	switch {
	case file != "":
		data, err = os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
	case stdin != nil:
		data, err = io.ReadAll(stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
	default:
		return fmt.Errorf("either --file or --stdin is required")
	}
	var watchlists []dossier.Watchlist
	if err := json.Unmarshal(data, &watchlists); err != nil {
		return fmt.Errorf("parse json: %w", err)
	}
	saved, err := svc.SaveWatchlists(ctx, watchlists)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}
	fmt.Fprintf(out, "applied %d watchlist(s)\n", len(saved))
	return nil
}

func runResearchWatchlistsDelete(ctx context.Context, svc *dossier.Service, id string, out io.Writer) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("watchlist id is required")
	}
	// Probe whether the watchlist existed so we can give a slightly clearer
	// confirmation. DeleteWatchlist itself is idempotent.
	_, getErr := svc.GetWatchlist(ctx, id)
	if err := svc.DeleteWatchlist(ctx, id); err != nil {
		return err
	}
	if getErr != nil {
		fmt.Fprintf(out, "deleted watchlist %q (or already absent)\n", id)
	} else {
		fmt.Fprintf(out, "deleted watchlist %q\n", id)
	}
	return nil
}

func runResearchWatchlistsShow(ctx context.Context, svc *dossier.Service, id string, out io.Writer) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("watchlist id is required")
	}
	wl, err := svc.GetWatchlist(ctx, id)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	last := "-"
	if wl.LastRunAt != nil {
		last = wl.LastRunAt.Format(time.RFC3339)
	}
	fmt.Fprintf(tw, "ID:\t%s\n", emptyDash(wl.ID))
	fmt.Fprintf(tw, "Name:\t%s\n", emptyDash(wl.Name))
	fmt.Fprintf(tw, "Topic:\t%s\n", emptyDash(wl.Topic))
	fmt.Fprintf(tw, "Company:\t%s\n", emptyDash(wl.Company))
	fmt.Fprintf(tw, "Market:\t%s\n", emptyDash(wl.Market))
	fmt.Fprintf(tw, "Country:\t%s\n", emptyDash(wl.Country))
	fmt.Fprintf(tw, "Location:\t%s\n", emptyDash(wl.Location))
	fmt.Fprintf(tw, "MaxSources:\t%d\n", wl.MaxSources)
	fmt.Fprintf(tw, "Domains:\t%s\n", emptyDash(strings.Join(wl.Domains, ", ")))
	fmt.Fprintf(tw, "Competitors:\t%s\n", emptyDash(strings.Join(wl.Competitors, ", ")))
	fmt.Fprintf(tw, "FocusAreas:\t%s\n", emptyDash(strings.Join(wl.FocusAreas, ", ")))
	fmt.Fprintf(tw, "LastRunAt:\t%s\n", last)
	fmt.Fprintf(tw, "LastDossierID:\t%s\n", emptyDash(wl.LastDossierID))
	return tw.Flush()
}

// --- dossier ---

func cmdResearchDossier(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: pookie research dossier <list|show|diff|evidence>")
		os.Exit(2)
	}
	svc := mustDossierService()
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("list", flag.ExitOnError)
		limit := fs.Int("limit", 50, "Maximum number of dossiers to return")
		_ = fs.Parse(args[1:])
		if err := runResearchDossierList(context.Background(), svc, *limit, os.Stdout); err != nil {
			slog.Error("list dossiers failed", "err", err)
			os.Exit(1)
		}
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: pookie research dossier show <id>")
			os.Exit(2)
		}
		if err := runResearchDossierShow(context.Background(), svc, args[1], os.Stdout); err != nil {
			slog.Error("show dossier failed", "err", err)
			os.Exit(1)
		}
	case "diff":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: pookie research dossier diff <watchlist-id>")
			os.Exit(2)
		}
		if err := runResearchDossierDiff(context.Background(), svc, args[1], os.Stdout); err != nil {
			slog.Error("dossier diff failed", "err", err)
			os.Exit(1)
		}
	case "evidence":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: pookie research dossier evidence <dossier-id> [--limit N]")
			os.Exit(2)
		}
		dossierID := args[1]
		fs := flag.NewFlagSet("evidence", flag.ExitOnError)
		limit := fs.Int("limit", 50, "Maximum number of evidence records to return")
		_ = fs.Parse(args[2:])
		if err := runResearchDossierEvidence(context.Background(), svc, dossierID, *limit, os.Stdout); err != nil {
			slog.Error("dossier evidence failed", "err", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: pookie research dossier <list|show|diff|evidence>")
		os.Exit(2)
	}
}

func runResearchDossierList(ctx context.Context, svc *dossier.Service, limit int, out io.Writer) error {
	if limit <= 0 {
		limit = 50
	}
	dossiers, err := svc.ListDossiers(ctx, limit)
	if err != nil {
		return err
	}
	if len(dossiers) == 0 {
		fmt.Fprintln(out, "no dossiers")
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tWATCHLIST\tTOPIC\tCREATED")
	for _, d := range dossiers {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", d.ID, emptyDash(d.WatchlistID), emptyDash(d.Topic), d.CreatedAt.Format(time.RFC3339))
	}
	return tw.Flush()
}

func runResearchDossierShow(ctx context.Context, svc *dossier.Service, id string, out io.Writer) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("dossier id is required")
	}
	// No GetDossier exists; pull a generous slice and filter by ID.
	dossiers, err := svc.ListDossiers(ctx, 0)
	if err != nil {
		return err
	}
	var found *dossier.Dossier
	for i := range dossiers {
		if dossiers[i].ID == id {
			found = &dossiers[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("dossier %q not found", id)
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "ID:\t%s\n", found.ID)
	fmt.Fprintf(tw, "WatchlistID:\t%s\n", emptyDash(found.WatchlistID))
	fmt.Fprintf(tw, "Topic:\t%s\n", emptyDash(found.Topic))
	fmt.Fprintf(tw, "Company:\t%s\n", emptyDash(found.Company))
	fmt.Fprintf(tw, "Provider:\t%s\n", emptyDash(found.Provider))
	fmt.Fprintf(tw, "Summary:\t%s\n", emptyDash(found.Summary))
	fmt.Fprintf(tw, "FallbackReason:\t%s\n", emptyDash(found.FallbackReason))
	fmt.Fprintf(tw, "MarkdownPath:\t%s\n", emptyDash(found.MarkdownPath))
	fmt.Fprintf(tw, "Findings:\t%d\n", len(found.Findings))
	fmt.Fprintf(tw, "Evidence:\t%d\n", len(found.EvidenceIDs))
	fmt.Fprintf(tw, "Changes:\t%d\n", len(found.ChangeIDs))
	fmt.Fprintf(tw, "Recommendations:\t%d\n", len(found.RecommendationIDs))
	fmt.Fprintf(tw, "CreatedAt:\t%s\n", found.CreatedAt.Format(time.RFC3339))
	return tw.Flush()
}

func runResearchDossierDiff(ctx context.Context, svc *dossier.Service, watchlistID string, out io.Writer) error {
	if strings.TrimSpace(watchlistID) == "" {
		return fmt.Errorf("watchlist id is required")
	}
	diff, err := svc.DiffLatest(ctx, watchlistID)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, c := range diff.Changes {
		counts[c.Kind]++
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "WatchlistID:\t%s\n", emptyDash(diff.WatchlistID))
	fmt.Fprintf(tw, "DossierID:\t%s\n", emptyDash(diff.DossierID))
	fmt.Fprintf(tw, "Summary:\t%s\n", emptyDash(diff.Summary))
	fmt.Fprintf(tw, "Added:\t%d\n", counts["added"])
	fmt.Fprintf(tw, "Modified:\t%d\n", counts["modified"])
	fmt.Fprintf(tw, "Removed:\t%d\n", counts["removed"])
	fmt.Fprintf(tw, "Total:\t%d\n", len(diff.Changes))
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(diff.Changes) > 0 {
		fmt.Fprintln(out)
		ctw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(ctw, "KIND\tENTITY\tSOURCE")
		for _, c := range diff.Changes {
			fmt.Fprintf(ctw, "%s\t%s\t%s\n", c.Kind, emptyDash(c.Entity), emptyDash(c.SourceURL))
		}
		_ = ctw.Flush()
	}
	return nil
}

func runResearchDossierEvidence(ctx context.Context, svc *dossier.Service, dossierID string, limit int, out io.Writer) error {
	if strings.TrimSpace(dossierID) == "" {
		return fmt.Errorf("dossier id is required")
	}
	if limit <= 0 {
		limit = 50
	}
	items, err := svc.ListEvidence(ctx, dossierID, limit)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintln(out, "no evidence")
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tURL\tTITLE\tCAPTURED")
	for _, e := range items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.ID, emptyDash(e.SourceURL), emptyDash(e.Title), e.ObservedAt.Format(time.RFC3339))
	}
	return tw.Flush()
}

// --- refresh ---

func cmdResearchRefresh(args []string) {
	fs := flag.NewFlagSet("refresh", flag.ExitOnError)
	_ = fs.Parse(args)

	stack, err := buildStack(resolveRuntimeRoot(), resolveWorkspaceRoot())
	if err != nil {
		slog.Error("build stack failed", "err", err)
		os.Exit(1)
	}
	defer stack.Close()

	wls, err := stack.dossier.ListWatchlists(context.Background())
	if err != nil {
		slog.Error("list watchlists failed", "err", err)
		os.Exit(1)
	}
	if len(wls) == 0 {
		slog.Error("no watchlists configured", "hint", "apply some first with: pookie research watchlists apply --file <json>")
		os.Exit(1)
	}

	wf, err := stack.coord.SubmitWorkflow(context.Background(), engine.WorkflowDefinition{
		Name:  "Manual watchlist refresh",
		Skill: scheduler.SkillName,
		Input: map[string]any{"trigger": "cli"},
	})
	if err != nil {
		slog.Error("submit workflow failed", "err", err)
		os.Exit(1)
	}
	fmt.Printf("submitted workflow %s\n", wf.ID)
}

// --- schedule ---

func cmdResearchSchedule(args []string) {
	fs := flag.NewFlagSet("schedule", flag.ExitOnError)
	mode := fs.String("mode", "", "Schedule mode (manual|hourly|daily)")
	_ = fs.Parse(args)

	switch *mode {
	case "":
		slog.Error("--mode is required", "valid", "manual|hourly|daily")
		os.Exit(2)
	case "manual", "hourly", "daily":
		// ok
	default:
		slog.Error("invalid mode", "mode", *mode, "valid", "manual|hourly|daily")
		os.Exit(2)
	}

	if err := writeVaultSecret("research_schedule", *mode); err != nil {
		slog.Error("write secret failed", "err", err)
		os.Exit(1)
	}
	fmt.Printf("research_schedule = %s\n", *mode)
}

// --- status ---

// researchStatusPayload is the shape returned by buildResearchStatusPayload.
// Centralized so both the JSON and human renderings agree on the hint logic.
type researchStatusPayload struct {
	Scheduler       scheduler.State       `json:"scheduler"`
	Watchlists      *int                  `json:"watchlists,omitempty"`
	WatchlistsError string                `json:"watchlists_error,omitempty"`
	Latest          *latestResearchStatus `json:"latest,omitempty"`
	Hint            string                `json:"hint,omitempty"`
}

// buildResearchStatusPayload constructs the status payload and attaches a
// helpful hint when the scheduler state looks completely empty (no schedule
// configured AND no tick has ever happened). That combination almost always
// means the daemon hasn't been started.
func buildResearchStatusPayload(st scheduler.State, watchlistsCount int, watchlistsErr error, latest *latestResearchStatus) researchStatusPayload {
	p := researchStatusPayload{Scheduler: st, Latest: latest}
	if watchlistsErr != nil {
		p.WatchlistsError = watchlistsErr.Error()
	} else {
		c := watchlistsCount
		p.Watchlists = &c
	}
	if st.LastTickAt.IsZero() && st.Schedule == "" {
		p.Hint = "scheduler runs only while the daemon is running. Start it with: pookie start"
	}
	return p
}

func cmdResearchStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	_ = fs.Parse(args)

	store := scheduler.NewStateStore(scheduler.DefaultStatePath(resolveRuntimeRoot()))
	st, err := store.Load()
	if err != nil {
		slog.Error("load state failed", "err", err)
		os.Exit(1)
	}
	svc := mustDossierService()
	wls, wlErr := svc.ListWatchlists(context.Background())
	latest, latestErr := loadLatestResearchStatus(context.Background(), svc)
	if latestErr != nil {
		slog.Error("load latest research failed", "err", latestErr)
	}

	payload := buildResearchStatusPayload(st, len(wls), wlErr, latest)

	if *jsonOut {
		emitJSONOrExit(payload)
		return
	}

	fmt.Printf("schedule:        %s\n", emptyDash(st.Schedule))
	if wlErr != nil {
		fmt.Printf("watchlists:      ? (load error: %v)\n", wlErr)
	} else {
		fmt.Printf("watchlists:      %d\n", len(wls))
	}
	fmt.Printf("last tick:       %s\n", formatTime(st.LastTickAt))
	fmt.Printf("last success:    %s\n", formatTime(st.LastSuccessAt))
	fmt.Printf("last workflow:   %s\n", emptyDash(st.LastWorkflow))
	fmt.Printf("next due:        %s\n", formatTime(st.NextDueAt))
	fmt.Printf("last error:      %s\n", emptyDash(st.LastError))
	if payload.Latest != nil {
		fmt.Printf("latest dossier:  %s\n", emptyDash(payload.Latest.DossierID))
		fmt.Printf("latest topic:    %s\n", emptyDash(payload.Latest.Topic))
		fmt.Printf("latest export:   %s\n", formatLatestExport(payload.Latest))
	}
	if payload.Hint != "" {
		fmt.Printf("\nHint: %s\n", payload.Hint)
	}
}

// --- recommendations ---

func cmdResearchRecommendations(args []string) {
	// Dispatch: if the first arg is a known subcommand, route to it.
	// Otherwise (no args, or first arg looks like a flag) fall through to
	// the existing list-with-filter behavior so the previous CLI shape keeps
	// working.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		svc := mustDossierService()
		switch args[0] {
		case "show":
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "usage: pookie research recommendations show <id>")
				os.Exit(2)
			}
			if err := runResearchRecommendationsShow(context.Background(), svc, args[1], os.Stdout); err != nil {
				slog.Error("show recommendation failed", "err", err)
				os.Exit(1)
			}
			return
		case "queue":
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "usage: pookie research recommendations queue <id> --workflow <wf-id>")
				os.Exit(2)
			}
			id := args[1]
			fs := flag.NewFlagSet("queue", flag.ExitOnError)
			workflow := fs.String("workflow", "", "Workflow ID to associate with the queued recommendation")
			_ = fs.Parse(args[2:])
			if strings.TrimSpace(*workflow) == "" {
				fmt.Fprintln(os.Stderr, "usage: pookie research recommendations queue <id> --workflow <wf-id>")
				os.Exit(2)
			}
			if err := runResearchRecommendationsQueue(context.Background(), svc, id, *workflow, os.Stdout); err != nil {
				slog.Error("queue recommendation failed", "err", err)
				os.Exit(1)
			}
			return
		case "discard":
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "usage: pookie research recommendations discard <id>")
				os.Exit(2)
			}
			if err := runResearchRecommendationsDiscard(context.Background(), svc, args[1], os.Stdout); err != nil {
				slog.Error("discard recommendation failed", "err", err)
				os.Exit(1)
			}
			return
		default:
			slog.Error("unknown recommendations subcommand", "subcommand", args[0])
			os.Exit(2)
		}
	}

	fs := flag.NewFlagSet("recommendations", flag.ExitOnError)
	status := fs.String("status", "", "Filter by status (draft|queued|submitted|discarded)")
	_ = fs.Parse(args)

	switch *status {
	case "",
		string(dossier.RecommendationDraft),
		string(dossier.RecommendationQueued),
		string(dossier.RecommendationSubmitted),
		string(dossier.RecommendationDiscarded):
		// ok
	default:
		slog.Error("invalid status", "status", *status, "valid", "draft|queued|submitted|discarded")
		os.Exit(2)
	}

	svc := mustDossierService()
	recs, err := svc.ListRecommendations(context.Background(), dossier.RecommendationStatus(*status), 100)
	if err != nil {
		slog.Error("list recommendations failed", "err", err)
		os.Exit(1)
	}
	if len(recs) == 0 {
		fmt.Println("no recommendations")
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tDOSSIER\tSTATUS\tTITLE")
	for _, r := range recs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ID, r.DossierID, r.Status, r.Title)
	}
	_ = tw.Flush()
}

func runResearchRecommendationsShow(ctx context.Context, svc *dossier.Service, id string, out io.Writer) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("recommendation id is required")
	}
	rec, err := svc.GetRecommendation(ctx, id)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "ID:\t%s\n", rec.ID)
	fmt.Fprintf(tw, "DossierID:\t%s\n", emptyDash(rec.DossierID))
	fmt.Fprintf(tw, "WatchlistID:\t%s\n", emptyDash(rec.WatchlistID))
	fmt.Fprintf(tw, "Status:\t%s\n", string(rec.Status))
	fmt.Fprintf(tw, "ApprovalStatus:\t%s\n", emptyDash(rec.ApprovalStatus))
	fmt.Fprintf(tw, "Title:\t%s\n", emptyDash(rec.Title))
	fmt.Fprintf(tw, "Topic:\t%s\n", emptyDash(rec.Topic))
	fmt.Fprintf(tw, "Summary:\t%s\n", emptyDash(rec.Summary))
	fmt.Fprintf(tw, "Confidence:\t%.2f\n", rec.Confidence)
	fmt.Fprintf(tw, "Provider:\t%s\n", emptyDash(rec.Provider))
	fmt.Fprintf(tw, "QueuedWorkflowID:\t%s\n", emptyDash(rec.QueuedWorkflowID))
	fmt.Fprintf(tw, "Evidence:\t%d\n", len(rec.EvidenceIDs))
	fmt.Fprintf(tw, "Sources:\t%d\n", len(rec.SourceURLs))
	fmt.Fprintf(tw, "CreatedAt:\t%s\n", rec.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(tw, "UpdatedAt:\t%s\n", rec.UpdatedAt.Format(time.RFC3339))
	return tw.Flush()
}

func runResearchRecommendationsQueue(ctx context.Context, svc *dossier.Service, id string, workflowID string, out io.Writer) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("recommendation id is required")
	}
	if strings.TrimSpace(workflowID) == "" {
		return fmt.Errorf("--workflow is required")
	}
	rec, err := svc.MarkRecommendationQueued(ctx, id, workflowID)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "recommendation %s now %s (workflow=%s)\n", rec.ID, rec.Status, emptyDash(rec.QueuedWorkflowID))
	return nil
}

func runResearchRecommendationsDiscard(ctx context.Context, svc *dossier.Service, id string, out io.Writer) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("recommendation id is required")
	}
	rec, err := svc.DiscardRecommendation(ctx, id)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "recommendation %s now %s\n", rec.ID, rec.Status)
	return nil
}

// --- shared helpers ---

func mustDossierService() *dossier.Service {
	svc, err := dossier.NewService(resolveRuntimeRoot())
	if err != nil {
		slog.Error("init dossier service failed", "err", err)
		os.Exit(1)
	}
	return svc
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func loadLatestResearchStatus(ctx context.Context, svc *dossier.Service) (*latestResearchStatus, error) {
	if svc == nil {
		return nil, nil
	}
	items, err := svc.ListDossiers(ctx, 1)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	latest := items[0]
	status := &latestResearchStatus{
		DossierID:     latest.ID,
		WatchlistID:   latest.WatchlistID,
		Topic:         latest.Topic,
		Company:       latest.Company,
		CreatedAt:     latest.CreatedAt,
		MarkdownPath:  latest.MarkdownPath,
		MarkdownSaved: false,
	}
	if strings.TrimSpace(latest.MarkdownPath) != "" {
		if _, err := os.Stat(latest.MarkdownPath); err == nil {
			status.MarkdownSaved = true
		}
	}
	return status, nil
}

func formatLatestExport(latest *latestResearchStatus) string {
	if latest == nil {
		return "-"
	}
	switch {
	case latest.MarkdownPath == "":
		return "disabled"
	case latest.MarkdownSaved:
		return latest.MarkdownPath
	default:
		return latest.MarkdownPath + " (missing)"
	}
}

func latestResearchID(latest *latestResearchStatus) string {
	if latest == nil {
		return ""
	}
	return latest.DossierID
}

func latestResearchTopic(latest *latestResearchStatus) string {
	if latest == nil {
		return ""
	}
	return latest.Topic
}

func latestResearchCreatedAt(latest *latestResearchStatus) string {
	if latest == nil {
		return "-"
	}
	return formatTime(latest.CreatedAt)
}

func formatCoverage(coverage research.Coverage) string {
	return fmt.Sprintf(
		"%s / kept %d / discovered %d / scraped %d / skipped %d",
		firstNonEmpty(coverage.Mode, "-"),
		coverage.Kept,
		coverage.Discovered,
		coverage.Scraped,
		coverage.Skipped,
	)
}
