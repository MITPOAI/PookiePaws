package brain

import (
	"path/filepath"
	"testing"

	"github.com/mitpoai/pookiepaws/internal/security"
)

func newTestSandbox(t *testing.T) *security.WorkspaceSandbox {
	t.Helper()
	root := t.TempDir()
	sb, err := security.NewWorkspaceSandbox(
		filepath.Join(root, ".pookiepaws"),
		filepath.Join(root, ".pookiepaws", "workspace"),
	)
	if err != nil {
		t.Fatalf("sandbox: %v", err)
	}
	return sb
}

func newTestSandboxAt(t *testing.T, root string) (*security.WorkspaceSandbox, error) {
	return security.NewWorkspaceSandbox(
		filepath.Join(root, ".pookiepaws"),
		filepath.Join(root, ".pookiepaws", "workspace"),
	)
}

func TestSecurityValidatorPathInBounds(t *testing.T) {
	v := NewSecurityValidator(newTestSandbox(t), nil)
	blocked, err := v.Validate("export_markdown", `{"content":"hello","filename":"exports/out"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked != nil {
		t.Fatalf("expected nil blocked, got %v", blocked)
	}
}

func TestSecurityValidatorPathOutOfBounds(t *testing.T) {
	v := NewSecurityValidator(newTestSandbox(t), nil)
	blocked, err := v.Validate("read_local_file", `{"path":"../escape.txt"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked == nil {
		t.Fatal("expected blocked result for traversal path")
	}
	if _, ok := blocked["error"]; !ok {
		t.Errorf("blocked result missing 'error' key: %v", blocked)
	}
}

func TestSecurityValidatorOSCommandApproved(t *testing.T) {
	v := NewSecurityValidator(newTestSandbox(t), func(string, string) bool { return true })
	blocked, err := v.Validate("os_command", `{"command":"git status"}`)
	if err != nil || blocked != nil {
		t.Fatalf("expected approval: blocked=%v err=%v", blocked, err)
	}
}

func TestSecurityValidatorOSCommandDenied(t *testing.T) {
	v := NewSecurityValidator(newTestSandbox(t), func(string, string) bool { return false })
	blocked, _ := v.Validate("os_command", `{"command":"git status"}`)
	if blocked == nil {
		t.Fatal("expected blocked result when user denies")
	}
	if blocked["denied"] != true {
		t.Errorf("expected denied=true, got %v", blocked)
	}
}

func TestSecurityValidatorOSCommandNilApproval(t *testing.T) {
	v := NewSecurityValidator(newTestSandbox(t), nil)
	blocked, _ := v.Validate("os_command", `{"command":"git status"}`)
	if blocked == nil {
		t.Fatal("expected block when approvalFn is nil")
	}
}

func TestSecurityValidatorUnknownTool(t *testing.T) {
	v := NewSecurityValidator(newTestSandbox(t), nil)
	blocked, err := v.Validate("web_search", `{"url":"https://example.com"}`)
	if err != nil || blocked != nil {
		t.Fatalf("unknown tool should pass: blocked=%v err=%v", blocked, err)
	}
}
