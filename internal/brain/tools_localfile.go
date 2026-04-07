package brain

import (
	"context"
	"fmt"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

// ReadLocalFileTool reads a file from the workspace and returns its content.
// The path must be relative to the workspace root; the sandbox enforces containment.
type ReadLocalFileTool struct {
	Sandbox engine.Sandbox
}

var _ Tool = (*ReadLocalFileTool)(nil)

func (t *ReadLocalFileTool) Name() string { return "read_local_file" }
func (t *ReadLocalFileTool) Description() string {
	return "Read a local context file from the workspace (e.g. brand-guidelines.txt). Path must be relative to the workspace root."
}
func (t *ReadLocalFileTool) ParameterSchema() string {
	return `{"path": "string (required) - relative path within workspace, e.g. brand-guidelines.txt"}`
}

func (t *ReadLocalFileTool) Definition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: JSONSchema{
				Type: "object",
				Properties: map[string]SchemaProperty{
					"path": {Type: "string", Description: "Relative path within workspace, e.g. exports/report.md"},
				},
				Required: []string{"path"},
			},
		},
	}
}

func (t *ReadLocalFileTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	path := strings.TrimSpace(asString(input["path"]))
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	data, err := t.Sandbox.ReadFile(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	content := string(data)
	if len(content) > 8000 {
		content = content[:8000] + "\n...(truncated)"
	}

	return map[string]any{
		"path":    path,
		"content": content,
		"bytes":   len(data),
	}, nil
}
