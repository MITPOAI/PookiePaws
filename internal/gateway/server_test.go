package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/adapters"
	"github.com/mitpoai/pookiepaws/internal/brain"
	"github.com/mitpoai/pookiepaws/internal/dossier"
	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/scheduler"
	"github.com/mitpoai/pookiepaws/internal/security"
	"github.com/mitpoai/pookiepaws/internal/skills"
	"github.com/mitpoai/pookiepaws/internal/state"
)

type stubBrain struct{}

func (stubBrain) Available() bool { return true }
func (stubBrain) Status() brain.Status {
	return brain.Status{
		Enabled:  true,
		Provider: "OpenAI-compatible",
		Mode:     "local",
		Model:    "stub-model",
	}
}

func (stubBrain) DispatchPrompt(_ context.Context, _ string) (brain.DispatchResult, error) {
	return brain.DispatchResult{
		Command: brain.Command{
			Action: "run_workflow",
			Skill:  "utm-validator",
		},
	}, nil
}

type harness struct {
	server      *Server
	bus         engine.EventBus
	coord       engine.WorkflowCoordinator
	secrets     *security.JSONSecretProvider
	dossier     *dossier.Service
	runtimeRoot string
}

func newHarness(t *testing.T, address string, promptBrain PromptDispatcher) harness {
	t.Helper()

	root := t.TempDir()
	runtimeRoot := filepath.Join(root, ".pookiepaws")
	workspaceRoot := filepath.Join(runtimeRoot, "workspace")
	bus := engine.NewEventBus()
	subturns := engine.NewSubTurnManager(engine.SubTurnManagerConfig{
		MaxDepth:           4,
		MaxConcurrent:      2,
		ConcurrencyTimeout: time.Second,
		DefaultTimeout:     time.Second,
		Bus:                bus,
	})
	sandbox, err := security.NewWorkspaceSandbox(runtimeRoot, workspaceRoot)
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	secrets, err := security.NewJSONSecretProvider(runtimeRoot)
	if err != nil {
		t.Fatalf("create secrets: %v", err)
	}
	store, err := state.NewFileStore(filepath.Join(runtimeRoot, "state"))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	registry, err := skills.NewDefaultRegistry()
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}
	coord, err := engine.NewWorkflowCoordinator(engine.WorkflowCoordinatorConfig{
		Bus:         bus,
		SubTurns:    subturns,
		Store:       store,
		Skills:      registry,
		Sandbox:     sandbox,
		Secrets:     secrets,
		Interceptor: security.NewSkillExecutionInterceptor(),
		CRMAdapter:  adapters.NewMockSalesmanagoAdapter(),
		SMSAdapter:  adapters.NewMockMittoAdapter(),
		WhatsApp:    adapters.NewMockWhatsAppAdapter(),
		RuntimeRoot: runtimeRoot,
		Workspace:   workspaceRoot,
	})
	if err != nil {
		t.Fatalf("create coordinator: %v", err)
	}
	dossierSvc, err := dossier.NewService(runtimeRoot)
	if err != nil {
		t.Fatalf("create dossier service: %v", err)
	}

	return harness{
		server: NewServer(Config{
			Coordinator: coord,
			EventBus:    bus,
			Brain:       promptBrain,
			Store:       store,
			Vault:       secrets,
			WhatsApp:    adapters.NewMockWhatsAppAdapter(),
			Dossier:     dossierSvc,
			Address:     address,
			AppVersion:  "test-version",
		}),
		bus:         bus,
		coord:       coord,
		secrets:     secrets,
		dossier:     dossierSvc,
		runtimeRoot: runtimeRoot,
	}
}

func TestGatewayWorkflowLifecycle(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/workflows", strings.NewReader(`{"name":"UTM audit","skill":"utm-validator","input":{"url":"https://example.com?utm_source=a&utm_medium=b&utm_campaign=c"}}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	h.server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}

	var workflow engine.Workflow
	if err := json.Unmarshal(recorder.Body.Bytes(), &workflow); err != nil {
		t.Fatalf("decode workflow: %v", err)
	}
	if workflow.Status != engine.WorkflowCompleted {
		t.Fatalf("expected completed workflow, got %q", workflow.Status)
	}
}

func TestGatewayConsoleSnapshot(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", stubBrain{})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/console", nil)
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}

	var snapshot ConsoleSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode console snapshot: %v", err)
	}
	if len(snapshot.Templates) == 0 {
		t.Fatalf("expected templates in console snapshot")
	}
	if !snapshot.Brain.Enabled {
		t.Fatalf("expected brain status to be enabled")
	}
	if !snapshot.Vault.CanWrite {
		t.Fatalf("expected vault writes to be enabled on loopback")
	}
	if snapshot.DemoSmoke != nil {
		t.Fatalf("expected no demo smoke before it has been run")
	}
	if snapshot.LiveResearchSmoke != nil {
		t.Fatalf("expected no live research smoke before it has been run")
	}
}

func TestGatewayConsoleSnapshotIncludesScheduler(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", stubBrain{})

	statePath := scheduler.DefaultStatePath(h.runtimeRoot)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir scheduler state dir: %v", err)
	}
	payload := []byte(`{"schedule":"hourly","last_workflow":"wf-123"}`)
	if err := os.WriteFile(statePath, payload, 0o600); err != nil {
		t.Fatalf("write scheduler state: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/console", nil)
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"scheduler"`) {
		t.Fatalf("expected scheduler field in response body: %s", body)
	}
	if !strings.Contains(body, `"schedule":"hourly"`) {
		t.Fatalf("expected schedule=hourly in response body: %s", body)
	}

	var snapshot ConsoleSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode console snapshot: %v", err)
	}
	if snapshot.Scheduler == nil {
		t.Fatalf("expected scheduler snapshot to be populated")
	}
	if snapshot.Scheduler.Schedule != "hourly" {
		t.Fatalf("expected schedule=hourly, got %q", snapshot.Scheduler.Schedule)
	}
	if snapshot.Scheduler.LastWorkflow != "wf-123" {
		t.Fatalf("expected last_workflow=wf-123, got %q", snapshot.Scheduler.LastWorkflow)
	}
	// With only schedule + last_workflow set, the zero timestamps must be
	// omitted from the JSON payload (encoding/json does not honour
	// `omitempty` for non-pointer time.Time, so SchedulerStatus uses
	// *time.Time fields to make this work).
	for _, field := range []string{"last_tick_at", "last_success_at", "next_due_at"} {
		if strings.Contains(body, `"`+field+`"`) {
			t.Fatalf("expected %q to be omitted from JSON when zero, body=%s", field, body)
		}
	}
	if snapshot.Scheduler.LastTickAt != nil {
		t.Fatalf("expected LastTickAt to be nil, got %v", snapshot.Scheduler.LastTickAt)
	}
	if snapshot.Scheduler.LastSuccessAt != nil {
		t.Fatalf("expected LastSuccessAt to be nil, got %v", snapshot.Scheduler.LastSuccessAt)
	}
	if snapshot.Scheduler.NextDueAt != nil {
		t.Fatalf("expected NextDueAt to be nil, got %v", snapshot.Scheduler.NextDueAt)
	}
}

func TestGatewayConsoleSnapshotSchedulerIncludesTimestamps(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", stubBrain{})

	statePath := scheduler.DefaultStatePath(h.runtimeRoot)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir scheduler state dir: %v", err)
	}
	payload := []byte(`{"schedule":"hourly","last_tick_at":"2026-04-18T10:00:00Z","last_success_at":"2026-04-18T10:00:05Z","next_due_at":"2026-04-18T11:00:00Z","last_workflow":"wf-456"}`)
	if err := os.WriteFile(statePath, payload, 0o600); err != nil {
		t.Fatalf("write scheduler state: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/console", nil)
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}

	var snapshot ConsoleSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode console snapshot: %v", err)
	}
	if snapshot.Scheduler == nil {
		t.Fatalf("expected scheduler snapshot to be populated")
	}
	if snapshot.Scheduler.LastTickAt == nil || snapshot.Scheduler.LastTickAt.IsZero() {
		t.Fatalf("expected LastTickAt to be set, got %v", snapshot.Scheduler.LastTickAt)
	}
	if snapshot.Scheduler.LastSuccessAt == nil || snapshot.Scheduler.LastSuccessAt.IsZero() {
		t.Fatalf("expected LastSuccessAt to be set, got %v", snapshot.Scheduler.LastSuccessAt)
	}
	if snapshot.Scheduler.NextDueAt == nil || snapshot.Scheduler.NextDueAt.IsZero() {
		t.Fatalf("expected NextDueAt to be set, got %v", snapshot.Scheduler.NextDueAt)
	}
}

func TestGatewayDiagnosticsIncludeBrainCheck(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", stubBrain{})
	if err := h.secrets.Update(map[string]string{
		"llm_provider": "openai-compatible",
		"llm_base_url": "http://127.0.0.1:9999/v1/chat/completions",
		"llm_model":    "mock-model",
	}); err != nil {
		t.Fatalf("seed secrets: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics", nil)
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response DiagnosticsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	if response.Brain.Model != "mock-model" {
		t.Fatalf("expected diagnostics brain model, got %+v", response.Brain)
	}
	if response.Brain.ConfigPresent != true {
		t.Fatalf("expected diagnostics brain config to be present, got %+v", response.Brain)
	}
}

func TestGatewayStaticAssets(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", nil)

	for _, route := range []string{"/", "/ui/style.css", "/ui/app.js", "/ui/pookie-paws.webp", "/healthz", "/readyz"} {
		request := httptest.NewRequest(http.MethodGet, route, nil)
		recorder := httptest.NewRecorder()
		h.server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("unexpected status %d for %s", recorder.Code, route)
		}
		if recorder.Body.Len() == 0 {
			t.Fatalf("expected body for %s", route)
		}
	}

	indexRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	indexRecorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(indexRecorder, indexRequest)
	body := indexRecorder.Body.String()
	for _, needle := range []string{"Home", "Run", "Review", "Settings", "Next Best Action", "PookiePaws vtest-version"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected index to contain %q, body=%s", needle, body)
		}
	}
}

func TestGatewaySystemStop(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", nil)
	stopped := make(chan struct{}, 1)
	h.server.requestShutdown = func() {
		select {
		case stopped <- struct{}{}:
		default:
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/stop", nil)
	request.RemoteAddr = "127.0.0.1:40123"
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("expected shutdown callback to be invoked")
	}
}

func TestGatewayVaultUpdate(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", nil)

	request := httptest.NewRequest(http.MethodPut, "/api/v1/settings/vault", strings.NewReader(`{"llm_base_url":"http://localhost:11434/v1/chat/completions","llm_model":"gpt-oss:20b"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}
	if value, err := h.secrets.Get("llm_model"); err != nil || value != "gpt-oss:20b" {
		t.Fatalf("expected llm_model to be persisted, got %q err=%v", value, err)
	}

	var status VaultStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode vault response: %v", err)
	}
	if !status.Brain.Configured {
		t.Fatalf("expected brain to be marked configured after update")
	}
}

func TestGatewayVaultRejectsNonLoopbackWrites(t *testing.T) {
	h := newHarness(t, "0.0.0.0:18800", nil)

	request := httptest.NewRequest(http.MethodPut, "/api/v1/settings/vault", strings.NewReader(`{"llm_model":"gpt-oss:20b"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGatewayWorkflowPlanRequiresApprovalForSend(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/plan", strings.NewReader(`{"nodes":[{"id":"a","type":"draft_sms","position":{"x":0,"y":0}},{"id":"b","type":"send","position":{"x":10,"y":10}}]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGatewayEventsStreamSummaries(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", nil)
	httpServer := httptest.NewServer(h.server.Handler())
	defer httpServer.Close()

	request, err := http.NewRequest(http.MethodGet, httpServer.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("connect events stream: %v", err)
	}
	defer response.Body.Close()

	time.Sleep(20 * time.Millisecond)
	if err := h.bus.Publish(context.Background(), engine.Event{
		Type:       engine.EventApprovalRequired,
		WorkflowID: "wf_1",
		Source:     "workflow-coordinator",
		Payload: map[string]any{
			"approval_id": "ap_1",
			"adapter":     "mitto",
			"action":      "send_sms",
		},
	}); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	reader := bufio.NewReader(response.Body)
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			if readErr == io.EOF {
				continue
			}
			t.Fatalf("read event stream: %v", readErr)
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		var entry AuditEntryView
		if err := json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(line), "data: ")), &entry); err != nil {
			t.Fatalf("decode event summary: %v", err)
		}
		if entry.Title != "Approval required" {
			t.Fatalf("unexpected event title %q", entry.Title)
		}
		if entry.ApprovalID != "ap_1" {
			t.Fatalf("unexpected approval id %q", entry.ApprovalID)
		}
		return
	}
}

func TestGatewayBrainDispatch(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", stubBrain{})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/brain/dispatch", strings.NewReader(`{"prompt":"validate this url"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	h.server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGatewayWorkflowBlockReturnsForbidden(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/workflows", strings.NewReader(`{"name":"Unsafe CRM export","skill":"salesmanago-lead-router","input":{"email":"all","segment":"vip"}}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	h.server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGatewayChannelsStatus(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}

	var channels []engine.ChannelProviderStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &channels); err != nil {
		t.Fatalf("decode channels: %v", err)
	}
	if len(channels) == 0 || channels[0].Channel != "whatsapp" {
		t.Fatalf("expected whatsapp channel status, got %+v", channels)
	}
}

func TestGatewayMessagesLifecycle(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"channel":"whatsapp","provider":"meta_cloud","to":"+61400000000","type":"text","text":"Hello from Pookie"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}

	var result engine.MessageSubmitResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode message submit result: %v", err)
	}
	if result.Message.Status != engine.MessagePendingApproval {
		t.Fatalf("expected message pending approval, got %q", result.Message.Status)
	}

	messageRequest := httptest.NewRequest(http.MethodGet, "/api/v1/messages/"+result.Message.ID, nil)
	messageRecorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(messageRecorder, messageRequest)
	if messageRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected get status %d body=%s", messageRecorder.Code, messageRecorder.Body.String())
	}

	var message engine.Message
	if err := json.Unmarshal(messageRecorder.Body.Bytes(), &message); err != nil {
		t.Fatalf("decode message: %v", err)
	}
	if message.Channel != "whatsapp" {
		t.Fatalf("expected whatsapp message, got %+v", message)
	}
}

func TestGatewayWhatsAppConnectionTest(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/channels/whatsapp/test", nil)
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGatewayDemoSmokeLifecycle(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", nil)

	runRequest := httptest.NewRequest(http.MethodPost, "/api/v1/demo/smoke", nil)
	runRecorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(runRecorder, runRequest)
	if runRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected run status %d body=%s", runRecorder.Code, runRecorder.Body.String())
	}

	var result struct {
		Passed       bool   `json:"passed"`
		ArtifactPath string `json:"artifact_path"`
	}
	if err := json.Unmarshal(runRecorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode demo smoke: %v", err)
	}
	if !result.Passed || result.ArtifactPath == "" {
		t.Fatalf("expected successful demo smoke, got %+v", result)
	}

	consoleRequest := httptest.NewRequest(http.MethodGet, "/api/v1/console", nil)
	consoleRecorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(consoleRecorder, consoleRequest)
	if consoleRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected console status %d body=%s", consoleRecorder.Code, consoleRecorder.Body.String())
	}

	var snapshot ConsoleSnapshot
	if err := json.Unmarshal(consoleRecorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode console snapshot: %v", err)
	}
	if snapshot.DemoSmoke == nil || snapshot.DemoSmoke.ArtifactPath == "" {
		t.Fatalf("expected demo smoke in console snapshot, got %+v", snapshot.DemoSmoke)
	}
	if snapshot.LiveResearchSmoke != nil {
		t.Fatalf("expected live research smoke to remain empty, got %+v", snapshot.LiveResearchSmoke)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/demo/smoke", nil)
	getRecorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(getRecorder, getRequest)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected get status %d body=%s", getRecorder.Code, getRecorder.Body.String())
	}
}

func TestGatewayLiveResearchSmokeLifecycle(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", nil)
	if err := h.secrets.Update(map[string]string{
		"firecrawl_api_key": "fc-test",
	}); err != nil {
		t.Fatalf("seed firecrawl secret: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/search":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"web": []map[string]any{
						{
							"title":       "OpenClaw Pricing",
							"description": "Operator pricing",
							"url":         "https://openclaw.example/pricing",
							"markdown":    "# Pricing\nPremium operator plan.",
						},
					},
				},
			})
		default:
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write([]byte("# Mock page"))
		}
	}))
	defer server.Close()

	t.Setenv("POOKIEPAWS_FIRECRAWL_BASE_URL", server.URL)
	t.Setenv("POOKIEPAWS_JINA_BASE_URL", server.URL)

	runRequest := httptest.NewRequest(http.MethodPost, "/api/v1/demo/smoke?mode=live", nil)
	runRecorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(runRecorder, runRequest)
	if runRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected run status %d body=%s", runRecorder.Code, runRecorder.Body.String())
	}

	var result struct {
		Passed       bool   `json:"passed"`
		Mode         string `json:"mode"`
		ArtifactPath string `json:"artifact_path"`
		SourceCount  int    `json:"source_count"`
	}
	if err := json.Unmarshal(runRecorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode live demo smoke: %v", err)
	}
	if !result.Passed || result.ArtifactPath == "" || result.Mode != "live" || result.SourceCount == 0 {
		t.Fatalf("expected successful live research smoke, got %+v", result)
	}

	consoleRequest := httptest.NewRequest(http.MethodGet, "/api/v1/console", nil)
	consoleRecorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(consoleRecorder, consoleRequest)
	if consoleRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected console status %d body=%s", consoleRecorder.Code, consoleRecorder.Body.String())
	}

	var snapshot ConsoleSnapshot
	if err := json.Unmarshal(consoleRecorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode console snapshot: %v", err)
	}
	if snapshot.LiveResearchSmoke == nil || snapshot.LiveResearchSmoke.ArtifactPath == "" {
		t.Fatalf("expected live research smoke in console snapshot, got %+v", snapshot.LiveResearchSmoke)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/demo/smoke?mode=live", nil)
	getRecorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(getRecorder, getRequest)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected get status %d body=%s", getRecorder.Code, getRecorder.Body.String())
	}
}

func TestGatewayResearchControlPlaneLifecycle(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", nil)
	if err := h.secrets.Update(map[string]string{
		"research_watchlists": `[{"name":"OpenClaw core watchlist","topic":"OpenClaw","company":"PookiePaws","competitors":["OpenClaw"],"domains":["openclaw.example"],"market":"AU pet gifting","focus_areas":["pricing","positioning","offers"]}]`,
		"research_schedule":   "manual",
		"autonomy_policy":     "trusted_ops_v1",
		"action_policy":       "approval_gated",
	}); err != nil {
		t.Fatalf("seed research settings: %v", err)
	}

	// Mirror the daemon startup flow: import the legacy `research_watchlists`
	// vault key into state-backed storage. The refresh endpoint no longer
	// reads the vault key directly — it consults state via the dossier
	// service, so this migration step is required for the test to exercise
	// the production code path.
	if _, err := dossier.MigrateLegacyWatchlists(context.Background(), h.dossier, h.secrets); err != nil {
		t.Fatalf("migrate legacy watchlists: %v", err)
	}

	refreshRequest := httptest.NewRequest(http.MethodPost, "/api/v1/research/watchlists/refresh", nil)
	refreshRecorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(refreshRecorder, refreshRequest)
	if refreshRecorder.Code != http.StatusCreated {
		t.Fatalf("unexpected refresh status %d body=%s", refreshRecorder.Code, refreshRecorder.Body.String())
	}

	consoleRequest := httptest.NewRequest(http.MethodGet, "/api/v1/console", nil)
	consoleRecorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(consoleRecorder, consoleRequest)
	if consoleRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected console status %d body=%s", consoleRecorder.Code, consoleRecorder.Body.String())
	}

	var snapshot ConsoleSnapshot
	if err := json.Unmarshal(consoleRecorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode console snapshot: %v", err)
	}
	if len(snapshot.Watchlists) == 0 || len(snapshot.Dossiers) == 0 || len(snapshot.Recommendations) == 0 {
		t.Fatalf("expected research state in console snapshot, got %+v", snapshot)
	}

	recommendationsRequest := httptest.NewRequest(http.MethodGet, "/api/v1/research/recommendations", nil)
	recommendationsRecorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recommendationsRecorder, recommendationsRequest)
	if recommendationsRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected recommendations status %d body=%s", recommendationsRecorder.Code, recommendationsRecorder.Body.String())
	}

	var recommendations []dossier.Recommendation
	if err := json.Unmarshal(recommendationsRecorder.Body.Bytes(), &recommendations); err != nil {
		t.Fatalf("decode recommendations: %v", err)
	}
	if len(recommendations) == 0 {
		t.Fatal("expected at least one recommendation")
	}

	editRequest := httptest.NewRequest(http.MethodPut, "/api/v1/research/recommendations/"+recommendations[0].ID+"/edit", strings.NewReader(`{"title":"Operator review export"}`))
	editRequest.Header.Set("Content-Type", "application/json")
	editRecorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(editRecorder, editRequest)
	if editRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected edit status %d body=%s", editRecorder.Code, editRecorder.Body.String())
	}

	queueRequest := httptest.NewRequest(http.MethodPost, "/api/v1/research/recommendations/"+recommendations[0].ID+"/queue", nil)
	queueRecorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(queueRecorder, queueRequest)
	if queueRecorder.Code != http.StatusCreated {
		t.Fatalf("unexpected queue status %d body=%s", queueRecorder.Code, queueRecorder.Body.String())
	}

	discardRequest := httptest.NewRequest(http.MethodPost, "/api/v1/research/recommendations/"+recommendations[1].ID+"/discard", nil)
	discardRecorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(discardRecorder, discardRequest)
	if discardRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected discard status %d body=%s", discardRecorder.Code, discardRecorder.Body.String())
	}
}

func TestVaultPUTRejectsResearchWatchlists(t *testing.T) {
	h := newHarness(t, "127.0.0.1:0", nil)
	body := `{"research_watchlists":"[{\"id\":\"wl-1\",\"name\":\"x\"}]"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/vault", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "research_watchlists") {
		t.Errorf("expected error to mention the deprecated key: %s", resp.Body.String())
	}
}

func TestVaultPUTAllowsEmptyResearchWatchlists(t *testing.T) {
	// Empty value remains accepted so stale form posts succeed.
	h := newHarness(t, "127.0.0.1:0", nil)
	body := `{"research_watchlists":"","research_schedule":"hourly"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/vault", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
}

func TestVaultPUTStillValidatesSchedule(t *testing.T) {
	h := newHarness(t, "127.0.0.1:0", nil)
	body := `{"research_schedule":"weekly"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/vault", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", resp.Code, resp.Body.String())
	}
}

func TestMaxBytesMiddlewareRejectsOversizedBody(t *testing.T) {
	const limit = 64
	called := false
	wrapped := maxBytesMiddleware(limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, err := io.ReadAll(r.Body)
		if err == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		// http.MaxBytesReader returns *http.MaxBytesError once the cap is
		// exceeded; surface it as a 413 so callers see the limit fire.
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, "too big", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
	}))

	oversized := strings.Repeat("a", limit*4)
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(oversized))
	resp := httptest.NewRecorder()
	wrapped.ServeHTTP(resp, req)

	if !called {
		t.Fatalf("inner handler was not invoked")
	}
	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", resp.Code, resp.Body.String())
	}
}

func TestGatewayHandlerAppliesBodyCap(t *testing.T) {
	root := t.TempDir()
	bus := engine.NewEventBus()
	store, err := state.NewFileStore(filepath.Join(root, "state"))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	dossierSvc, err := dossier.NewService(root)
	if err != nil {
		t.Fatalf("create dossier service: %v", err)
	}
	srv := NewServer(Config{
		EventBus:     bus,
		Store:        store,
		Dossier:      dossierSvc,
		WhatsApp:     adapters.NewMockWhatsAppAdapter(),
		Address:      "127.0.0.1:0",
		MaxBodyBytes: 32, // tiny cap so any real JSON payload trips it
	})

	// /api/v1/workflows POST handler reads JSON; an oversized body must
	// not produce a 2xx success.
	body := strings.Repeat("x", 1024)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code >= 200 && resp.Code < 300 {
		t.Fatalf("oversized body got success status %d; expected rejection", resp.Code)
	}
}

func TestGatewayMaxBodyBytesDefaults(t *testing.T) {
	root := t.TempDir()
	bus := engine.NewEventBus()
	store, err := state.NewFileStore(filepath.Join(root, "state"))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	dossierSvc, err := dossier.NewService(root)
	if err != nil {
		t.Fatalf("create dossier service: %v", err)
	}

	// Zero ⇒ default 1 MiB.
	def := NewServer(Config{
		EventBus: bus, Store: store, Dossier: dossierSvc,
		WhatsApp: adapters.NewMockWhatsAppAdapter(), Address: "127.0.0.1:0",
	})
	if def.maxBodyBytes != DefaultMaxBodyBytes {
		t.Fatalf("default maxBodyBytes = %d, want %d", def.maxBodyBytes, DefaultMaxBodyBytes)
	}

	// Negative ⇒ disabled.
	off := NewServer(Config{
		EventBus: bus, Store: store, Dossier: dossierSvc,
		WhatsApp: adapters.NewMockWhatsAppAdapter(), Address: "127.0.0.1:0",
		MaxBodyBytes: -1,
	})
	if off.maxBodyBytes != 0 {
		t.Fatalf("disabled maxBodyBytes = %d, want 0", off.maxBodyBytes)
	}
}
