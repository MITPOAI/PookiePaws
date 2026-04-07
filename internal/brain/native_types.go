package brain

import "context"

// SystemPromptBase is the core persona and rule set for Pookie, injected as the
// first system message in every NativeOrchestrate conversation.
const SystemPromptBase = `You are Pookie, an elite marketing operations agent for MITPO powered by PookiePaws. You orchestrate marketing research, content creation, and campaign execution with precision.

CORE RULES:
1. NEVER guess URLs or domain names. Always use the web_search tool to look up URLs first.
2. NEVER hallucinate statistics, pricing data, or facts. Research with web_search instead.
3. If an OS command fails or is denied, explain clearly why and suggest an alternative.
4. If operator instructions are ambiguous, ask for clarification via casual_chat before executing.
5. Only operate within marketing and system operations. Decline unrelated requests politely.
6. All file operations must stay within the workspace. Never access files outside ~/.pookiepaws/workspace/.
7. High-risk actions (OS commands, bulk sends) require explicit human approval. Never bypass approval gates.`

// ChatMessage is a single turn in a native tool-calling conversation.
// Role is one of "system", "user", "assistant", "tool".
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // set when role="tool"
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // set when role="assistant" + tool calls
}

// ToolCall is a single function invocation requested by the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // always "function"
	Function ToolCallFunc `json:"function"`
}

// ToolCallFunc holds the function name and its JSON-encoded arguments string.
type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // raw JSON: {"url":"..."}
}

// NativeCompletionResponse is the parsed result of one CompleteNative round-trip.
type NativeCompletionResponse struct {
	Message      ChatMessage
	FinishReason string // "stop" | "tool_calls" | "length"
	Model        string
}

// ToolDefinition is the OpenAI-compatible JSON Schema function definition.
type ToolDefinition struct {
	Type     string      `json:"type"` // always "function"
	Function FunctionDef `json:"function"`
}

// FunctionDef describes a callable function for the model.
type FunctionDef struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  JSONSchema `json:"parameters"`
}

// JSONSchema is a simplified JSON Schema object for function parameters.
type JSONSchema struct {
	Type       string                    `json:"type"` // always "object"
	Properties map[string]SchemaProperty `json:"properties"`
	Required   []string                  `json:"required,omitempty"`
}

// SchemaProperty describes one parameter field.
type SchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// NativeClient extends CompletionClient with OpenAI-native tool-calling support.
// OpenAICompatibleClient implements this; MCP providers do not.
type NativeClient interface {
	CompleteNative(ctx context.Context, messages []ChatMessage, tools []ToolDefinition) (NativeCompletionResponse, error)
}
