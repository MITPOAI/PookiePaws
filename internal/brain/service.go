package brain

import (
	"context"
	"fmt"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type Service struct {
	client      CompletionClient
	coordinator engine.WorkflowCoordinator
	bus         engine.EventBus
	status      Status
}

func NewService(client CompletionClient, coordinator engine.WorkflowCoordinator, bus engine.EventBus) *Service {
	service := &Service{
		client:      client,
		coordinator: coordinator,
		bus:         bus,
	}
	if provider, ok := client.(interface{ Status() Status }); ok {
		service.status = provider.Status()
	} else {
		service.status = Status{Enabled: client != nil, Provider: "unknown", Mode: "unknown"}
	}
	return service
}

func (s *Service) Available() bool {
	return s != nil && s.client != nil
}

func (s *Service) Status() Status {
	if s == nil {
		return Status{Enabled: false, Provider: "OpenAI-compatible", Mode: "disabled"}
	}
	if !s.Available() {
		status := s.status
		status.Enabled = false
		if strings.TrimSpace(status.Provider) == "" {
			status.Provider = "OpenAI-compatible"
		}
		if strings.TrimSpace(status.Mode) == "" {
			status.Mode = "disabled"
		}
		return status
	}
	return s.status
}

func (s *Service) DispatchPrompt(ctx context.Context, prompt string) (DispatchResult, error) {
	if !s.Available() {
		return DispatchResult{}, fmt.Errorf("llm brain is not configured")
	}

	skillDefinitions := s.coordinator.SkillDefinitions()
	skillNames := make([]string, 0, len(skillDefinitions))
	for _, def := range skillDefinitions {
		skillNames = append(skillNames, def.Name)
	}

	systemPrompt := buildSystemPrompt(skillDefinitions)
	response, err := s.client.Complete(ctx, CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   prompt,
	})
	if err != nil {
		s.publishEvent(engine.EventBrainCommandError, map[string]any{
			"error": err.Error(),
		})
		return DispatchResult{}, err
	}

	command, err := ParseCommand(response.Raw)
	if err != nil {
		s.publishEvent(engine.EventBrainCommandError, map[string]any{
			"error": err.Error(),
			"raw":   response.Raw,
		})
		return DispatchResult{}, err
	}
	if err := command.Validate(skillNames); err != nil {
		s.publishEvent(engine.EventBrainCommandError, map[string]any{
			"error": err.Error(),
			"raw":   response.Raw,
		})
		return DispatchResult{}, err
	}

	s.publishEvent(engine.EventBrainCommand, map[string]any{
		"action": command.Action,
		"skill":  command.Skill,
		"model":  response.Model,
	})

	workflow, err := s.coordinator.SubmitWorkflow(ctx, engine.WorkflowDefinition{
		Name:  firstNonEmpty(command.Name, command.Skill),
		Skill: command.Skill,
		Input: command.Input,
	})
	if err != nil {
		s.publishEvent(engine.EventBrainCommandError, map[string]any{
			"error": err.Error(),
			"skill": command.Skill,
			"raw":   response.Raw,
		})
		return DispatchResult{}, err
	}

	return DispatchResult{
		Command:  command,
		Workflow: &workflow,
		Model:    response.Model,
		Raw:      response.Raw,
	}, nil
}

func (s *Service) publishEvent(eventType engine.EventType, payload map[string]any) {
	if s.bus == nil {
		return
	}
	_ = s.bus.Publish(engine.Event{
		Type:   eventType,
		Source: "brain",
		Payload: payload,
	})
}

func buildSystemPrompt(defs []engine.SkillDefinition) string {
	var builder strings.Builder
	builder.WriteString("You are the PookiePaws workflow router. ")
	builder.WriteString("Convert the user request into exactly one JSON object and no surrounding prose. ")
	builder.WriteString(`Use this schema: {"action":"run_workflow","name":"short human title","skill":"one-skill-name","input":{...},"explanation":"short reason"}. `)
	builder.WriteString("Only use one of these skills:\n")
	for _, def := range defs {
		builder.WriteString("- ")
		builder.WriteString(def.Name)
		if def.Description != "" {
			builder.WriteString(": ")
			builder.WriteString(def.Description)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("If the request is missing required fields, still output valid JSON using the best matching skill and include only the fields you can infer.")
	return builder.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
