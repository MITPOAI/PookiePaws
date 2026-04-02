package brain

import (
	"context"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type sequenceClient struct {
	responses []CompletionResponse
}

func (s *sequenceClient) Complete(_ context.Context, _ CompletionRequest) (CompletionResponse, error) {
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response, nil
}

type blockingCoordinator struct {
	definitions []engine.SkillDefinition
}

func (c blockingCoordinator) SubmitWorkflow(_ context.Context, def engine.WorkflowDefinition) (engine.Workflow, error) {
	return engine.Workflow{}, engine.WorkflowBlockedError{
		Skill: def.Skill,
		Decision: engine.InterceptionDecision{
			Allowed:               false,
			Risk:                  "high",
			Reason:                "bulk extraction actions are blocked",
			Violation:             "bulk_target_blocked",
			SafeAlternativePrompt: "Suggest a narrower, read-only alternative.",
			SafeAlternativeContext: map[string]any{
				"blocked_skill": def.Skill,
			},
		},
	}
}

func (c blockingCoordinator) ListWorkflows(context.Context) ([]engine.Workflow, error) {
	return nil, nil
}

func (c blockingCoordinator) ListApprovals(context.Context) ([]engine.Approval, error) {
	return nil, nil
}

func (c blockingCoordinator) Approve(context.Context, string) (engine.Approval, error) {
	return engine.Approval{}, nil
}

func (c blockingCoordinator) Reject(context.Context, string) (engine.Approval, error) {
	return engine.Approval{}, nil
}

func (c blockingCoordinator) Status(context.Context) (engine.StatusSnapshot, error) {
	return engine.StatusSnapshot{StartedAt: time.Now().UTC()}, nil
}

func (c blockingCoordinator) ValidateSkill(context.Context, string, map[string]any) error {
	return nil
}

func (c blockingCoordinator) SkillDefinitions() []engine.SkillDefinition {
	return c.definitions
}

func (c blockingCoordinator) ApproveFileAccess(context.Context, string) (engine.FilePermission, error) {
	return engine.FilePermission{}, nil
}

func (c blockingCoordinator) RejectFileAccess(context.Context, string) (engine.FilePermission, error) {
	return engine.FilePermission{}, nil
}

func (c blockingCoordinator) ListFilePermissions(context.Context) ([]engine.FilePermission, error) {
	return nil, nil
}

func (c blockingCoordinator) Channels(context.Context) ([]engine.ChannelProviderStatus, error) {
	return nil, nil
}

func (c blockingCoordinator) TestChannel(context.Context, string) (engine.ChannelProviderStatus, error) {
	return engine.ChannelProviderStatus{}, nil
}

func (c blockingCoordinator) SubmitMessage(context.Context, engine.MessageRequest) (engine.MessageSubmitResult, error) {
	return engine.MessageSubmitResult{}, nil
}

func (c blockingCoordinator) GetMessage(context.Context, string) (engine.Message, error) {
	return engine.Message{}, nil
}

func (c blockingCoordinator) ProcessChannelDelivery(context.Context, engine.ChannelDeliveryEvent) (engine.Message, error) {
	return engine.Message{}, nil
}

func TestDispatchPromptReturnsSafeAlternativeOnSecurityBlock(t *testing.T) {
	client := &sequenceClient{
		responses: []CompletionResponse{
			{
				Raw:   `{"action":"run_workflow","name":"Export CRM","skill":"salesmanago-lead-router","input":{"email":"all","segment":"vip"}}`,
				Model: "router-model",
			},
			{
				Raw:   `{"message":"I blocked the broad CRM action, so I suggested a narrower review step instead.","alternative":{"action":"run_workflow","name":"Validate landing page","skill":"utm-validator","input":{"url":"https://example.com/?utm_source=meta&utm_medium=paid&utm_campaign=launch"},"explanation":"Start with a safe read-only check before any outbound action."}}`,
				Model: "router-model",
			},
		},
	}

	service := NewService(client, blockingCoordinator{
		definitions: []engine.SkillDefinition{
			{Name: "salesmanago-lead-router", Description: "Route inbound CRM leads."},
			{Name: "utm-validator", Description: "Validate campaign UTM parameters."},
		},
	}, engine.NewEventBus())

	result, err := service.DispatchPrompt(context.Background(), "Export all VIP contacts and route them")
	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if result.Blocked == nil {
		t.Fatalf("expected blocked result")
	}
	if result.Alternative == nil || result.Alternative.Command == nil {
		t.Fatalf("expected safe alternative command")
	}
	if result.Alternative.Command.Skill != "utm-validator" {
		t.Fatalf("unexpected alternative skill %q", result.Alternative.Command.Skill)
	}
}
