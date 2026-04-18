package brain

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/security"
)

// ── stubs ──────────────────────────────────────────────────────────────────

// stubNativeClient implements both CompletionClient and NativeClient.
type stubNativeClient struct {
	responses []NativeCompletionResponse
	callIdx   int
}

func (s *stubNativeClient) Complete(_ context.Context, _ CompletionRequest) (CompletionResponse, error) {
	return CompletionResponse{Raw: `{"action":"casual_chat","explanation":"hello"}`}, nil
}

func (s *stubNativeClient) CompleteNative(_ context.Context, _ []ChatMessage, _ []ToolDefinition) (NativeCompletionResponse, error) {
	if s.callIdx >= len(s.responses) {
		return NativeCompletionResponse{
			Message:      ChatMessage{Role: "assistant", Content: "done"},
			FinishReason: "stop",
		}, nil
	}
	r := s.responses[s.callIdx]
	s.callIdx++
	return r, nil
}

// stubToolForOrchestrator is a minimal Tool implementation used in NativeOrchestrate tests.
type stubToolForOrchestrator struct {
	name     string
	result   map[string]any
	executed bool
}

func (s *stubToolForOrchestrator) Name() string        { return s.name }
func (s *stubToolForOrchestrator) Description() string { return "stub" }
func (s *stubToolForOrchestrator) ParameterSchema() string {
	return "{}"
}
func (s *stubToolForOrchestrator) Definition() ToolDefinition {
	return ToolDefinition{Type: "function", Function: FunctionDef{
		Name:       s.name,
		Parameters: JSONSchema{Type: "object", Properties: map[string]SchemaProperty{}},
	}}
}
func (s *stubToolForOrchestrator) Execute(_ context.Context, _ map[string]any) (map[string]any, error) {
	s.executed = true
	if s.result != nil {
		return s.result, nil
	}
	return map[string]any{"ok": true}, nil
}

// noopCoordinator satisfies engine.WorkflowCoordinator with no-op behaviour.
type noopCoordinator struct{}

func (noopCoordinator) SubmitWorkflow(_ context.Context, _ engine.WorkflowDefinition) (engine.Workflow, error) {
	return engine.Workflow{}, nil
}
func (noopCoordinator) ListWorkflows(context.Context) ([]engine.Workflow, error) { return nil, nil }
func (noopCoordinator) ListWorkflowsByStatus(context.Context, ...engine.WorkflowStatus) ([]engine.Workflow, error) {
	return nil, nil
}
func (noopCoordinator) ListApprovals(context.Context) ([]engine.Approval, error) { return nil, nil }
func (noopCoordinator) Approve(context.Context, string) (engine.Approval, error) {
	return engine.Approval{}, nil
}
func (noopCoordinator) Reject(context.Context, string) (engine.Approval, error) {
	return engine.Approval{}, nil
}
func (noopCoordinator) Status(context.Context) (engine.StatusSnapshot, error) {
	return engine.StatusSnapshot{StartedAt: time.Now().UTC()}, nil
}
func (noopCoordinator) ValidateSkill(context.Context, string, map[string]any) error { return nil }
func (noopCoordinator) SkillDefinitions() []engine.SkillDefinition                  { return nil }
func (noopCoordinator) ApproveFileAccess(context.Context, string) (engine.FilePermission, error) {
	return engine.FilePermission{}, nil
}
func (noopCoordinator) RejectFileAccess(context.Context, string) (engine.FilePermission, error) {
	return engine.FilePermission{}, nil
}
func (noopCoordinator) ListFilePermissions(context.Context) ([]engine.FilePermission, error) {
	return nil, nil
}
func (noopCoordinator) Channels(context.Context) ([]engine.ChannelProviderStatus, error) {
	return nil, nil
}
func (noopCoordinator) TestChannel(context.Context, string) (engine.ChannelProviderStatus, error) {
	return engine.ChannelProviderStatus{}, nil
}
func (noopCoordinator) SubmitMessage(context.Context, engine.MessageRequest) (engine.MessageSubmitResult, error) {
	return engine.MessageSubmitResult{}, nil
}
func (noopCoordinator) GetMessage(context.Context, string) (engine.Message, error) {
	return engine.Message{}, nil
}
func (noopCoordinator) ProcessChannelDelivery(context.Context, engine.ChannelDeliveryEvent) (engine.Message, error) {
	return engine.Message{}, nil
}

// newStubServiceForOrchestrator builds a Service using stubNativeClient and noopCoordinator.
func newStubServiceForOrchestrator(client CompletionClient) *Service {
	return NewService(client, noopCoordinator{}, engine.NewEventBus())
}

// newToolRegistryWithStub creates a ToolRegistry containing a single stub tool.
func newToolRegistryWithStub(tool *stubToolForOrchestrator) *ToolRegistry {
	r := NewToolRegistry()
	r.Register(tool)
	return r
}

// ── tests ──────────────────────────────────────────────────────────────────

// TestNativeOrchestrateStopOnFirstCall: client returns finish_reason "stop"
// immediately → Action is "casual_chat", zero iterations.
func TestNativeOrchestrateStopOnFirstCall(t *testing.T) {
	client := &stubNativeClient{
		responses: []NativeCompletionResponse{
			{
				Message:      ChatMessage{Role: "assistant", Content: "All done!"},
				FinishReason: "stop",
				Model:        "test-model",
			},
		},
	}

	svc := newStubServiceForOrchestrator(client)
	tool := &stubToolForOrchestrator{name: "web_search"}
	cfg := OrchestrateConfig{Tools: newToolRegistryWithStub(tool)}

	result, err := svc.NativeOrchestrate(context.Background(), "hello", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Command.Action != "casual_chat" {
		t.Errorf("expected casual_chat, got %q", result.Command.Action)
	}
	if len(result.Iterations) != 0 {
		t.Errorf("expected 0 iterations, got %d", len(result.Iterations))
	}
	if tool.executed {
		t.Error("tool should not have been executed")
	}
}

// TestNativeOrchestrateToolCallThenStop: first response has tool_calls for
// "web_search", second has finish_reason "stop" → 1 iteration recorded.
func TestNativeOrchestrateToolCallThenStop(t *testing.T) {
	argsJSON := `{"query":"golang testing"}`
	client := &stubNativeClient{
		responses: []NativeCompletionResponse{
			{
				Message: ChatMessage{
					Role:    "assistant",
					Content: "",
					ToolCalls: []ToolCall{
						{
							ID:   "call-1",
							Type: "function",
							Function: ToolCallFunc{
								Name:      "web_search",
								Arguments: argsJSON,
							},
						},
					},
				},
				FinishReason: "tool_calls",
				Model:        "test-model",
			},
			{
				Message:      ChatMessage{Role: "assistant", Content: "Search done."},
				FinishReason: "stop",
				Model:        "test-model",
			},
		},
	}

	tool := &stubToolForOrchestrator{name: "web_search", result: map[string]any{"results": "some results"}}
	svc := newStubServiceForOrchestrator(client)
	cfg := OrchestrateConfig{Tools: newToolRegistryWithStub(tool)}

	result, err := svc.NativeOrchestrate(context.Background(), "search golang", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Command.Action != "casual_chat" {
		t.Errorf("expected casual_chat, got %q", result.Command.Action)
	}
	if len(result.Iterations) != 1 {
		t.Errorf("expected 1 iteration, got %d", len(result.Iterations))
	}
	if !tool.executed {
		t.Error("tool should have been executed")
	}
	iter := result.Iterations[0]
	if iter.Tool != "web_search" {
		t.Errorf("expected tool web_search, got %q", iter.Tool)
	}
	if iter.ToolOutput == nil {
		t.Error("expected non-nil ToolOutput")
	}

	// Verify ToolInput was captured.
	var input map[string]any
	json.Unmarshal([]byte(argsJSON), &input)
	if iter.ToolInput["query"] != input["query"] {
		t.Errorf("expected ToolInput.query=%q, got %v", input["query"], iter.ToolInput["query"])
	}
}

// TestNativeOrchestrateValidatorBlocks: tool call for "read_local_file" with
// "../escape.txt" → validator blocks it, tool.Execute never called, 1
// iteration with ToolOutput containing "error" key.
func TestNativeOrchestrateValidatorBlocks(t *testing.T) {
	argsJSON := `{"path":"../escape.txt"}`
	client := &stubNativeClient{
		responses: []NativeCompletionResponse{
			{
				Message: ChatMessage{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{
							ID:   "call-1",
							Type: "function",
							Function: ToolCallFunc{
								Name:      "read_local_file",
								Arguments: argsJSON,
							},
						},
					},
				},
				FinishReason: "tool_calls",
				Model:        "test-model",
			},
			{
				Message:      ChatMessage{Role: "assistant", Content: "I cannot read that file."},
				FinishReason: "stop",
				Model:        "test-model",
			},
		},
	}

	tool := &stubToolForOrchestrator{name: "read_local_file"}
	svc := newStubServiceForOrchestrator(client)

	// Set up a sandbox that will block path traversal.
	root := t.TempDir()
	sandbox, err := security.NewWorkspaceSandbox(
		filepath.Join(root, ".pookiepaws"),
		filepath.Join(root, ".pookiepaws", "workspace"),
	)
	if err != nil {
		t.Fatalf("sandbox: %v", err)
	}

	cfg := OrchestrateConfig{
		Tools:     newToolRegistryWithStub(tool),
		Validator: NewSecurityValidator(sandbox, nil),
	}

	result, err := svc.NativeOrchestrate(context.Background(), "read file", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.executed {
		t.Error("tool should NOT have been executed when validator blocks")
	}
	if len(result.Iterations) != 1 {
		t.Fatalf("expected 1 iteration, got %d", len(result.Iterations))
	}
	iter := result.Iterations[0]
	if iter.ToolOutput == nil {
		t.Fatal("expected ToolOutput to be set by validator block")
	}
	if _, ok := iter.ToolOutput["error"]; !ok {
		t.Errorf("expected 'error' key in ToolOutput, got %v", iter.ToolOutput)
	}
}

// TestNativeOrchestrateMaxIterations: always returns tool_calls → fallback
// casual_chat with MaxReActIterations iterations.
func TestNativeOrchestrateMaxIterations(t *testing.T) {
	// The client always returns a tool_call response; it will exhaust the pool
	// and then the stubNativeClient default kicks in ("stop"), but we need it
	// to always return tool_calls. Build enough responses.
	responses := make([]NativeCompletionResponse, MaxReActIterations+1)
	for i := range responses {
		responses[i] = NativeCompletionResponse{
			Message: ChatMessage{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{
						ID:   "call-loop",
						Type: "function",
						Function: ToolCallFunc{
							Name:      "web_search",
							Arguments: `{"query":"loop"}`,
						},
					},
				},
			},
			FinishReason: "tool_calls",
			Model:        "test-model",
		}
	}

	client := &stubNativeClient{responses: responses}
	tool := &stubToolForOrchestrator{name: "web_search"}
	svc := newStubServiceForOrchestrator(client)
	cfg := OrchestrateConfig{Tools: newToolRegistryWithStub(tool)}

	result, err := svc.NativeOrchestrate(context.Background(), "loop forever", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Command.Action != "casual_chat" {
		t.Errorf("expected casual_chat fallback, got %q", result.Command.Action)
	}
	if len(result.Iterations) != MaxReActIterations {
		t.Errorf("expected %d iterations, got %d", MaxReActIterations, len(result.Iterations))
	}
}

// TestNativeOrchestrateFallbackForNonNativeClient: pass a stubClient (non-native)
// → falls back to text-JSON Orchestrate, result is still casual_chat.
func TestNativeOrchestrateFallbackForNonNativeClient(t *testing.T) {
	// stubClient only implements CompletionClient, NOT NativeClient.
	client := stubClient{
		response: CompletionResponse{
			Raw:   `{"action":"casual_chat","explanation":"hello from text path"}`,
			Model: "text-model",
		},
	}

	svc := newStubServiceForOrchestrator(client)
	tool := &stubToolForOrchestrator{name: "web_search"}
	cfg := OrchestrateConfig{Tools: newToolRegistryWithStub(tool)}

	result, err := svc.NativeOrchestrate(context.Background(), "hello", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Command.Action != "casual_chat" {
		t.Errorf("expected casual_chat, got %q", result.Command.Action)
	}
	// tool should not be executed since the text-JSON path handles it differently.
	if tool.executed {
		t.Error("tool should not have been executed on the fallback path")
	}
}
