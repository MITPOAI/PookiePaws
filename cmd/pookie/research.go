package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mitpoai/pookiepaws/internal/dossier"
	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/scheduler"
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
		fmt.Fprintf(os.Stderr, "unknown research subcommand: %s\n", args[0])
		printResearchUsage(os.Stderr)
		os.Exit(2)
	}
}

func printResearchUsage(w io.Writer) {
	fmt.Fprint(w, `pookie research <subcommand>

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
			fmt.Fprintf(os.Stderr, "list: %v\n", err)
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
			fmt.Fprintf(os.Stderr, "apply: %v\n", err)
			os.Exit(1)
		}
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: pookie research watchlists show <id>")
			os.Exit(2)
		}
		if err := runResearchWatchlistsShow(context.Background(), svc, args[1], os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "show: %v\n", err)
			os.Exit(1)
		}
	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: pookie research watchlists delete <id>")
			os.Exit(2)
		}
		if err := runResearchWatchlistsDelete(context.Background(), svc, args[1], os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "delete: %v\n", err)
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
			fmt.Fprintf(os.Stderr, "list: %v\n", err)
			os.Exit(1)
		}
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: pookie research dossier show <id>")
			os.Exit(2)
		}
		if err := runResearchDossierShow(context.Background(), svc, args[1], os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "show: %v\n", err)
			os.Exit(1)
		}
	case "diff":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: pookie research dossier diff <watchlist-id>")
			os.Exit(2)
		}
		if err := runResearchDossierDiff(context.Background(), svc, args[1], os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "diff: %v\n", err)
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
			fmt.Fprintf(os.Stderr, "evidence: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "build stack: %v\n", err)
		os.Exit(1)
	}
	defer stack.Close()

	wls, err := stack.dossier.ListWatchlists(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "list watchlists: %v\n", err)
		os.Exit(1)
	}
	if len(wls) == 0 {
		fmt.Fprintln(os.Stderr, "no watchlists configured — apply some first with: pookie research watchlists apply --file <json>")
		os.Exit(1)
	}

	wf, err := stack.coord.SubmitWorkflow(context.Background(), engine.WorkflowDefinition{
		Name:  "Manual watchlist refresh",
		Skill: scheduler.SkillName,
		Input: map[string]any{"trigger": "cli"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "submit: %v\n", err)
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
		fmt.Fprintln(os.Stderr, "--mode is required (manual|hourly|daily)")
		os.Exit(2)
	case "manual", "hourly", "daily":
		// ok
	default:
		fmt.Fprintf(os.Stderr, "invalid mode %q; use manual|hourly|daily\n", *mode)
		os.Exit(2)
	}

	if err := writeVaultSecret("research_schedule", *mode); err != nil {
		fmt.Fprintf(os.Stderr, "write secret: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("research_schedule = %s\n", *mode)
}

// --- status ---

func cmdResearchStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	_ = fs.Parse(args)

	store := scheduler.NewStateStore(scheduler.DefaultStatePath(resolveRuntimeRoot()))
	st, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load state: %v\n", err)
		os.Exit(1)
	}
	svc := mustDossierService()
	wls, wlErr := svc.ListWatchlists(context.Background())

	if *jsonOut {
		payload := map[string]any{
			"scheduler": st,
		}
		if wlErr != nil {
			payload["watchlists_error"] = wlErr.Error()
		} else {
			payload["watchlists"] = len(wls)
		}
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
				fmt.Fprintf(os.Stderr, "show: %v\n", err)
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
				fmt.Fprintf(os.Stderr, "queue: %v\n", err)
				os.Exit(1)
			}
			return
		case "discard":
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "usage: pookie research recommendations discard <id>")
				os.Exit(2)
			}
			if err := runResearchRecommendationsDiscard(context.Background(), svc, args[1], os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "discard: %v\n", err)
				os.Exit(1)
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown recommendations subcommand: %s\n", args[0])
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
		fmt.Fprintf(os.Stderr, "invalid status %q; valid: draft|queued|submitted|discarded\n", *status)
		os.Exit(2)
	}

	svc := mustDossierService()
	recs, err := svc.ListRecommendations(context.Background(), dossier.RecommendationStatus(*status), 100)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "init dossier service: %v\n", err)
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
