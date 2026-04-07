package brain

import (
	"context"
	"fmt"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type DynamicService struct {
	secrets         engine.SecretProvider
	coordinator     engine.WorkflowCoordinator
	bus             engine.EventBus
	providerFactory ProviderFactory
	persona         Persona
	memory          MemoryReader
	window          *ConversationWindow
}

func NewDynamicService(secrets engine.SecretProvider, coordinator engine.WorkflowCoordinator, bus engine.EventBus) *DynamicService {
	service := &DynamicService{
		secrets:         secrets,
		coordinator:     coordinator,
		bus:             bus,
		providerFactory: NewSecretBackedProviderFactory(secrets),
		persona:         DefaultPookiePersona(),
		window:          NewConversationWindow(24),
	}
	service.bindFlushListener()
	return service
}

func (s *DynamicService) WithWindowPath(path string) *DynamicService {
	if s == nil {
		return nil
	}
	s.window.SetPath(path)
	return s
}

func (s *DynamicService) WithMemory(memory MemoryReader) *DynamicService {
	if s == nil {
		return nil
	}
	s.memory = memory
	return s
}

func (s *DynamicService) WithPersona(persona Persona) *DynamicService {
	if s == nil {
		return nil
	}
	s.persona = persona
	return s
}

func (s *DynamicService) Available() bool {
	if s == nil {
		return false
	}
	return s.providerFactory != nil && s.providerFactory.Available()
}

func (s *DynamicService) Status() Status {
	if s == nil {
		return Status{Enabled: false, Provider: "OpenAI-compatible", Mode: "disabled"}
	}
	if s.providerFactory == nil {
		return Status{Enabled: false, Provider: "OpenAI-compatible", Mode: "disabled"}
	}
	return s.providerFactory.Status()
}

// DebugRoutingPrompt renders the full system prompt that would be sent to the
// LLM on the next dispatch. Useful for CLI inspection (pookie context --prompt).
func (s *DynamicService) DebugRoutingPrompt() string {
	if s == nil {
		return "(brain not configured)"
	}
	defs := []engine.SkillDefinition{}
	if s.coordinator != nil {
		defs = s.coordinator.SkillDefinitions()
	}
	var memory MemorySnapshot
	if s.memory != nil {
		memory, _ = s.memory.Snapshot(context.Background())
	}
	var turns []ConversationTurn
	if s.window != nil {
		turns = s.window.Snapshot()
	}
	return s.persona.RoutingPrompt(defs, memory, turns)
}

func (s *DynamicService) DispatchPrompt(ctx context.Context, prompt string) (DispatchResult, error) {
	if s == nil {
		return DispatchResult{}, fmt.Errorf("llm brain is not configured")
	}
	if s.providerFactory == nil {
		return DispatchResult{}, s.persona.Humanize(ErrProviderNotConfigured)
	}
	client, err := s.providerFactory.New(ctx)
	if err != nil {
		return DispatchResult{}, s.persona.Humanize(err)
	}
	defer client.Close()

	return NewService(client, s.coordinator, s.bus).
		WithPersona(s.persona).
		WithMemory(s.memory).
		WithWindow(s.window).
		DispatchPrompt(ctx, prompt)
}

// Orchestrate runs the ReAct orchestrator loop with tool-calling support.
func (s *DynamicService) Orchestrate(ctx context.Context, prompt string, cfg OrchestrateConfig) (OrchestrateResult, error) {
	if s == nil {
		return OrchestrateResult{}, fmt.Errorf("llm brain is not configured")
	}
	if s.providerFactory == nil {
		return OrchestrateResult{}, s.persona.Humanize(ErrProviderNotConfigured)
	}
	client, err := s.providerFactory.New(ctx)
	if err != nil {
		return OrchestrateResult{}, s.persona.Humanize(err)
	}
	defer client.Close()

	return NewService(client, s.coordinator, s.bus).
		WithPersona(s.persona).
		WithMemory(s.memory).
		WithWindow(s.window).
		Orchestrate(ctx, prompt, cfg)
}

// NativeOrchestrate runs the native tool-calling loop with tool-calling support.
// Falls back to text-JSON Orchestrate when the provider doesn't implement NativeClient.
func (s *DynamicService) NativeOrchestrate(ctx context.Context, prompt string, cfg OrchestrateConfig) (OrchestrateResult, error) {
	if s == nil {
		return OrchestrateResult{}, fmt.Errorf("llm brain is not configured")
	}
	if s.providerFactory == nil {
		return OrchestrateResult{}, s.persona.Humanize(ErrProviderNotConfigured)
	}
	client, err := s.providerFactory.New(ctx)
	if err != nil {
		return OrchestrateResult{}, s.persona.Humanize(err)
	}
	defer client.Close()

	return NewService(client, s.coordinator, s.bus).
		WithPersona(s.persona).
		WithMemory(s.memory).
		WithWindow(s.window).
		NativeOrchestrate(ctx, prompt, cfg)
}

func (s *DynamicService) bindFlushListener() {
	if s == nil || s.bus == nil || s.window == nil {
		return
	}

	subscription := s.bus.Subscribe(8)
	go func() {
		for event := range subscription.C {
			if event.Type == engine.EventContextFlush {
				s.window.Reset()
			}
		}
	}()
}
