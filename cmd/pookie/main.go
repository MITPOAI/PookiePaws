package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mitpoai/pookiepaws/internal/cli"
	"github.com/mitpoai/pookiepaws/internal/gateway"
)

const version = "0.3.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
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
	case "version", "--version", "-v":
		fmt.Printf("pookie %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	p := cli.Stdout()
	p.Banner()
	p.Plain("Usage:  pookie <command> [flags]")
	p.Blank()
	p.Plain("Commands:")
	p.Blank()
	p.Plain("  start              Boot the local agent and open the web console")
	p.Plain("  status             Check whether the agent is running")
	p.Plain("  run <skill>        Execute a marketing skill in this terminal")
	p.Plain("  install <repo>     Install a skill from a GitHub repository")
	p.Plain("  init               Interactive first-run setup wizard")
	p.Plain("  version            Print version and exit")
	p.Blank()
	p.Plain("Global flags:")
	p.Blank()
	p.Plain("  --addr  host:port   Listen address for start/status (default 127.0.0.1:18800)")
	p.Plain("  --home  path        Override runtime home directory")
	p.Blank()
	p.Dim("Source:  github.com/mitpoai/pookiepaws")
	p.Blank()
}

// ── pookie start ─────────────────────────────────────────────────────────────

func cmdStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:18800", "HTTP listen address")
	home := fs.String("home", "", "override runtime home directory")
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

	api := gateway.NewServer(gateway.Config{
		Coordinator: stack.coord,
		EventBus:    stack.bus,
		Brain:       stack.brainSvc,
		Vault:       stack.secrets,
		Address:     *addr,
	})

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           api.Handler(),
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
	<-stop

	p.Blank()
	p.Info("Shutting down…")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		p.Warning("HTTP shutdown: %v", err)
	}
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
