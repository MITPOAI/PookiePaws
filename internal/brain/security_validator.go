package brain

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

// SecurityValidator intercepts native tool call arguments before execution.
// It enforces path confinement for file tools and human-in-the-loop approval
// for OS command execution.
type SecurityValidator struct {
	sandbox    engine.Sandbox
	approvalFn ApprovalFunc // nil → deny all approval-gated tools
}

// NewSecurityValidator creates a validator using the given sandbox and approval function.
func NewSecurityValidator(sandbox engine.Sandbox, approvalFn ApprovalFunc) *SecurityValidator {
	return &SecurityValidator{sandbox: sandbox, approvalFn: approvalFn}
}

// Validate checks whether a tool call is permitted before execution.
//   - Returns (blockedResult, nil) when the call is blocked/denied.
//   - Returns (nil, nil) when the call is approved and may proceed.
//   - Returns (nil, err) on internal JSON parse errors.
func (v *SecurityValidator) Validate(toolName, argsJSON string) (map[string]any, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("invalid tool arguments: %w", err)
	}

	switch toolName {
	case "export_markdown":
		return v.validatePath(asString(args["filename"]))
	case "read_local_file":
		return v.validatePath(asString(args["path"]))
	case "os_command":
		return v.requireHITL(asString(args["command"]))
	}
	return nil, nil
}

func (v *SecurityValidator) validatePath(rawPath string) (map[string]any, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return nil, nil // let the tool itself reject a missing required arg
	}
	if _, err := v.sandbox.ResolveWithinWorkspace(rawPath); err != nil {
		return map[string]any{
			"error":  "Security Violation: Path out of bounds.",
			"detail": err.Error(),
		}, nil
	}
	return nil, nil
}

func (v *SecurityValidator) requireHITL(cmdStr string) (map[string]any, error) {
	description := fmt.Sprintf("[⚠] Pookie wants to run: %s. Allow? [Y/n]", cmdStr)
	if v.approvalFn == nil || !v.approvalFn("os_command", description) {
		return map[string]any{
			"denied":  true,
			"reason":  "User denied permission to execute this command.",
			"command": cmdStr,
		}, nil
	}
	return nil, nil
}
