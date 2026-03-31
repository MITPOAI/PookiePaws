package brain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type Service struct {
	client      CompletionClient
	coordinator engine.WorkflowCoordinator
	bus         engine.EventBus
	persona     Persona
	memory      MemoryReader
	window      *ConversationWindow
	status      Status
}

func NewService(client CompletionClient, coordinator engine.WorkflowCoordinator, bus engine.EventBus) *Service {
	service := &Service{
		client:      client,
		coordinator: coordinator,
		bus:         bus,
		persona:     DefaultPookiePersona(),
	}
	if provider, ok := client.(interface{ Status() Status }); ok {
		service.status = provider.Status()
	} else {
		service.status = Status{Enabled: client != nil, Provider: "unknown", Mode: "unknown"}
	}
	return service
}

func (s *Service) WithPersona(persona Persona) *Service {
	if s == nil {
		return nil
	}
	s.persona = persona
	return s
}

func (s *Service) WithMemory(memory MemoryReader) *Service {
	if s == nil {
		return nil
	}
	s.memory = memory
	return s
}

func (s *Service) WithWindow(window *ConversationWindow) *Service {
	if s == nil {
		return nil
	}
	s.window = window
	return s
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
		return DispatchResult{}, s.persona.Humanize(ErrProviderNotConfigured)
	}

	skillDefinitions := s.coordinator.SkillDefinitions()
	skillNames := make([]string, 0, len(skillDefinitions))
	for _, def := range skillDefinitions {
		skillNames = append(skillNames, def.Name)
	}

	memorySnapshot, _ := s.snapshotMemory(ctx)
	recentTurns := s.snapshotTurns()

	systemPrompt := s.persona.RoutingPrompt(skillDefinitions, memorySnapshot, recentTurns)
	userPrompt := buildUserPrompt(prompt, recentTurns)
	response, err := s.client.Complete(ctx, CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	})
	if err != nil {
		s.publishEvent(engine.EventBrainCommandError, map[string]any{
			"error": err.Error(),
		})
		return DispatchResult{}, s.persona.Humanize(err)
	}

	command, err := ParseCommand(response.Raw)
	if err != nil {
		s.publishEvent(engine.EventBrainCommandError, map[string]any{
			"error": err.Error(),
			"raw":   response.Raw,
		})
		return DispatchResult{}, s.persona.Humanize(err)
	}
	if err := command.Validate(skillNames); err != nil {
		s.publishEvent(engine.EventBrainCommandError, map[string]any{
			"error": err.Error(),
			"raw":   response.Raw,
		})
		return DispatchResult{}, s.persona.Humanize(err)
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
		var blocked engine.WorkflowBlockedError
		if errors.As(err, &blocked) {
			s.publishEvent(engine.EventExecutionBlocked, map[string]any{
				"skill":     command.Skill,
				"risk":      blocked.Decision.Risk,
				"reason":    blocked.Decision.Reason,
				"violation": blocked.Decision.Violation,
			})
			result := DispatchResult{
				Command: command,
				Model:   response.Model,
				Raw:     response.Raw,
				Blocked: &SafetyIntervention{
					Skill:     command.Skill,
					Risk:      blocked.Decision.Risk,
					Reason:    blocked.Decision.Reason,
					Violation: blocked.Decision.Violation,
				},
				Alternative: s.formulateSafeAlternative(ctx, prompt, command, blocked, skillDefinitions, skillNames),
			}
			if s.window != nil {
				s.window.Add("user", prompt)
				if result.Alternative != nil {
					s.window.Add("assistant", firstNonEmpty(result.Alternative.Message, "Suggested a safer alternative after a policy block."))
				}
			}
			return result, nil
		}
		s.publishEvent(engine.EventBrainCommandError, map[string]any{
			"error": err.Error(),
			"skill": command.Skill,
			"raw":   response.Raw,
		})
		return DispatchResult{}, s.persona.Humanize(err)
	}

	if s.window != nil {
		s.window.Add("user", prompt)
		s.window.Add("assistant", buildAssistantMemoryTurn(command))
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
		Type:    eventType,
		Source:  "brain",
		Payload: payload,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Service) snapshotMemory(ctx context.Context) (MemorySnapshot, error) {
	if s == nil || s.memory == nil {
		return MemorySnapshot{}, nil
	}
	return s.memory.Snapshot(ctx)
}

func (s *Service) snapshotTurns() []ConversationTurn {
	if s == nil || s.window == nil {
		return nil
	}
	return s.window.Snapshot()
}

func buildUserPrompt(prompt string, turns []ConversationTurn) string {
	prompt = strings.TrimSpace(prompt)
	if len(turns) == 0 {
		return prompt
	}

	var builder strings.Builder
	builder.WriteString("Recent routing context:\n")
	for _, turn := range turns {
		builder.WriteString("- ")
		builder.WriteString(turn.Role)
		builder.WriteString(": ")
		builder.WriteString(strings.TrimSpace(turn.Content))
		builder.WriteString("\n")
	}
	builder.WriteString("\nCurrent request:\n")
	builder.WriteString(prompt)
	return builder.String()
}

func buildAssistantMemoryTurn(command Command) string {
	description := firstNonEmpty(command.Explanation, command.Name, command.Skill)
	return strings.TrimSpace(fmt.Sprintf("Selected %s. %s", command.Skill, description))
}

func (s *Service) formulateSafeAlternative(ctx context.Context, originalPrompt string, blockedCommand Command, blocked engine.WorkflowBlockedError, defs []engine.SkillDefinition, skillNames []string) *AlternativeSuggestion {
	fallback := &AlternativeSuggestion{
		Message: firstNonEmpty(
			blocked.Decision.Reason,
			"I blocked that workflow because it crossed a security rule. Please narrow the request and try again.",
		),
	}
	if !s.Available() {
		return fallback
	}

	payload, err := json.MarshalIndent(map[string]any{
		"original_prompt": originalPrompt,
		"blocked_command": blockedCommand,
		"decision":        blocked.Decision,
	}, "", "  ")
	if err != nil {
		return fallback
	}

	response, err := s.client.Complete(ctx, CompletionRequest{
		SystemPrompt: s.buildSafeAlternativeSystemPrompt(defs),
		UserPrompt:   string(payload),
	})
	if err != nil {
		return fallback
	}

	suggestion, err := ParseAlternativeSuggestion(response.Raw)
	if err != nil {
		fallback.Raw = response.Raw
		return fallback
	}
	if suggestion.Command != nil {
		if err := suggestion.Command.Validate(skillNames); err != nil {
			suggestion.Command = nil
			if suggestion.Message == "" {
				suggestion.Message = fallback.Message
			}
			return &suggestion
		}
	}
	if suggestion.Message == "" {
		suggestion.Message = fallback.Message
	}
	if suggestion.Raw == "" {
		suggestion.Raw = response.Raw
	}
	return &suggestion
}

func (s *Service) buildSafeAlternativeSystemPrompt(defs []engine.SkillDefinition) string {
	var builder strings.Builder
	builder.WriteString("You are Pookie, the PookiePaws security fallback planner. ")
	builder.WriteString("A requested workflow was blocked by the police layer. ")
	builder.WriteString(`Return exactly one JSON object and no surrounding prose using this schema: {"message":"short calm explanation for a marketer","alternative":{"action":"run_workflow","name":"short title","skill":"one-skill-name","input":{...},"explanation":"short reason"}}. `)
	builder.WriteString("If no safe workflow exists, set alternative to null and explain why in message. ")
	builder.WriteString("Keep all alternatives read-only or approval-gated, never destructive, never credential-seeking, and never shell-executing.\n\n")
	builder.WriteString("Available safe skills:\n")
	for _, def := range defs {
		builder.WriteString("- ")
		builder.WriteString(def.Name)
		if def.Description != "" {
			builder.WriteString(": ")
			builder.WriteString(def.Description)
		}
		builder.WriteString("\n")
	}
	return builder.String()
}
