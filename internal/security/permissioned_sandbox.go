package security

import (
	"context"
	"fmt"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

// FileAccessApprover creates file access requests and waits for decisions.
type FileAccessApprover interface {
	RequestFileAccess(ctx context.Context, path string, mode engine.FileAccessMode, requester string) (engine.FilePermission, error)
	WaitForDecision(ctx context.Context, permID string) (engine.FilePermission, error)
}

// PermissionedSandbox wraps a WorkspaceSandbox with approval-gated file access.
// All read/write operations require explicit operator approval before proceeding.
type PermissionedSandbox struct {
	inner    *WorkspaceSandbox
	approver FileAccessApprover
	bus      engine.EventBus
}

var _ engine.Sandbox = (*PermissionedSandbox)(nil)

func NewPermissionedSandbox(inner *WorkspaceSandbox, approver FileAccessApprover, bus engine.EventBus) *PermissionedSandbox {
	return &PermissionedSandbox{
		inner:    inner,
		approver: approver,
		bus:      bus,
	}
}

func (s *PermissionedSandbox) RuntimeRoot() string {
	return s.inner.RuntimeRoot()
}

func (s *PermissionedSandbox) WorkspaceRoot() string {
	return s.inner.WorkspaceRoot()
}

func (s *PermissionedSandbox) ResolveWithinWorkspace(path string) (string, error) {
	return s.inner.ResolveWithinWorkspace(path)
}

func (s *PermissionedSandbox) ReadFile(ctx context.Context, path string) ([]byte, error) {
	perm, err := s.approver.RequestFileAccess(ctx, path, engine.FileAccessRead, "skill")
	if err != nil {
		return nil, fmt.Errorf("file access request failed: %w", err)
	}

	decision, err := s.approver.WaitForDecision(ctx, perm.ID)
	if err != nil {
		s.publishDenied(path, engine.FileAccessRead, "timeout or context canceled")
		return nil, fmt.Errorf("file access decision failed: %w", err)
	}

	if decision.State != engine.ApprovalApproved {
		s.publishDenied(path, engine.FileAccessRead, string(decision.State))
		return nil, fmt.Errorf("file read denied for %q: %s", path, decision.State)
	}

	return s.inner.ReadFile(ctx, path)
}

func (s *PermissionedSandbox) WriteFile(ctx context.Context, path string, data []byte) error {
	perm, err := s.approver.RequestFileAccess(ctx, path, engine.FileAccessWrite, "skill")
	if err != nil {
		return fmt.Errorf("file access request failed: %w", err)
	}

	decision, err := s.approver.WaitForDecision(ctx, perm.ID)
	if err != nil {
		s.publishDenied(path, engine.FileAccessWrite, "timeout or context canceled")
		return fmt.Errorf("file access decision failed: %w", err)
	}

	if decision.State != engine.ApprovalApproved {
		s.publishDenied(path, engine.FileAccessWrite, string(decision.State))
		return fmt.Errorf("file write denied for %q: %s", path, decision.State)
	}

	return s.inner.WriteFile(ctx, path, data)
}

func (s *PermissionedSandbox) publishDenied(path string, mode engine.FileAccessMode, reason string) {
	_ = s.bus.Publish(engine.Event{
		Type:   engine.EventFileAccessDenied,
		Source: "permissioned-sandbox",
		Payload: map[string]any{
			"path":   path,
			"mode":   string(mode),
			"reason": reason,
		},
	})
}
