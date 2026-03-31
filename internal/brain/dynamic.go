package brain

import (
	"context"
	"fmt"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type DynamicService struct {
	secrets     engine.SecretProvider
	coordinator engine.WorkflowCoordinator
	bus         engine.EventBus
}

func NewDynamicService(secrets engine.SecretProvider, coordinator engine.WorkflowCoordinator, bus engine.EventBus) *DynamicService {
	return &DynamicService{
		secrets:     secrets,
		coordinator: coordinator,
		bus:         bus,
	}
}

func (s *DynamicService) Available() bool {
	if s == nil {
		return false
	}
	_, err := NewOpenAICompatibleClient(s.secrets)
	return err == nil
}

func (s *DynamicService) Status() Status {
	if s == nil {
		return Status{Enabled: false, Provider: "OpenAI-compatible", Mode: "disabled"}
	}
	client, err := NewOpenAICompatibleClient(s.secrets)
	if err != nil {
		return Status{Enabled: false, Provider: "OpenAI-compatible", Mode: "disabled"}
	}
	return client.Status()
}

func (s *DynamicService) DispatchPrompt(ctx context.Context, prompt string) (DispatchResult, error) {
	if s == nil {
		return DispatchResult{}, fmt.Errorf("llm brain is not configured")
	}
	client, err := NewOpenAICompatibleClient(s.secrets)
	if err != nil {
		return DispatchResult{}, fmt.Errorf("llm brain is not configured")
	}
	return NewService(client, s.coordinator, s.bus).DispatchPrompt(ctx, prompt)
}
