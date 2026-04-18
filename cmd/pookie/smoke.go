package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/adapters"
	"github.com/mitpoai/pookiepaws/internal/brain"
	"github.com/mitpoai/pookiepaws/internal/cli"
	"github.com/mitpoai/pookiepaws/internal/demo"
	"github.com/mitpoai/pookiepaws/internal/gateway"
	"github.com/mitpoai/pookiepaws/internal/security"
	"github.com/mitpoai/pookiepaws/internal/state"
)

type smokeCheck struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	Detail   string `json:"detail"`
	Duration string `json:"duration"`
}

type smokeScenarioSummary struct {
	Brand      string `json:"brand"`
	Competitor string `json:"competitor"`
	Market     string `json:"market"`
}

type smokeReport struct {
	Scope          string                `json:"scope"`
	Passed         bool                  `json:"passed"`
	Checks         []smokeCheck          `json:"checks"`
	Stopped        bool                  `json:"stopped_early,omitempty"`
	ArtifactPath   string                `json:"artifact_path,omitempty"`
	Mode           string                `json:"mode,omitempty"`
	Provider       string                `json:"provider,omitempty"`
	FallbackReason string                `json:"fallback_reason,omitempty"`
	SourceCount    int                   `json:"source_count,omitempty"`
	SkippedCount   int                   `json:"skipped_count,omitempty"`
	Warnings       []string              `json:"warnings,omitempty"`
	Scenario       *smokeScenarioSummary `json:"scenario,omitempty"`
}

type smokeOptions struct {
	providerOnly bool
	cliOnly      bool
	apiOnly      bool
	scenarioOnly bool
	scenarioLive bool
	all          bool
}

func cmdSmoke(args []string) {
	fs := flag.NewFlagSet("smoke", flag.ExitOnError)
	home := fs.String("home", "", "override runtime home directory for --provider")
	providerOnly := fs.Bool("provider", false, "validate the active saved provider configuration")
	cliOnly := fs.Bool("cli", false, "run deterministic CLI smoke checks with a mocked provider")
	apiOnly := fs.Bool("api", false, "run deterministic API smoke checks with a mocked provider")
	scenarioOnly := fs.Bool("scenario", false, "run the deterministic scenario smoke from workflow to saved export")
	scenarioLive := fs.Bool("scenario-live", false, "run the live bounded research smoke from workflow to saved export")
	all := fs.Bool("all", false, "run provider, CLI, scenario, live scenario when configured, and API smoke checks in order")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	_ = fs.Parse(args)

	options := smokeOptions{
		providerOnly: *providerOnly,
		cliOnly:      *cliOnly,
		apiOnly:      *apiOnly,
		scenarioOnly: *scenarioOnly,
		scenarioLive: *scenarioLive,
		all:          *all,
	}
	if !options.providerOnly && !options.cliOnly && !options.apiOnly && !options.scenarioOnly && !options.scenarioLive && !options.all {
		options.all = true
	}

	report := executeSmokeReport(*home, options)

	if *jsonOut {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(report)
		if !report.Passed {
			os.Exit(1)
		}
		return
	}

	p := cli.Stdout()
	p.Banner()
	p.Box("Smoke", [][2]string{
		{"scope", report.Scope},
		{"passed", fmt.Sprintf("%t", report.Passed)},
		{"checks", fmt.Sprintf("%d", len(report.Checks))},
	})
	p.Blank()
	for _, check := range report.Checks {
		status := "PASS"
		if !check.Passed {
			status = "FAIL"
		}
		p.Box(check.Name, [][2]string{
			{"status", status},
			{"detail", firstValue(check.Detail, "-")},
			{"duration", firstValue(check.Duration, "-")},
		})
		p.Blank()
	}
	if report.Scenario != nil {
		title := "Scenario Demo"
		if report.Mode == "live" {
			title = "Live Research Smoke"
		}
		rows := [][2]string{
			{"brand", report.Scenario.Brand},
			{"competitor", report.Scenario.Competitor},
			{"market", report.Scenario.Market},
			{"artifact", firstValue(report.ArtifactPath, "-")},
		}
		if report.Mode != "" {
			rows = append(rows, [2]string{"mode", report.Mode})
		}
		if report.Provider != "" {
			rows = append(rows, [2]string{"provider", report.Provider})
		}
		if report.SourceCount > 0 || report.Mode == "live" {
			rows = append(rows,
				[2]string{"sources", fmt.Sprintf("%d", report.SourceCount)},
				[2]string{"skipped", fmt.Sprintf("%d", report.SkippedCount)},
				[2]string{"warnings", fmt.Sprintf("%d", len(report.Warnings))},
			)
		}
		if report.FallbackReason != "" {
			rows = append(rows, [2]string{"fallback", report.FallbackReason})
		}
		p.Box(title, rows)
		p.Blank()
	}
	if !report.Passed {
		os.Exit(1)
	}
}

func executeSmokeReport(home string, options smokeOptions) smokeReport {
	report := smokeReport{Scope: smokeScope(options), Passed: true}

	if options.providerOnly || options.all {
		checks, ok := runProviderSmoke(home)
		report.Checks = append(report.Checks, checks...)
		if !ok {
			report.Passed = false
			if options.all {
				report.Stopped = true
				return report
			}
		}
	}
	if report.Passed && (options.cliOnly || options.all) {
		checks, ok := runCLISmoke()
		report.Checks = append(report.Checks, checks...)
		if !ok {
			report.Passed = false
			if options.all {
				report.Stopped = true
				return report
			}
		}
	}
	if report.Passed && (options.scenarioOnly || options.all) {
		checks, scenarioReport, ok := runScenarioSmoke(home)
		report.Checks = append(report.Checks, checks...)
		if scenarioReport != nil {
			report.ArtifactPath = scenarioReport.ArtifactPath
			report.Mode = scenarioReport.Mode
			report.Provider = scenarioReport.Provider
			report.FallbackReason = scenarioReport.FallbackReason
			report.SourceCount = scenarioReport.SourceCount
			report.SkippedCount = scenarioReport.SkippedCount
			report.Warnings = append([]string(nil), scenarioReport.Warnings...)
			report.Scenario = &smokeScenarioSummary{
				Brand:      scenarioReport.Scenario.Brand,
				Competitor: scenarioReport.Scenario.Competitor,
				Market:     scenarioReport.Scenario.Market,
			}
		}
		if !ok {
			report.Passed = false
			if options.all {
				report.Stopped = true
				return report
			}
		}
	}
	if report.Passed && (options.scenarioLive || (options.all && canRunLiveScenarioSmoke(home))) {
		checks, scenarioReport, ok := runScenarioLiveSmoke(home)
		report.Checks = append(report.Checks, checks...)
		if scenarioReport != nil {
			report.ArtifactPath = scenarioReport.ArtifactPath
			report.Mode = scenarioReport.Mode
			report.Provider = scenarioReport.Provider
			report.FallbackReason = scenarioReport.FallbackReason
			report.SourceCount = scenarioReport.SourceCount
			report.SkippedCount = scenarioReport.SkippedCount
			report.Warnings = append([]string(nil), scenarioReport.Warnings...)
			report.Scenario = &smokeScenarioSummary{
				Brand:      scenarioReport.Scenario.Brand,
				Competitor: scenarioReport.Scenario.Competitor,
				Market:     scenarioReport.Scenario.Market,
			}
		}
		if !ok {
			report.Passed = false
			if options.all {
				report.Stopped = true
				return report
			}
		}
	}
	if report.Passed && (options.apiOnly || options.all) {
		checks, ok := runAPISmoke()
		report.Checks = append(report.Checks, checks...)
		if !ok {
			report.Passed = false
		}
	}
	return report
}

func smokeScope(options smokeOptions) string {
	switch {
	case options.providerOnly:
		return "provider"
	case options.cliOnly:
		return "cli"
	case options.apiOnly:
		return "api"
	case options.scenarioOnly:
		return "scenario"
	case options.scenarioLive:
		return "scenario-live"
	default:
		return "all"
	}
}

func runProviderSmoke(home string) ([]smokeCheck, bool) {
	runtimeRoot, _, err := resolveSecretsPath(home)
	if err != nil {
		return []smokeCheck{failedSmoke("provider", err)}, false
	}
	stackSecrets, err := buildSecretsOnly(runtimeRoot)
	if err != nil {
		return []smokeCheck{failedSmoke("provider", err)}, false
	}
	return smokeStep("provider", func() (string, error) {
		health := brain.CheckProviderHealth(context.Background(), stackSecrets)
		if !health.Healthy() {
			return "", fmt.Errorf("%s (%s)", firstValue(health.Error, health.Detail, "provider validation failed"), brainHealthRemediation(health))
		}
		return fmt.Sprintf("Provider %s / %s is reachable.", firstValue(health.Provider, "configured"), firstValue(health.Model, "model")), nil
	})
}

func runCLISmoke() ([]smokeCheck, bool) {
	env, err := newSmokeEnvironment()
	if err != nil {
		return []smokeCheck{failedSmoke("cli.setup", err)}, false
	}
	defer env.Close()

	stack, err := buildStack(env.runtimeRoot, env.workspaceRoot)
	if err != nil {
		return []smokeCheck{failedSmoke("cli.stack", err)}, false
	}
	defer stack.Close()

	ctx := context.Background()
	checks := make([]smokeCheck, 0, 6)
	ok := true

	stepChecks := []struct {
		name string
		run  func() (string, error)
	}{
		{
			name: "cli.doctor",
			run: func() (string, error) {
				health := checkStackBrainHealth(ctx, stack)
				if !health.Healthy() {
					return "", fmt.Errorf(firstValue(health.Error, health.Detail, "brain health failed"))
				}
				return "Brain health checks passed for the mocked local provider.", nil
			},
		},
		{
			name: "cli.status",
			run: func() (string, error) {
				status, err := stack.coord.Status(ctx)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("Status snapshot readable with %d tracked workflows.", status.Workflows), nil
			},
		},
		{
			name: "cli.list",
			run: func() (string, error) {
				if len(stack.coord.SkillDefinitions()) == 0 {
					return "", fmt.Errorf("no skills registered")
				}
				return fmt.Sprintf("%d built-in skills are registered.", len(stack.coord.SkillDefinitions())), nil
			},
		},
		{
			name: "cli.sessions",
			run: func() (string, error) {
				session, err := loadOrCreateChatSession(ctx, stack.store, "")
				if err != nil {
					return "", err
				}
				run, sessionState, err := beginChatRun(ctx, stack.store, session.ID, "hello")
				if err != nil {
					return "", err
				}
				result, err := stack.brainSvc.DispatchPrompt(ctx, "hello")
				if err != nil {
					return "", err
				}
				_ = finishChatRunSuccess(ctx, stack.store, sessionState, run.ID, result)
				sessions, err := stack.store.ListSessions(ctx)
				if err != nil {
					return "", err
				}
				if len(sessions) == 0 {
					return "", fmt.Errorf("chat session was not persisted")
				}
				return fmt.Sprintf("Chat session %s was persisted.", sessions[0].ID), nil
			},
		},
		{
			name: "cli.approvals",
			run: func() (string, error) {
				approvals, err := stack.coord.ListApprovals(ctx)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("Approval queue is readable with %d pending records.", len(approvals)), nil
			},
		},
		{
			name: "cli.audit",
			run: func() (string, error) {
				entries, err := state.ReadRecentAuditEntries(filepath.Join(env.runtimeRoot, "state"), 5)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("Audit tail remains readable with %d recent entries.", len(entries)), nil
			},
		},
	}

	for _, step := range stepChecks {
		current, passed := smokeStep(step.name, step.run)
		checks = append(checks, current...)
		if !passed {
			ok = false
			break
		}
	}
	return checks, ok
}

func runAPISmoke() ([]smokeCheck, bool) {
	env, err := newSmokeEnvironment()
	if err != nil {
		return []smokeCheck{failedSmoke("api.setup", err)}, false
	}
	defer env.Close()

	stack, err := buildStack(env.runtimeRoot, env.workspaceRoot)
	if err != nil {
		return []smokeCheck{failedSmoke("api.stack", err)}, false
	}
	defer stack.Close()

	server := gateway.NewServer(gateway.Config{
		Coordinator: stack.coord,
		EventBus:    stack.bus,
		Brain:       stack.brainSvc,
		Store:       stack.store,
		Vault:       stack.secrets,
		WhatsApp:    adapters.NewMockWhatsAppAdapter(),
		Dossier:     stack.dossier,
		Address:     "127.0.0.1:0",
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := httpServer.Client()
	checks := make([]smokeCheck, 0, 8)
	ok := true

	stepChecks := []struct {
		name string
		run  func() (string, error)
	}{
		{name: "api.healthz", run: func() (string, error) {
			return "Health endpoint returned 200.", expectStatus(client, httpServer.URL+"/healthz", http.StatusOK)
		}},
		{name: "api.readyz", run: func() (string, error) {
			return "Readiness endpoint returned 200.", expectStatus(client, httpServer.URL+"/readyz", http.StatusOK)
		}},
		{name: "api.status", run: func() (string, error) {
			return "Status endpoint returned 200.", expectStatus(client, httpServer.URL+"/api/v1/status", http.StatusOK)
		}},
		{name: "api.console", run: func() (string, error) {
			return "Console snapshot returned 200.", expectStatus(client, httpServer.URL+"/api/v1/console", http.StatusOK)
		}},
		{name: "api.diagnostics", run: func() (string, error) {
			return "Diagnostics endpoint returned 200.", expectStatus(client, httpServer.URL+"/api/v1/diagnostics", http.StatusOK)
		}},
		{
			name: "api.chat.sessions",
			run: func() (string, error) {
				request, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/v1/chat/sessions", nil)
				if err != nil {
					return "", err
				}
				response, err := client.Do(request)
				if err != nil {
					return "", err
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusCreated {
					return "", fmt.Errorf("expected status %d, got %d", http.StatusCreated, response.StatusCode)
				}
				return "Chat session endpoint created a new session.", nil
			},
		},
		{
			name: "api.brain.dispatch",
			run: func() (string, error) {
				body := strings.NewReader(`{"prompt":"hello"}`)
				request, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/v1/brain/dispatch", body)
				if err != nil {
					return "", err
				}
				request.Header.Set("Content-Type", "application/json")
				response, err := client.Do(request)
				if err != nil {
					return "", err
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					return "", fmt.Errorf("expected status %d, got %d", http.StatusOK, response.StatusCode)
				}
				return "Brain dispatch endpoint accepted a structured prompt.", nil
			},
		},
		{
			name: "api.events",
			run: func() (string, error) {
				request, err := http.NewRequest(http.MethodGet, httpServer.URL+"/api/v1/events", nil)
				if err != nil {
					return "", err
				}
				request.Header.Set("Accept", "text/event-stream")
				response, err := client.Do(request)
				if err != nil {
					return "", err
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					return "", fmt.Errorf("expected status %d, got %d", http.StatusOK, response.StatusCode)
				}
				if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
					return "", fmt.Errorf("unexpected content type %q", contentType)
				}
				return "Event stream is reachable with SSE headers.", nil
			},
		},
	}

	for _, step := range stepChecks {
		current, passed := smokeStep(step.name, step.run)
		checks = append(checks, current...)
		if !passed {
			ok = false
			break
		}
	}
	return checks, ok
}

func runScenarioSmoke(home string) ([]smokeCheck, *demo.Result, bool) {
	runtimeRoot, workspaceRoot, err := resolveRoots(home)
	if err != nil {
		return []smokeCheck{failedSmoke("scenario.setup", err)}, nil, false
	}

	stack, err := buildStack(runtimeRoot, workspaceRoot)
	if err != nil {
		return []smokeCheck{failedSmoke("scenario.stack", err)}, nil, false
	}
	defer stack.Close()

	result, err := demo.RunScenarioSmoke(context.Background(), stack.coord, runtimeRoot, workspaceRoot)
	if err != nil {
		checks := make([]smokeCheck, 0, len(result.Checks))
		for _, check := range result.Checks {
			checks = append(checks, smokeCheck{
				Name:     check.Name,
				Passed:   check.Passed,
				Detail:   check.Detail,
				Duration: check.Duration,
			})
		}
		if len(checks) == 0 {
			checks = append(checks, failedSmoke("scenario", err))
		}
		return checks, &result, false
	}

	checks := make([]smokeCheck, 0, len(result.Checks))
	for _, check := range result.Checks {
		checks = append(checks, smokeCheck{
			Name:     check.Name,
			Passed:   check.Passed,
			Detail:   check.Detail,
			Duration: check.Duration,
		})
	}
	return checks, &result, true
}

func runScenarioLiveSmoke(home string) ([]smokeCheck, *demo.Result, bool) {
	runtimeRoot, workspaceRoot, err := resolveRoots(home)
	if err != nil {
		return []smokeCheck{failedSmoke("scenario.live.setup", err)}, nil, false
	}

	stack, err := buildStack(runtimeRoot, workspaceRoot)
	if err != nil {
		return []smokeCheck{failedSmoke("scenario.live.stack", err)}, nil, false
	}
	defer stack.Close()

	result, err := demo.RunScenarioLiveSmoke(context.Background(), stack.coord, runtimeRoot, workspaceRoot)
	if err != nil {
		checks := make([]smokeCheck, 0, len(result.Checks))
		for _, check := range result.Checks {
			checks = append(checks, smokeCheck{
				Name:     check.Name,
				Passed:   check.Passed,
				Detail:   check.Detail,
				Duration: check.Duration,
			})
		}
		if len(checks) == 0 {
			checks = append(checks, failedSmoke("scenario.live", err))
		}
		return checks, &result, false
	}

	checks := make([]smokeCheck, 0, len(result.Checks))
	for _, check := range result.Checks {
		checks = append(checks, smokeCheck{
			Name:     check.Name,
			Passed:   check.Passed,
			Detail:   check.Detail,
			Duration: check.Duration,
		})
	}
	return checks, &result, true
}

type smokeEnvironment struct {
	runtimeRoot   string
	workspaceRoot string
	server        *httptest.Server
}

func newSmokeEnvironment() (*smokeEnvironment, error) {
	root, err := os.MkdirTemp("", "pookie-smoke-*")
	if err != nil {
		return nil, err
	}
	runtimeRoot := filepath.Join(root, ".pookiepaws")
	workspaceRoot := filepath.Join(runtimeRoot, "workspace")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "mock-model"}},
			})
		case "/v1/chat/completions":
			if auth := r.Header.Get("Authorization"); auth != "Bearer sk-mock" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{"message": "unauthorized"},
				})
				return
			}
			var payload struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{"message": err.Error()},
				})
				return
			}
			if strings.TrimSpace(payload.Model) != "mock-model" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{"message": payload.Model + " is not a valid model ID"},
				})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": "mock-model",
				"choices": []map[string]any{
					{"message": map[string]string{"content": `{"action":"casual_chat","explanation":"Mock provider is healthy."}`}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))

	payload := map[string]string{
		"llm_provider": "openai-compatible",
		"llm_base_url": server.URL + "/v1/chat/completions",
		"llm_model":    "mock-model",
		"llm_api_key":  "sk-mock",
	}
	if err := saveSecurityConfig(runtimeRoot, filepath.Join(runtimeRoot, ".security.json"), payload); err != nil {
		server.Close()
		_ = os.RemoveAll(root)
		return nil, err
	}
	return &smokeEnvironment{
		runtimeRoot:   runtimeRoot,
		workspaceRoot: workspaceRoot,
		server:        server,
	}, nil
}

func (e *smokeEnvironment) Close() {
	if e == nil {
		return
	}
	if e.server != nil {
		e.server.Close()
	}
	_ = os.RemoveAll(filepath.Dir(e.runtimeRoot))
}

func buildSecretsOnly(runtimeRoot string) (brain.SecretReader, error) {
	return security.NewJSONSecretProvider(runtimeRoot)
}

func canRunLiveScenarioSmoke(home string) bool {
	runtimeRoot, _, err := resolveSecretsPath(home)
	if err != nil {
		return false
	}
	secrets, err := buildSecretsOnly(runtimeRoot)
	if err != nil {
		return false
	}
	preference, err := secrets.Get("research_provider")
	if err != nil || strings.TrimSpace(preference) == "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(preference)) {
	case "internal", "auto":
		return true
	case "firecrawl":
		value, err := secrets.Get("firecrawl_api_key")
		return err == nil && strings.TrimSpace(value) != ""
	case "jina":
		return false
	default:
		return true
	}
}

func expectStatus(client *http.Client, rawURL string, expected int) error {
	request, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != expected {
		return fmt.Errorf("expected status %d, got %d", expected, response.StatusCode)
	}
	return nil
}

func smokeStep(name string, run func() (string, error)) ([]smokeCheck, bool) {
	started := time.Now()
	detail, err := run()
	if err != nil {
		return []smokeCheck{{
			Name:     name,
			Passed:   false,
			Detail:   err.Error(),
			Duration: time.Since(started).Round(time.Millisecond).String(),
		}}, false
	}
	return []smokeCheck{{
		Name:     name,
		Passed:   true,
		Detail:   firstValue(detail, "ok"),
		Duration: time.Since(started).Round(time.Millisecond).String(),
	}}, true
}

func failedSmoke(name string, err error) smokeCheck {
	return smokeCheck{
		Name:     name,
		Passed:   false,
		Detail:   err.Error(),
		Duration: "0s",
	}
}
