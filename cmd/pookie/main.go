package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mitpoai/pookiepaws/internal/adapters"
	"github.com/mitpoai/pookiepaws/internal/cli"
	"github.com/mitpoai/pookiepaws/internal/gateway"
)

var version = "0.5.2"

func main() {
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
	case "version", "--version", "-v":
		cmdVersion(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func launchInteractiveMenu() {
	defer maybeShowUpdateNotice(context.Background(), version, os.Stderr, "")
	p := cli.Stdout()
	p.Banner()

	items := []string{
		"Start Web UI & Daemon",
		"Chat with Pookie (AI Mode)",
		"List Marketing Skills",
		"Run Specific Skill",
		"Exit",
	}

	choice := cli.RunMenu(p, "What would you like to do?", items)
	p.Blank()

	switch choice {
	case 0:
		cmdStart(nil)
	case 1:
		cmdChat(nil)
	case 2:
		cmdList(nil)
	case 3:
		cmdRun(nil)
	case 4:
		p.Dim("Goodbye! \u2014 Pookie")
		p.Blank()
	}
}

func printUsage() {
	p := cli.Stdout()
	p.Banner()
	p.Plain("Usage:  pookie [command] [flags]")
	p.Blank()
	p.Accent("Commands:")
	p.Blank()
	p.Plain("  start              Boot the local agent and open the web console")
	p.Plain("  chat               Talk to Pookie in your terminal (AI mode)")
	p.Plain("  list               Show all installed marketing skills")
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
	p.Blank()
	p.Accent("Flags:")
	p.Blank()
	p.Plain("  -v, --version       Print version and build info")
	p.Plain("  -h, --help          Show this help message")
	p.Plain("      --addr          Listen address for start/status (default 127.0.0.1:18800)")
	p.Plain("      --home          Override runtime home directory")
	p.Plain("      --verbose       Print request timing logs (for start)")
	p.Blank()
	p.Dim("Run pookie with no arguments for an interactive menu.")
	p.Dim("Source:  github.com/mitpoai/pookiepaws")
	p.Blank()
}

// ── pookie start ─────────────────────────────────────────────────────────────

func cmdStart(args []string) {
	defer maybeShowUpdateNotice(context.Background(), version, os.Stderr, "")
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:18800", "HTTP listen address")
	home := fs.String("home", "", "override runtime home directory")
	verbose := fs.Bool("verbose", false, "print request timing logs")
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

	shutdown := make(chan struct{}, 1)
	api := gateway.NewServer(gateway.Config{
		Coordinator: stack.coord,
		EventBus:    stack.bus,
		Brain:       stack.brainSvc,
		Store:       stack.store,
		Vault:       stack.secrets,
		WhatsApp:    adapters.NewWhatsAppAdapter(),
		Address:     *addr,
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
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:18800", "agent address")
	_ = fs.Parse(args)

	p := cli.Stdout()
	p.Banner()

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + *addr + "/api/v1/status")
	if err != nil {
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
		p.Error("Could not parse status response: %v", err)
		os.Exit(1)
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
