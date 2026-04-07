package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

// MaxReActIterations caps the number of tool calls in a single orchestration
// run to prevent infinite loops and runaway token usage.
const MaxReActIterations = 10

// OrchestrateConfig configures a ReAct orchestrator run.
type OrchestrateConfig struct {
	// Tools is the registry of tools the LLM can invoke. If nil, the
	// orchestrator falls back to single-shot dispatch.
	Tools *ToolRegistry
	// ApprovalFn is called before executing tools that require human
	// confirmation (e.g. os_command). If nil, approval-gated tools are denied.
	ApprovalFn ApprovalFunc
	// OnToolStart is called when a tool execution begins (e.g. to update a
	// spinner label). May be nil.
	OnToolStart func(toolName string, input map[string]any)
	// OnToolDone is called when a tool execution completes. May be nil.
	OnToolDone func(toolName string, result map[string]any, err error)
	// Validator intercepts native tool call arguments before execution.
	// Used by NativeOrchestrate to enforce path confinement and HITL approval.
	// If nil, no pre-execution validation is performed.
	Validator *SecurityValidator
}

// OrchestrateResult extends DispatchResult with the tool-call iteration trace.
type OrchestrateResult struct {
	DispatchResult
	Iterations []ReActIteration `json:"iterations,omitempty"`
}

// ReActIteration records a single tool call within the orchestrator loop.
type ReActIteration struct {
	Index      int            `json:"index"`
	Tool       string         `json:"tool"`
	ToolInput  map[string]any `json:"tool_input,omitempty"`
	ToolOutput map[string]any `json:"tool_output,omitempty"`
	ToolError  string         `json:"tool_error,omitempty"`
}

// Orchestrate runs a ReAct (Reasoning + Acting) loop. The LLM can request
// tool calls via the "use_tool" action; the orchestrator executes the tool,
// appends the result to the conversation, and calls the LLM again. The loop
// terminates when the LLM returns a non-tool action (casual_chat, run_workflow,
// run_chain) or when MaxReActIterations is reached.
func (s *Service) Orchestrate(ctx context.Context, prompt string, cfg OrchestrateConfig) (OrchestrateResult, error) {
	if !s.Available() {
		return OrchestrateResult{}, s.persona.Humanize(ErrProviderNotConfigured)
	}

	// No tools: fall back to single-shot dispatch.
	if cfg.Tools == nil || len(cfg.Tools.tools) == 0 {
		result, err := s.DispatchPrompt(ctx, prompt)
		return OrchestrateResult{DispatchResult: result}, err
	}

	skillDefinitions := s.coordinator.SkillDefinitions()
	skillNames := make([]string, 0, len(skillDefinitions))
	for _, def := range skillDefinitions {
		skillNames = append(skillNames, def.Name)
	}

	memorySnapshot, _ := s.snapshotMemory(ctx)
	recentTurns := s.snapshotTurns()
	systemPrompt := s.persona.RoutingPrompt(skillDefinitions, memorySnapshot, recentTurns, cfg.Tools.List()...)

	userPrompt := buildUserPrompt(prompt, recentTurns)

	var iterations []ReActIteration
	var conversationHistory strings.Builder
	conversationHistory.WriteString(userPrompt)

	for i := 0; i < MaxReActIterations; i++ {
		// Check for context cancellation before each LLM call.
		if err := ctx.Err(); err != nil {
			return OrchestrateResult{}, fmt.Errorf("orchestrator cancelled: %w", err)
		}

		currentUserPrompt := conversationHistory.String()
		trace := &PromptTrace{
			Mode:         PromptModeOperator,
			SystemPrompt: systemPrompt,
			UserPrompt:   currentUserPrompt,
		}

		response, err := s.client.Complete(ctx, CompletionRequest{
			SystemPrompt: systemPrompt,
			UserPrompt:   currentUserPrompt,
		})
		if err != nil {
			trace.Error = err.Error()
			return OrchestrateResult{}, s.persona.Humanize(err)
		}
		trace.Model = response.Model
		trace.RawResponse = response.Raw

		command, err := ParseCommand(response.Raw)
		if err != nil {
			// Non-JSON fallback: treat as casual chat.
			rawText := strings.TrimSpace(response.Raw)
			if rawText != "" {
				command = Command{Action: "casual_chat", Explanation: rawText}
			} else {
				return OrchestrateResult{}, s.persona.Humanize(err)
			}
		}

		// Terminal action: anything that is NOT use_tool.
		if command.Action != "use_tool" {
			if err := command.Validate(skillNames); err != nil {
				s.publishEvent(ctx, engine.EventBrainCommandError, map[string]any{
					"error": err.Error(),
					"raw":   response.Raw,
				})
				return OrchestrateResult{}, s.persona.Humanize(err)
			}
			s.publishEvent(ctx, engine.EventBrainCommand, map[string]any{
				"action": command.Action,
				"skill":  command.Skill,
				"model":  response.Model,
			})
			result, routeErr := s.routeCommand(ctx, prompt, command, response, trace, skillDefinitions, skillNames)
			return OrchestrateResult{
				DispatchResult: result,
				Iterations:     iterations,
			}, routeErr
		}

		// ── use_tool: execute the tool and loop ────────────────────────
		toolName := strings.TrimSpace(command.Tool)
		tool, ok := cfg.Tools.Get(toolName)
		if !ok {
			names := listToolNames(cfg.Tools)
			conversationHistory.WriteString(fmt.Sprintf(
				"\n\n[TOOL-OUTPUT] Tool %q not found. Available tools: %s. Choose a different action or tool.",
				toolName, names,
			))
			continue
		}

		if cfg.OnToolStart != nil {
			cfg.OnToolStart(toolName, command.ToolInput)
		}

		iteration := ReActIteration{
			Index:     i,
			Tool:      toolName,
			ToolInput: command.ToolInput,
		}

		toolResult, toolErr := tool.Execute(ctx, command.ToolInput)
		if toolErr != nil {
			iteration.ToolError = toolErr.Error()
			conversationHistory.WriteString(fmt.Sprintf(
				"\n\n[TOOL-OUTPUT] Tool %q failed: %s. You may try a different approach or return a final action.",
				toolName, toolErr.Error(),
			))
		} else {
			iteration.ToolOutput = toolResult
			resultJSON, _ := json.Marshal(toolResult)
			// Truncate large tool outputs to prevent token overflow.
			resultStr := string(resultJSON)
			if len(resultStr) > 4000 {
				resultStr = resultStr[:4000] + "...(truncated)"
			}
			conversationHistory.WriteString(fmt.Sprintf(
				"\n\n[TOOL-OUTPUT] Tool %q result:\n%s\n\nNow decide your next action. You may call another tool or return a final action (casual_chat, run_workflow, or run_chain).",
				toolName, resultStr,
			))
		}

		if cfg.OnToolDone != nil {
			cfg.OnToolDone(toolName, toolResult, toolErr)
		}

		iterations = append(iterations, iteration)
	}

	// Max iterations reached: return a fallback.
	return OrchestrateResult{
		DispatchResult: DispatchResult{
			Command: Command{
				Action:      "casual_chat",
				Explanation: "I reached the maximum number of reasoning steps. Here is what I gathered so far during my research.",
			},
		},
		Iterations: iterations,
	}, nil
}

// listToolNames returns a comma-separated list of registered tool names.
func listToolNames(registry *ToolRegistry) string {
	tools := registry.List()
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name()
	}
	return strings.Join(names, ", ")
}
