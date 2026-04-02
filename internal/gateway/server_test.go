package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/adapters"
	"github.com/mitpoai/pookiepaws/internal/brain"
	"github.com/mitpoai/pookiepaws/internal/engine"
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
	server  *Server
	bus     engine.EventBus
	coord   engine.WorkflowCoordinator
	secrets *security.JSONSecretProvider
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

	return harness{
		server: NewServer(Config{
			Coordinator: coord,
			EventBus:    bus,
			Brain:       promptBrain,
			Vault:       secrets,
			WhatsApp:    adapters.NewMockWhatsAppAdapter(),
			Address:     address,
		}),
		bus:     bus,
		coord:   coord,
		secrets: secrets,
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
}

func TestGatewayStaticAssets(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", nil)

	for _, route := range []string{"/", "/ui/style.css", "/ui/app.js", "/healthz", "/readyz"} {
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
	if err := h.bus.Publish(engine.Event{
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
