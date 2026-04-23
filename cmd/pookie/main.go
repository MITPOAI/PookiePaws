package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mitpoai/pookiepaws/internal/adapters"
	"github.com/mitpoai/pookiepaws/internal/cli"
	"github.com/mitpoai/pookiepaws/internal/dossier"
	"github.com/mitpoai/pookiepaws/internal/gateway"
	"github.com/mitpoai/pookiepaws/internal/scheduler"
	"github.com/mitpoai/pookiepaws/internal/security"
)

var version = "1.1.0"

func main() {
	initLogger()
	// No arguments → launch interactive menu.
	if len(os.Args) < 2 {
		launchInteractiveMenu()
		return
	}

	switch os.Args[1] {
	case "start":
		cmdStart(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "research":
		cmdResearch(os.Args[2:])
	case "run":
		cmdRun(os.Args[2:])
	case "install":
		cmdInstall(os.Args[2:])
	case "init":
		cmdInit(os.Args[2:])
	case "chat":
		cmdChat(os.Args[2:])
	case "list":
		cmdList(os.Args[2:])
	case "sessions":
		cmdSessions(os.Args[2:])
	case "approvals":
		cmdApprovals(os.Args[2:])
	case "audit":
		cmdAudit(os.Args[2:])
	case "doctor":
		cmdDoctor(os.Args[2:])
	case "smoke":
		cmdSmoke(os.Args[2:])
	case "context":
		cmdContext(os.Args[2:])
	case "memory":
		cmdMemory(os.Args[2:])
	case "completion":
		cmdCompletion(os.Args[2:])
	case "version", "--version", "-v":
		cmdVersion(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		printUnknownCommand(os.Args[1])
		os.Exit(1)
	}
}

func launchInteractiveMenu() {
	defer maybeShowUpdateNotice(context.Background(), version, os.Stderr, "")
	p := cli.Stdout()
	p.Banner()

	items := []cli.MenuItem{
		{
			Label: "Open Home Console & Daemon",
			Hint:  "Start the local runtime, web console, and recurring research scheduler.",
		},
		{
			Label: "Guided Competitor Research",
			Hint:  "Prompt for company, competitors, domains, and schedule, then save a local dossier.",
		},
		{
			Label: "Research Status",
			Hint:  "Show scheduler state, latest dossier, and latest local export path.",
		},
		{
			Label: "Recent Dossiers",
			Hint:  "List saved competitor dossiers and recent analysis runs.",
		},
		{
			Label: "Chat with Pookie (AI Mode)",
			Hint:  "Open the terminal chat interface.",
		},
		{
			Label: "List Marketing Skills",
			Hint:  "Show executable skills. This is different from top-level commands like research or doctor.",
		},
		{
			Label: "Run Specific Skill",
			Hint:  "Execute a skill directly by name.",
		},
		{
			Label: "Diagnostics",
			Hint:  "Open doctor to inspect runtime, brain, scheduler, and latest research state.",
		},
		{
			Label: "Exit",
			Hint:  "Leave the operator menu.",
		},
	}

	choice, _ := cli.NewWizard(p).Select(
		"What would you like to do?",
		"Use competitor research for command-driven analysis and `list` only for installed skills.",
		items,
		len(items)-1,
	)
	p.Blank()

	switch choice {
	case 0:
		cmdStart(nil)
	case 1:
		launchGuidedResearchAnalyze()
	case 2:
		cmdResearch([]string{"status"})
	case 3:
		cmdResearch([]string{"dossier", "list"})
	case 4:
		cmdChat(nil)
	case 5:
		cmdList(nil)
	case 6:
		cmdRun(nil)
	case 7:
		cmdDoctor(nil)
	case 8:
		p.Dim("Goodbye! \u2014 Pookie")
		p.Blank()
	}
}

func printUsage() {
	defer maybeShowUpdateNotice(context.Background(), version, os.Stderr, "")
	p := cli.Stdout()
	p.Banner()
	printUsageBody(p)
}

func printUsageBody(p *cli.Printer) {
	p.Plain("Usage:  pookie [command] [flags]")
	p.Blank()
	p.Accent("Commands:")
	p.Blank()
	p.Plain("  start              Boot the local agent and open the Home console")
	p.Plain("  chat               Talk to Pookie in your terminal (AI mode)")
	p.Plain("  list               Show all installed marketing skills")
	p.Plain("  research <sub>     Analyze competitors, manage watchlists, scheduler, and dossiers")
	p.Plain("  run <skill>        Execute a marketing skill in this terminal")
	p.Plain("  status             Check whether the agent is running")
	p.Plain("  sessions           Inspect persisted control-plane sessions")
	p.Plain("  approvals          Review or resolve pending approvals")
	p.Plain("  audit              Tail recent audit events from local state")
	p.Plain("  doctor             Print local runtime diagnostics")
	p.Plain("  smoke              Run operator smoke checks for provider, CLI, and API")
	p.Plain("  context            Inspect the current prompt, memory, and variables")
	p.Plain("  memory             Manage persistent brain memory (prune, inspect)")
	p.Plain("  install <repo>     Install a skill from a GitHub repository")
	p.Plain("  init               Interactive first-run setup wizard")
	p.Plain("  completion <shell> Generate shell completion (bash|zsh|fish|powershell)")
	p.Blank()
	p.Accent("Quick Start:")
	p.Blank()
	p.Plain("  pookie research analyze --company \"PookiePaws\" --competitors \"OpenClaw,PetBox\"")
	p.Plain("  pookie research status")
	p.Plain("  pookie research dossier list")
	p.Blank()
	p.Accent("Flags:")
	p.Blank()
	p.Plain("  -v, --version       Print version and build info (use --check to force a live release lookup)")
	p.Plain("  -h, --help          Show this help message")
	p.Plain("      --addr          Listen address for start/status (default 127.0.0.1:18800)")
	p.Plain("      --home          Override runtime home directory")
	p.Plain("      --verbose       Print request timing logs (for start)")
	p.Blank()
	p.Dim("Most diagnostic commands accept --json for machine-readable output.")
	p.Dim("Run pookie with no arguments for the interactive operator menu.")
	p.Dim("`pookie list` shows installed skills. `pookie --help` shows top-level commands.")
	p.Dim("The web console is organised around Home, Run, Review, and Settings.")
	p.Dim("Source:  github.com/mitpoai/pookiepaws")
	p.Blank()
}

func printUnknownCommand(command string) {
	defer maybeShowUpdateNotice(context.Background(), version, os.Stderr, "")
	p := cli.Stdout()
	p.Banner()
	p.Error("Unknown command: %s", command)
	if suggestion := suggestCommand(command, topLevelCommands); suggestion != "" {
		p.Info("Did you mean: pookie %s", suggestion)
	}
	p.Dim("Top-level commands live under `pookie --help`.")
	p.Blank()
	printUsageBody(p)
}

func launchGuidedResearchAnalyze() {
	p := cli.Stdout()
	p.Banner()
	p.Accent("Guided Competitor Research")
	p.Dim("Press Enter to skip optional fields. Company is required.")
	p.Blank()

	company, ok := cli.ReadLine(p, "Company > ")
	if !ok || strings.TrimSpace(company) == "" {
		p.Warning("Research setup cancelled.")
		p.Blank()
		return
	}
	competitors, _ := cli.ReadLine(p, "Competitors (comma-separated) > ")
	domains, _ := cli.ReadLine(p, "Domains (comma-separated) > ")
	focusAreas, _ := cli.ReadLine(p, "Focus areas (comma-separated) > ")
	market, _ := cli.ReadLine(p, "Market (optional) > ")

	schedule := scheduler.ModeManual
	selection, _ := cli.NewWizard(p).Select(
		"Research Schedule",
		"Manual runs once. Hourly and daily keep checking online while `pookie start` is running.",
		[]cli.MenuItem{
			{Label: "Manual", Hint: "Run once and save local state only."},
			{Label: "Hourly", Hint: "Keep refreshing the saved watchlist every hour while the daemon is running."},
			{Label: "Daily", Hint: "Keep refreshing the saved watchlist every day while the daemon is running."},
		},
		0,
	)
	switch selection {
	case 1:
		schedule = scheduler.ModeHourly
	case 2:
		schedule = scheduler.ModeDaily
	}

	runtimeRoot := resolveRuntimeRoot()
	svc, err := dossier.NewService(runtimeRoot)
	if err != nil {
		p.Error("init dossier service: %v", err)
		p.Blank()
		return
	}
	secrets, err := security.NewJSONSecretProvider(runtimeRoot)
	if err != nil {
		p.Error("open secrets vault: %v", err)
		p.Blank()
		return
	}

	err = runResearchAnalyze(
		context.Background(),
		svc,
		secrets,
		researchAnalyzeOptions{
			Company:     strings.TrimSpace(company),
			Competitors: splitCSVArgs(competitors),
			Domains:     splitCSVArgs(domains),
			FocusAreas:  splitCSVArgs(focusAreas),
			Market:      strings.TrimSpace(market),
			Schedule:    schedule,
		},
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
		p.Blank()
		return
	}
	if schedule != scheduler.ModeManual {
		p.Info("Recurring research is set to %s. Keep `pookie start` running for scheduled refreshes.", schedule)
		p.Blank()
	}
}

// ── pookie start ─────────────────────────────────────────────────────────────

func cmdStart(args []string) {
	defer maybeShowUpdateNotice(context.Background(), version, os.Stderr, "")
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:18800", "HTTP listen address")
	home := fs.String("home", "", "override runtime home directory")
	verbose := fs.Bool("verbose", false, "print request timing logs")
	maxBody := fs.Int64("max-body-bytes", gateway.DefaultMaxBodyBytes, "max gateway request body size in bytes (-1 disables)")
	_ = fs.Parse(args)

	p := cli.Stdout()
	p.Banner()

	runtimeRoot, workspaceRoot, err := resolveRoots(*home)
	if err != nil {
		p.Error("resolve runtime path: %v", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		p.Error("create workspace: %v", err)
		os.Exit(1)
	}

	spin := p.NewSpinner("Initialising engine…")
	spin.Start()

	stack, err := buildStack(runtimeRoot, workspaceRoot)
	if err != nil {
		spin.Stop(false, "Engine initialisation failed")
		p.Error("%v", err)
		os.Exit(1)
	}
	defer stack.Close()
	spin.Stop(true, "Engine ready")

	if !stack.brainSvc.Available() {
		p.Warning("No LLM provider configured — run  pookie init  to set one up")
	}
	for _, warning := range startupWarnings(stack.secrets) {
		p.Warning(warning)
	}
	if !isLoopbackAddr(*addr) {
		p.Warning("listening on a non-loopback address (%s) — gateway is unauthenticated; only do this on a trusted network", *addr)
	}

	if migrated, err := dossier.MigrateLegacyWatchlists(context.Background(), stack.dossier, stack.secrets); err != nil {
		slog.Warn("legacy watchlist migration failed", "err", err)
	} else if migrated > 0 {
		slog.Info("migrated legacy watchlists", "count", migrated)
	}

	schedCtx, cancelSched := context.WithCancel(context.Background())
	defer cancelSched()
	go func() {
		sched := scheduler.New(scheduler.Config{
			Coordinator:  stack.coord,
			Secrets:      stack.secrets,
			StateStore:   scheduler.NewStateStore(scheduler.DefaultStatePath(runtimeRoot)),
			MaxLastRunAt: stack.dossier.MaxLastRunAt,
			Logger:       schedulerLoggerAdapter(slog.Default()),
		})
		sched.Run(schedCtx)
	}()

	shutdown := make(chan struct{}, 1)
	api := gateway.NewServer(gateway.Config{
		Coordinator:  stack.coord,
		EventBus:     stack.bus,
		Brain:        stack.brainSvc,
		Store:        stack.store,
		Vault:        stack.secrets,
		WhatsApp:     adapters.NewWhatsAppAdapter(),
		Dossier:      stack.dossier,
		Address:      *addr,
		AppVersion:   version,
		MaxBodyBytes: *maxBody,
		RequestShutdown: func() {
			select {
			case shutdown <- struct{}{}:
			default:
			}
		},
	})

	handler := api.Handler()
	if *verbose {
		p.Info("Verbose mode enabled — timing logs active")
		handler = timingMiddleware(handler, p)
	}

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		// WriteTimeout is intentionally unset (0): the SSE handler at
		// /api/v1/events holds connections open indefinitely with a 20s
		// keepalive. A global write deadline would tear those streams
		// down. Real protection comes from ReadHeaderTimeout, IdleTimeout,
		// and the request-body cap installed by the gateway.
		IdleTimeout: 60 * time.Second,
	}

	p.Success("Console ready at  http://%s", *addr)
	p.Info("Press Ctrl+C to stop")
	p.Blank()

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			p.Error("HTTP server error: %v", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stop:
	case <-shutdown:
	}
	signal.Stop(stop)

	p.Blank()
	p.Info("Shutting down gracefully…")

	cancelSched()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		p.Warning("HTTP shutdown: %v", err)
	}
	stack.Close()
	p.Success("Stopped cleanly. See you next time!")
	p.Blank()
}

// ── pookie status ────────────────────────────────────────────────────────────

func cmdStatus(args []string) {
	defer maybeShowUpdateNotice(context.Background(), version, os.Stderr, "")
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:18800", "agent address")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	_ = fs.Parse(args)

	p := cli.Stdout()
	if !*jsonOut {
		p.Banner()
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + *addr + "/api/v1/status")
	if err != nil {
		if *jsonOut {
			emitJSONOrExit(map[string]any{
				"running": false,
				"address": *addr,
				"error":   err.Error(),
			})
			os.Exit(1)
		}
		p.Error("Agent is not running at %s", *addr)
		p.Blank()
		p.Dim("Start it with:  pookie start")
		p.Blank()
		os.Exit(1)
	}
	defer resp.Body.Close()

	var snap struct {
		RuntimeRoot            string    `json:"runtime_root"`
		WorkspaceRoot          string    `json:"workspace_root"`
		Workflows              int       `json:"workflows"`
		PendingApprovals       int       `json:"pending_approvals"`
		PendingFilePermissions int       `json:"pending_file_permissions"`
		StartedAt              time.Time `json:"started_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		if *jsonOut {
			emitJSONOrExit(map[string]any{
				"running": true,
				"address": *addr,
				"error":   "decode: " + err.Error(),
			})
			os.Exit(1)
		}
		p.Error("Could not parse status response: %v", err)
		os.Exit(1)
	}

	if *jsonOut {
		emitJSONOrExit(map[string]any{
			"running":                  true,
			"address":                  *addr,
			"uptime_seconds":           int64(time.Since(snap.StartedAt).Seconds()),
			"runtime_root":             snap.RuntimeRoot,
			"workspace_root":           snap.WorkspaceRoot,
			"workflows":                snap.Workflows,
			"pending_approvals":        snap.PendingApprovals,
			"pending_file_permissions": snap.PendingFilePermissions,
			"started_at":               snap.StartedAt,
		})
		return
	}

	uptime := time.Since(snap.StartedAt).Round(time.Second)
	p.Box("Agent Status", [][2]string{
		{"status", "running"},
		{"address", *addr},
		{"uptime", uptime.String()},
		{"workflows", fmt.Sprintf("%d", snap.Workflows)},
		{"pending approvals", fmt.Sprintf("%d", snap.PendingApprovals)},
		{"file permissions", fmt.Sprintf("%d", snap.PendingFilePermissions)},
		{"runtime root", snap.RuntimeRoot},
		{"workspace", snap.WorkspaceRoot},
	})
	p.Blank()
}

func startupWarnings(secrets interface {
	Get(name string) (string, error)
}) []string {
	if secrets == nil {
		return nil
	}

	checks := []struct {
		label    string
		required []string
	}{
		{label: "brain", required: []string{"llm_base_url", "llm_model"}},
		{label: "salesmanago", required: []string{"salesmanago_api_key", "salesmanago_base_url"}},
		{label: "mitto", required: []string{"mitto_api_key", "mitto_base_url", "mitto_from"}},
		{label: "whatsapp", required: []string{"whatsapp_access_token", "whatsapp_phone_number_id"}},
	}

	warnings := make([]string, 0, len(checks))
	for _, check := range checks {
		present := 0
		missing := make([]string, 0, len(check.required))
		for _, key := range check.required {
			value, err := secrets.Get(key)
			if err == nil && strings.TrimSpace(value) != "" {
				present++
				continue
			}
			missing = append(missing, key)
		}
		if present == 0 || len(missing) == 0 {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("%s configuration is incomplete — missing %s", check.label, strings.Join(missing, ", ")))
	}
	return warnings
}

func timingMiddleware(next http.Handler, p *cli.Printer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		elapsed := time.Since(start)
		p.Dim("[%s] %s %s  %s", elapsed.Round(time.Microsecond), r.Method, r.URL.Path, r.RemoteAddr)
	})
}

// isLoopbackAddr reports whether the given listen address binds only to a
// loopback interface. An empty host (":18800") binds to all interfaces and
// is therefore not loopback-only.
func isLoopbackAddr(addr string) bool {
	addr = strings.TrimSpace(addr)
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.TrimSpace(host)
	if host == "" {
		// ":port" — binds to all interfaces.
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}
