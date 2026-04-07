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
	"github.com/mitpoai/pookiepaws/internal/gateway"
	"github.com/mitpoai/pookiepaws/internal/security"
)

type smokeCheck struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	Detail   string `json:"detail"`
	Duration string `json:"duration"`
}

type smokeReport struct {
	Scope   string       `json:"scope"`
	Passed  bool         `json:"passed"`
	Checks  []smokeCheck `json:"checks"`
	Stopped bool         `json:"stopped_early,omitempty"`
}

func cmdSmoke(args []string) {
	fs := flag.NewFlagSet("smoke", flag.ExitOnError)
	home := fs.String("home", "", "override runtime home directory for --provider")
	providerOnly := fs.Bool("provider", false, "validate the active saved provider configuration")
	cliOnly := fs.Bool("cli", false, "run deterministic CLI smoke checks with a mocked provider")
	apiOnly := fs.Bool("api", false, "run deterministic API smoke checks with a mocked provider")
	all := fs.Bool("all", false, "run provider, CLI, and API smoke checks in order")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	_ = fs.Parse(args)

	scope := "all"
	switch {
	case *providerOnly:
		scope = "provider"
	case *cliOnly:
		scope = "cli"
	case *apiOnly:
		scope = "api"
	case *all:
		scope = "all"
	}

	if !*providerOnly && !*cliOnly && !*apiOnly && !*all {
		*all = true
	}

	report := smokeReport{Scope: scope, Passed: true}
	if *providerOnly || *all {
		checks, ok := runProviderSmoke(*home)
		report.Checks = append(report.Checks, checks...)
		if !ok {
			report.Passed = false
			if *all {
				report.Stopped = true
			}
		}
	}
	if report.Passed && (*cliOnly || *all) {
		checks, ok := runCLISmoke()
		report.Checks = append(report.Checks, checks...)
		if !ok {
			report.Passed = false
			if *all {
				report.Stopped = true
			}
		}
	}
	if report.Passed && (*apiOnly || *all) {
		checks, ok := runAPISmoke()
		report.Checks = append(report.Checks, checks...)
		if !ok {
			report.Passed = false
		}
	}

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
	if !report.Passed {
		os.Exit(1)
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
	return smokeStep("provider", func() error {
		health := brain.CheckProviderHealth(context.Background(), stackSecrets)
		if !health.Healthy() {
			return fmt.Errorf("%s (%s)", firstValue(health.Error, health.Detail, "provider validation failed"), brainHealthRemediation(health))
		}
		return nil
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
		run  func() error
	}{
		{
			name: "cli.doctor",
			run: func() error {
				health := checkStackBrainHealth(ctx, stack)
				if !health.Healthy() {
					return fmt.Errorf(firstValue(health.Error, health.Detail, "brain health failed"))
				}
				return nil
			},
		},
		{
			name: "cli.status",
			run: func() error {
				_, err := stack.coord.Status(ctx)
				return err
			},
		},
		{
			name: "cli.list",
			run: func() error {
				if len(stack.coord.SkillDefinitions()) == 0 {
					return fmt.Errorf("no skills registered")
				}
				return nil
			},
		},
		{
			name: "cli.sessions",
			run: func() error {
				session, err := loadOrCreateChatSession(ctx, stack.store, "")
				if err != nil {
					return err
				}
				run, sessionState, err := beginChatRun(ctx, stack.store, session.ID, "hello")
				if err != nil {
					return err
				}
				result, err := stack.brainSvc.DispatchPrompt(ctx, "hello")
				if err != nil {
					return err
				}
				_ = finishChatRunSuccess(ctx, stack.store, sessionState, run.ID, result)
				sessions, err := stack.store.ListSessions(ctx)
				if err != nil {
					return err
				}
				if len(sessions) == 0 {
					return fmt.Errorf("chat session was not persisted")
				}
				return nil
			},
		},
		{
			name: "cli.approvals",
			run: func() error {
				_, err := stack.coord.ListApprovals(ctx)
				return err
			},
		},
		{
			name: "cli.audit",
			run: func() error {
				path := filepath.Join(env.runtimeRoot, "state", "audits", "audit.jsonl")
				if _, err := tailLines(path, 5); err != nil {
					return err
				}
				return nil
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
		Address:     "127.0.0.1:0",
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	client := httpServer.Client()
	checks := make([]smokeCheck, 0, 8)
	ok := true

	stepChecks := []struct {
		name string
		run  func() error
	}{
		{name: "api.healthz", run: func() error { return expectStatus(client, httpServer.URL+"/healthz", http.StatusOK) }},
		{name: "api.readyz", run: func() error { return expectStatus(client, httpServer.URL+"/readyz", http.StatusOK) }},
		{name: "api.status", run: func() error { return expectStatus(client, httpServer.URL+"/api/v1/status", http.StatusOK) }},
		{name: "api.console", run: func() error { return expectStatus(client, httpServer.URL+"/api/v1/console", http.StatusOK) }},
		{name: "api.diagnostics", run: func() error { return expectStatus(client, httpServer.URL+"/api/v1/diagnostics", http.StatusOK) }},
		{
			name: "api.chat.sessions",
			run: func() error {
				request, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/v1/chat/sessions", nil)
				if err != nil {
					return err
				}
				response, err := client.Do(request)
				if err != nil {
					return err
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusCreated {
					return fmt.Errorf("expected status %d, got %d", http.StatusCreated, response.StatusCode)
				}
				return nil
			},
		},
		{
			name: "api.brain.dispatch",
			run: func() error {
				body := strings.NewReader(`{"prompt":"hello"}`)
				request, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/v1/brain/dispatch", body)
				if err != nil {
					return err
				}
				request.Header.Set("Content-Type", "application/json")
				response, err := client.Do(request)
				if err != nil {
					return err
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					return fmt.Errorf("expected status %d, got %d", http.StatusOK, response.StatusCode)
				}
				return nil
			},
		},
		{
			name: "api.events",
			run: func() error {
				request, err := http.NewRequest(http.MethodGet, httpServer.URL+"/api/v1/events", nil)
				if err != nil {
					return err
				}
				request.Header.Set("Accept", "text/event-stream")
				response, err := client.Do(request)
				if err != nil {
					return err
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					return fmt.Errorf("expected status %d, got %d", http.StatusOK, response.StatusCode)
				}
				if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
					return fmt.Errorf("unexpected content type %q", contentType)
				}
				return nil
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

func smokeStep(name string, run func() error) ([]smokeCheck, bool) {
	started := time.Now()
	if err := run(); err != nil {
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
		Detail:   "ok",
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
