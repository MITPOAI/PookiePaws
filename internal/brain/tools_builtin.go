package brain

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

// ── ExportMarkdownTool ─────────────────────────────────────────────────────

// ExportMarkdownTool saves text content as a timestamped markdown file.
type ExportMarkdownTool struct {
	Sandbox engine.Sandbox
}

var _ Tool = (*ExportMarkdownTool)(nil)

func (t *ExportMarkdownTool) Name() string        { return "export_markdown" }
func (t *ExportMarkdownTool) Description() string {
	return "Save text content as a Markdown file in the workspace exports folder. Returns the file path."
}
func (t *ExportMarkdownTool) ParameterSchema() string {
	return `{"content": "string (required) - the markdown content to save", "title": "string (optional) - document title", "filename": "string (optional) - filename prefix, default export"}`
}

func (t *ExportMarkdownTool) Definition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: JSONSchema{
				Type: "object",
				Properties: map[string]SchemaProperty{
					"content":  {Type: "string", Description: "The Markdown content to save (required)"},
					"title":    {Type: "string", Description: "Optional document title"},
					"filename": {Type: "string", Description: "Optional filename prefix; default: export"},
				},
				Required: []string{"content"},
			},
		},
	}
}

func (t *ExportMarkdownTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	content := strings.TrimSpace(asString(input["content"]))
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	title := strings.TrimSpace(asString(input["title"]))
	prefix := strings.TrimSpace(asString(input["filename"]))
	if prefix == "" {
		prefix = "export"
	}

	var doc strings.Builder
	if title != "" {
		doc.WriteString("# ")
		doc.WriteString(title)
		doc.WriteString("\n\n")
	}
	doc.WriteString(content)
	doc.WriteString("\n")
	data := []byte(doc.String())

	stamp := time.Now().UTC().Format("2006-01-02T15-04-05")
	filename := fmt.Sprintf("%s-%s.md", prefix, stamp)

	exportsDir, err := t.Sandbox.ResolveWithinWorkspace("exports")
	if err != nil {
		return nil, fmt.Errorf("resolve exports dir: %w", err)
	}
	fullPath := filepath.Join(exportsDir, filename)

	if err := t.Sandbox.WriteFile(ctx, fullPath, data); err != nil {
		return nil, fmt.Errorf("write export: %w", err)
	}

	return map[string]any{
		"path": fullPath,
		"size": len(data),
	}, nil
}

// ── OSCommandTool ──────────────────────────────────────────────────────────

// OSCommandTool runs allowlisted shell commands with human-in-the-loop approval.
type OSCommandTool struct {
	Guard   engine.ExecGuard
	Approve ApprovalFunc
}

var _ Tool = (*OSCommandTool)(nil)

func (t *OSCommandTool) Name() string        { return "os_command" }
func (t *OSCommandTool) Description() string {
	return "Run a shell command on the host machine. Requires user approval before execution. Only allowlisted read-only commands are permitted (cat, git status, git log, go test, whoami, etc)."
}
func (t *OSCommandTool) ParameterSchema() string {
	return `{"command": "string (required) - the command to run, e.g. git status", "args": "array of strings (optional) - command arguments"}`
}

func (t *OSCommandTool) Definition() ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: JSONSchema{
				Type: "object",
				Properties: map[string]SchemaProperty{
					"command": {Type: "string", Description: "Command to run, e.g. git status"},
					"args":    {Type: "string", Description: "Space-separated additional arguments"},
				},
				Required: []string{"command"},
			},
		},
	}
}

func (t *OSCommandTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	cmdStr := strings.TrimSpace(asString(input["command"]))
	if cmdStr == "" {
		return nil, fmt.Errorf("command is required")
	}

	// Build the full command + args list for validation.
	parts := strings.Fields(cmdStr)
	if rawArgs, ok := input["args"]; ok {
		if argSlice, ok := rawArgs.([]any); ok {
			for _, arg := range argSlice {
				if s, ok := arg.(string); ok {
					parts = append(parts, s)
				}
			}
		}
	}

	// Validate via ExecGuard allowlist.
	if t.Guard != nil {
		if err := t.Guard.Validate(parts); err != nil {
			return map[string]any{
				"denied":  true,
				"reason":  err.Error(),
				"command": strings.Join(parts, " "),
			}, nil
		}
	}

	// Request human approval.
	description := fmt.Sprintf("Command: %s", strings.Join(parts, " "))
	if t.Approve == nil || !t.Approve(t.Name(), description) {
		return map[string]any{
			"denied":  true,
			"reason":  "User denied permission to execute this command.",
			"command": strings.Join(parts, " "),
		}, nil
	}

	// Execute with a 30-second timeout.
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, parts[0], parts[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("exec failed: %w", err)
		}
	}

	return map[string]any{
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"exit_code": exitCode,
		"command":   strings.Join(parts, " "),
	}, nil
}

// asString safely extracts a string from an interface value.
func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
