package brain

import (
	"context"
	"sort"
)

// Tool represents a capability the LLM can invoke during a ReAct reasoning
// loop. Tools are lightweight functions (fetch data, write files, run commands)
// that the orchestrator executes on behalf of the model.
type Tool interface {
	// Name returns the tool identifier the LLM uses to request this tool.
	Name() string
	// Description explains what the tool does (included in the system prompt).
	Description() string
	// ParameterSchema describes the expected input fields for the LLM.
	ParameterSchema() string
	// Definition returns the JSON Schema definition used in native tool-calling API requests.
	Definition() ToolDefinition
	// Execute runs the tool with the given input and returns the result.
	Execute(ctx context.Context, input map[string]any) (map[string]any, error)
}

// ApprovalFunc is called before executing tools that require human confirmation
// (e.g. os_command). Returns true if the user approves, false to deny.
type ApprovalFunc func(toolName string, description string) bool

// ToolRegistry holds tools available for the ReAct orchestrator.
type ToolRegistry struct {
	tools map[string]Tool
}

// NewToolRegistry creates an empty tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]Tool)}
}

// Register adds or replaces a tool in the registry.
func (r *ToolRegistry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get returns a tool by name and whether it was found.
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools sorted by name.
func (r *ToolRegistry) List() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name() < out[j].Name()
	})
	return out
}

// BuildDefinitions returns ToolDefinition slices for all registered tools.
// Uses a type-assertion guard so tools that don't yet implement Definition()
// are silently skipped rather than causing a panic.
func (r *ToolRegistry) BuildDefinitions() []ToolDefinition {
	type definable interface {
		Definition() ToolDefinition
	}
	tools := r.List()
	defs := make([]ToolDefinition, 0, len(tools))
	for _, t := range tools {
		if d, ok := t.(definable); ok {
			defs = append(defs, d.Definition())
		}
	}
	return defs
}
