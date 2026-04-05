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
	trace := &PromptTrace{
		Mode:         PromptModeOperator,
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	}
	response, err := s.client.Complete(ctx, CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	})
	if err != nil {
		trace.Error = err.Error()
		s.publishEvent(ctx, engine.EventBrainCommandError, map[string]any{
			"error": err.Error(),
		})
		return DispatchResult{}, s.persona.Humanize(err)
	}
	trace.Model = response.Model
	trace.RawResponse = response.Raw

	command, err := ParseCommand(response.Raw)
	if err != nil {
		s.publishEvent(ctx, engine.EventBrainCommandError, map[string]any{
			"error": err.Error(),
			"raw":   response.Raw,
		})
		return DispatchResult{}, s.persona.Humanize(err)
	}
	if err := command.Validate(skillNames); err != nil {
		s.publishEvent(ctx, engine.EventBrainCommandError, map[string]any{
			"error": err.Error(),
			"raw":   response.Raw,
		})
		return DispatchResult{}, s.persona.Humanize(err)
	}

	s.publishEvent(ctx, engine.EventBrainCommand, map[string]any{
		"action": command.Action,
		"skill":  command.Skill,
		"model":  response.Model,
	})

	// Casual chat — return the conversational response directly.
	if command.Action == "casual_chat" {
		if s.window != nil {
			s.window.Add("user", prompt)
			s.window.Add("assistant", command.Explanation)
		}
		return DispatchResult{
			Command:     command,
			Model:       response.Model,
			Raw:         response.Raw,
			PromptTrace: trace,
		}, nil
	}

	// Chained pipeline — execute steps sequentially.
	if command.Action == "run_chain" {
		return s.executeChain(ctx, prompt, command, response, trace)
	}

	workflow, err := s.coordinator.SubmitWorkflow(ctx, engine.WorkflowDefinition{
		Name:  firstNonEmpty(command.Name, command.Skill),
		Skill: command.Skill,
		Input: command.Input,
	})
	if err != nil {
		var blocked engine.WorkflowBlockedError
		if errors.As(err, &blocked) {
			s.publishEvent(ctx, engine.EventExecutionBlocked, map[string]any{
				"skill":     command.Skill,
				"risk":      blocked.Decision.Risk,
				"reason":    blocked.Decision.Reason,
				"violation": blocked.Decision.Violation,
			})
			result := DispatchResult{
				Command:     command,
				Model:       response.Model,
				Raw:         response.Raw,
				PromptTrace: trace,
				Blocked: &SafetyIntervention{
					Skill:     command.Skill,
					Risk:      blocked.Decision.Risk,
					Reason:    blocked.Decision.Reason,
					Violation: blocked.Decision.Violation,
				},
				Alternative: nil,
			}
			result.Alternative, result.AltTrace = s.formulateSafeAlternative(ctx, prompt, command, blocked, skillDefinitions, skillNames)
			if s.window != nil {
				s.window.Add("user", prompt)
				if result.Alternative != nil {
					s.window.Add("assistant", firstNonEmpty(result.Alternative.Message, "Suggested a safer alternative after a policy block."))
				}
			}
			return result, nil
		}
		s.publishEvent(ctx, engine.EventBrainCommandError, map[string]any{
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
		Command:     command,
		Workflow:    &workflow,
		Model:       response.Model,
		Raw:         response.Raw,
		PromptTrace: trace,
	}, nil
}

func (s *Service) publishEvent(ctx context.Context, eventType engine.EventType, payload map[string]any) {
	if s.bus == nil {
		return
	}
	_ = s.bus.Publish(ctx, engine.Event{
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

func buildAssistantMemoryTurn(command Command) string {
	description := firstNonEmpty(command.Explanation, command.Name, command.Skill)
	return strings.TrimSpace(fmt.Sprintf("Selected %s. %s", command.Skill, description))
}

func (s *Service) formulateSafeAlternative(ctx context.Context, originalPrompt string, blockedCommand Command, blocked engine.WorkflowBlockedError, defs []engine.SkillDefinition, skillNames []string) (*AlternativeSuggestion, *PromptTrace) {
	fallback := &AlternativeSuggestion{
		Message: firstNonEmpty(
			blocked.Decision.Reason,
			"I blocked that workflow because it crossed a security rule. Please narrow the request and try again.",
		),
	}
	if !s.Available() {
		return fallback, nil
	}

	payload, err := json.MarshalIndent(map[string]any{
		"original_prompt": originalPrompt,
		"blocked_command": blockedCommand,
		"decision":        blocked.Decision,
	}, "", "  ")
	if err != nil {
		return fallback, nil
	}
	systemPrompt := s.buildSafeAlternativeSystemPrompt(defs)
	trace := &PromptTrace{
		Mode:         PromptModeSafeAlternative,
		SystemPrompt: systemPrompt,
		UserPrompt:   string(payload),
	}

	response, err := s.client.Complete(ctx, CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   string(payload),
	})
	if err != nil {
		trace.Error = err.Error()
		return fallback, trace
	}
	trace.Model = response.Model
	trace.RawResponse = response.Raw

	suggestion, err := ParseAlternativeSuggestion(response.Raw)
	if err != nil {
		fallback.Raw = response.Raw
		return fallback, trace
	}
	if suggestion.Command != nil {
		if err := suggestion.Command.Validate(skillNames); err != nil {
			suggestion.Command = nil
			if suggestion.Message == "" {
				suggestion.Message = fallback.Message
			}
			return &suggestion, trace
		}
	}
	if suggestion.Message == "" {
		suggestion.Message = fallback.Message
	}
	if suggestion.Raw == "" {
		suggestion.Raw = response.Raw
	}
	return &suggestion, trace
}

func (s *Service) buildSafeAlternativeSystemPrompt(defs []engine.SkillDefinition) string {
	return NewPromptBuilder(PromptModeSafeAlternative).BuildSafeAlternativePrompt(defs)
}

// executeChain runs a sequence of workflow steps. Each step's output is merged
// into the next step's input (explicit input takes precedence over inherited
// values). The chain halts on error or if a step requires approval.
func (s *Service) executeChain(ctx context.Context, prompt string, command Command, response CompletionResponse, trace *PromptTrace) (DispatchResult, error) {
	var lastOutput map[string]any
	var lastWorkflow *engine.Workflow

	for i, step := range command.Steps {
		input := make(map[string]any)
		// Carry forward output from the previous step.
		for k, v := range lastOutput {
			input[k] = v
		}
		// Explicit step input takes precedence.
		for k, v := range step.Input {
			input[k] = v
		}

		wf, err := s.coordinator.SubmitWorkflow(ctx, engine.WorkflowDefinition{
			Name:  fmt.Sprintf("chain-step-%d-%s", i+1, step.Skill),
			Skill: step.Skill,
			Input: input,
		})
		if err != nil {
			var blocked engine.WorkflowBlockedError
			if errors.As(err, &blocked) {
				result := DispatchResult{
					Command:     command,
					Workflow:    lastWorkflow,
					Model:       response.Model,
					Raw:         response.Raw,
					PromptTrace: trace,
					Blocked: &SafetyIntervention{
						Skill:     step.Skill,
						Risk:      blocked.Decision.Risk,
						Reason:    fmt.Sprintf("chain step %d (%s): %s", i+1, step.Skill, blocked.Decision.Reason),
						Violation: blocked.Decision.Violation,
					},
				}
				return result, nil
			}
			return DispatchResult{}, s.persona.Humanize(
				fmt.Errorf("chain step %d (%s): %w", i+1, step.Skill, err),
			)
		}

		lastWorkflow = &wf
		lastOutput = wf.Output

		// If a step requires approval, halt the chain and inform the user.
		if wf.Status == engine.WorkflowWaitingApproval {
			break
		}
	}

	if s.window != nil {
		s.window.Add("user", prompt)
		skills := make([]string, 0, len(command.Steps))
		for _, step := range command.Steps {
			skills = append(skills, step.Skill)
		}
		s.window.Add("assistant", fmt.Sprintf("Executed chain: %s", strings.Join(skills, " → ")))
	}

	return DispatchResult{
		Command:     command,
		Workflow:    lastWorkflow,
		Model:       response.Model,
		Raw:         response.Raw,
		PromptTrace: trace,
	}, nil
}
