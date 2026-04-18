package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
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

  watchlists list                 Print configured watchlists
  watchlists apply --file <json>  Replace watchlists from JSON file (or --stdin)
  refresh                         Submit a watchlist refresh workflow now
  schedule --mode <m>             Set research schedule (manual|hourly|daily)
  status                          Show scheduler state
  recommendations [--status s]    List recommendations (draft|queued|submitted|discarded)
`)
}

// --- watchlists subcommand ---

func cmdResearchWatchlists(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: pookie research watchlists <list|apply>")
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
	default:
		fmt.Fprintln(os.Stderr, "usage: pookie research watchlists <list|apply>")
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
	_ = args
	store := scheduler.NewStateStore(scheduler.DefaultStatePath(resolveRuntimeRoot()))
	st, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load state: %v\n", err)
		os.Exit(1)
	}
	svc := mustDossierService()
	wls, _ := svc.ListWatchlists(context.Background())

	fmt.Printf("schedule:        %s\n", emptyDash(st.Schedule))
	fmt.Printf("watchlists:      %d\n", len(wls))
	fmt.Printf("last tick:       %s\n", formatTime(st.LastTickAt))
	fmt.Printf("last success:    %s\n", formatTime(st.LastSuccessAt))
	fmt.Printf("last workflow:   %s\n", emptyDash(st.LastWorkflow))
	fmt.Printf("next due:        %s\n", formatTime(st.NextDueAt))
	fmt.Printf("last error:      %s\n", emptyDash(st.LastError))
}

// --- recommendations ---

func cmdResearchRecommendations(args []string) {
	fs := flag.NewFlagSet("recommendations", flag.ExitOnError)
	status := fs.String("status", "", "Filter by status (draft|queued|submitted|discarded)")
	_ = fs.Parse(args)

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
