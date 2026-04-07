package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

// NativeOrchestrate runs the ReAct loop using the LLM's native tool-calling API.
// It manages a proper []ChatMessage conversation, routes tool_calls through
// SecurityValidator, and executes tools. Falls back to Orchestrate when the
// configured client doesn't implement NativeClient.
func (s *Service) NativeOrchestrate(ctx context.Context, prompt string, cfg OrchestrateConfig) (OrchestrateResult, error) {
	if !s.Available() {
		return OrchestrateResult{}, s.persona.Humanize(ErrProviderNotConfigured)
	}

	nativeClient, ok := s.client.(NativeClient)
	if !ok {
		return s.Orchestrate(ctx, prompt, cfg)
	}

	if cfg.Tools == nil || len(cfg.Tools.tools) == 0 {
		result, err := s.DispatchPrompt(ctx, prompt)
		return OrchestrateResult{DispatchResult: result}, err
	}

	skillDefinitions := s.coordinator.SkillDefinitions()
	memorySnapshot, _ := s.snapshotMemory(ctx)
	recentTurns := s.snapshotTurns()

	systemPromptText := buildNativeSystemPrompt(s, skillDefinitions, memorySnapshot, recentTurns, cfg)
	messages := []ChatMessage{
		{Role: "system", Content: systemPromptText},
		{Role: "user", Content: buildUserPrompt(prompt, recentTurns)},
	}
	toolDefs := cfg.Tools.BuildDefinitions()

	var iterations []ReActIteration

	for i := 0; i < MaxReActIterations; i++ {
		if err := ctx.Err(); err != nil {
			return OrchestrateResult{}, fmt.Errorf("orchestrator cancelled: %w", err)
		}

		resp, err := nativeClient.CompleteNative(ctx, messages, toolDefs)
		if err != nil {
			return OrchestrateResult{}, s.persona.Humanize(err)
		}

		messages = append(messages, resp.Message)

		if resp.FinishReason != "tool_calls" || len(resp.Message.ToolCalls) == 0 {
			return s.nativeTerminalResult(ctx, resp, iterations)
		}

		for _, tc := range resp.Message.ToolCalls {
			toolName := tc.Function.Name
			argsJSON := tc.Function.Arguments

			iter := ReActIteration{Index: i, Tool: toolName}
			json.Unmarshal([]byte(argsJSON), &iter.ToolInput) //nolint:errcheck

			if cfg.Validator != nil {
				blocked, vErr := cfg.Validator.Validate(toolName, argsJSON)
				if vErr != nil {
					resultJSON, _ := json.Marshal(map[string]any{"error": vErr.Error()})
					messages = append(messages, ChatMessage{Role: "tool", ToolCallID: tc.ID, Content: string(resultJSON)})
					iter.ToolError = vErr.Error()
					iterations = append(iterations, iter)
					continue
				}
				if blocked != nil {
					resultJSON, _ := json.Marshal(blocked)
					messages = append(messages, ChatMessage{Role: "tool", ToolCallID: tc.ID, Content: string(resultJSON)})
					iter.ToolOutput = blocked
					iterations = append(iterations, iter)
					continue
				}
			}

			tool, ok := cfg.Tools.Get(toolName)
			if !ok {
				resultJSON, _ := json.Marshal(map[string]any{"error": fmt.Sprintf("unknown tool %q", toolName)})
				msg := string(resultJSON)
				messages = append(messages, ChatMessage{Role: "tool", ToolCallID: tc.ID, Content: msg})
				iterations = append(iterations, iter)
				continue
			}

			var toolInput map[string]any
			if err := json.Unmarshal([]byte(argsJSON), &toolInput); err != nil {
				resultJSON, _ := json.Marshal(map[string]any{"error": "invalid arguments: " + err.Error()})
				msg := string(resultJSON)
				messages = append(messages, ChatMessage{Role: "tool", ToolCallID: tc.ID, Content: msg})
				iterations = append(iterations, iter)
				continue
			}

			// For os_command: SecurityValidator already prompted the user.
			// Use a shallow copy with a pass-through Approve to avoid a second prompt.
			effectiveTool := tool
			if toolName == "os_command" && cfg.Validator != nil {
				if osTool, ok := tool.(*OSCommandTool); ok {
					copy := *osTool
					copy.Approve = func(string, string) bool { return true }
					effectiveTool = &copy
				}
			}

			if cfg.OnToolStart != nil {
				cfg.OnToolStart(toolName, toolInput)
			}

			toolResult, toolErr := effectiveTool.Execute(ctx, toolInput)
			if toolErr != nil {
				iter.ToolError = toolErr.Error()
				resultJSON, _ := json.Marshal(map[string]any{"error": toolErr.Error()})
				messages = append(messages, ChatMessage{Role: "tool", ToolCallID: tc.ID, Content: string(resultJSON)})
			} else {
				iter.ToolOutput = toolResult
				resultJSON, _ := json.Marshal(toolResult)
				resultStr := string(resultJSON)
				if len(resultStr) > 4000 {
					resultStr = resultStr[:4000] + "...(truncated)"
				}
				messages = append(messages, ChatMessage{Role: "tool", ToolCallID: tc.ID, Content: resultStr})
			}

			if cfg.OnToolDone != nil {
				cfg.OnToolDone(toolName, toolResult, toolErr)
			}
			iterations = append(iterations, iter)
		}
	}

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

func (s *Service) nativeTerminalResult(ctx context.Context, resp NativeCompletionResponse, iterations []ReActIteration) (OrchestrateResult, error) {
	content := strings.TrimSpace(resp.Message.Content)
	if content == "" {
		content = "Research complete."
	}

	command, err := ParseCommand(content)
	if err != nil || command.Action == "" {
		command = Command{Action: "casual_chat", Explanation: content}
	}

	s.publishEvent(ctx, engine.EventBrainCommand, map[string]any{
		"action": command.Action,
		"model":  resp.Model,
	})

	return OrchestrateResult{
		DispatchResult: DispatchResult{
			Command: command,
			Model:   resp.Model,
			Raw:     content,
		},
		Iterations: iterations,
	}, nil
}

func buildNativeSystemPrompt(s *Service, defs []engine.SkillDefinition, mem MemorySnapshot, turns []ConversationTurn, cfg OrchestrateConfig) string {
	base := s.persona.RoutingPrompt(defs, mem, turns, cfg.Tools.List()...)
	return SystemPromptBase + "\n\n" + base
}
