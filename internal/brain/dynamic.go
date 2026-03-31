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
		window:          NewConversationWindow(8),
	}
	service.bindFlushListener()
	return service
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
