package brain

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/adapters"
	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/security"
	"github.com/mitpoai/pookiepaws/internal/skills"
	"github.com/mitpoai/pookiepaws/internal/state"
)

type stubClient struct {
	response CompletionResponse
	err      error
}

func (s stubClient) Complete(context.Context, CompletionRequest) (CompletionResponse, error) {
	return s.response, s.err
}

func TestDispatchPromptCreatesWorkflow(t *testing.T) {
	root := t.TempDir()
	bus := engine.NewEventBus()
	subturns := engine.NewSubTurnManager(engine.SubTurnManagerConfig{
		MaxDepth:           4,
		MaxConcurrent:      2,
		ConcurrencyTimeout: time.Second,
		DefaultTimeout:     time.Second,
		Bus:                bus,
	})
	sandbox, _ := security.NewWorkspaceSandbox(filepath.Join(root, ".pookiepaws"), filepath.Join(root, ".pookiepaws", "workspace"))
	secrets, _ := security.NewJSONSecretProvider(filepath.Join(root, ".pookiepaws"))
	store, _ := state.NewFileStore(filepath.Join(root, ".pookiepaws", "state"))
	registry, _ := skills.NewDefaultRegistry()

	coord, err := engine.NewWorkflowCoordinator(engine.WorkflowCoordinatorConfig{
		Bus:         bus,
		SubTurns:    subturns,
		Store:       store,
		Skills:      registry,
		Sandbox:     sandbox,
		Secrets:     secrets,
		CRMAdapter:  adapters.NewMockSalesmanagoAdapter(),
		SMSAdapter:  adapters.NewMockMittoAdapter(),
		RuntimeRoot: filepath.Join(root, ".pookiepaws"),
		Workspace:   filepath.Join(root, ".pookiepaws", "workspace"),
	})
	if err != nil {
		t.Fatalf("create coordinator: %v", err)
	}

	service := NewService(stubClient{
		response: CompletionResponse{
			Raw:   `{"action":"run_workflow","name":"Validate campaign UTM","skill":"utm-validator","input":{"url":"https://example.com?utm_source=a&utm_medium=b&utm_campaign=c"}}`,
			Model: "test-model",
		},
	}, coord, bus)

	result, err := service.DispatchPrompt(context.Background(), "validate this campaign link")
	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if result.Workflow == nil {
		t.Fatalf("expected workflow to be created")
	}
	if result.Workflow.Skill != "utm-validator" {
		t.Fatalf("unexpected skill %q", result.Workflow.Skill)
	}
}

func TestParseCommandStripsMarkdownFence(t *testing.T) {
	command, err := ParseCommand("```json\n{\"action\":\"run_workflow\",\"skill\":\"utm-validator\",\"input\":{\"url\":\"https://example.com\"}}\n```")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if command.Skill != "utm-validator" {
		t.Fatalf("unexpected skill %q", command.Skill)
	}
}
