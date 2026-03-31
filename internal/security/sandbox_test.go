package security

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWorkspaceSandboxWriteAndRead(t *testing.T) {
	root := t.TempDir()
	sandbox, err := NewWorkspaceSandbox(filepath.Join(root, ".pookiepaws"), filepath.Join(root, ".pookiepaws", "workspace"))
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	if err := sandbox.WriteFile(context.Background(), "notes/test.txt", []byte("hello")); err != nil {
		t.Fatalf("write file: %v", err)
	}

	data, err := sandbox.ReadFile(context.Background(), "notes/test.txt")
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected data %q", string(data))
	}
}

func TestWorkspaceSandboxRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	sandbox, err := NewWorkspaceSandbox(filepath.Join(root, ".pookiepaws"), filepath.Join(root, ".pookiepaws", "workspace"))
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	if _, err := sandbox.ResolveWithinWorkspace("..\\outside.txt"); err == nil {
		t.Fatalf("expected traversal path to be rejected")
	}
}

func TestWorkspaceSandboxRejectsAbsolutePath(t *testing.T) {
	root := t.TempDir()
	sandbox, err := NewWorkspaceSandbox(filepath.Join(root, ".pookiepaws"), filepath.Join(root, ".pookiepaws", "workspace"))
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	absolute := filepath.Join(root, "outside.txt")
	if _, err := sandbox.ResolveWithinWorkspace(absolute); err == nil {
		t.Fatalf("expected absolute path to be rejected")
	}
}

func TestWorkspaceSandboxRejectsSymlinkSegments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not reliable on this Windows environment")
	}

	root := t.TempDir()
	runtimeRoot := filepath.Join(root, ".pookiepaws")
	workspaceRoot := filepath.Join(runtimeRoot, "workspace")
	sandbox, err := NewWorkspaceSandbox(runtimeRoot, workspaceRoot)
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	outsideDir := filepath.Join(root, "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	linkPath := filepath.Join(workspaceRoot, "linked")
	if err := os.Symlink(outsideDir, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if _, err := sandbox.ResolveWithinWorkspace(filepath.Join("linked", "payload.txt")); err == nil {
		t.Fatalf("expected symlink path segment to be rejected")
	}
}

func TestExecGuardAllowsReadOnlyCommands(t *testing.T) {
	guard := NewCommandExecGuard()
	if err := guard.Validate([]string{"git", "status"}); err != nil {
		t.Fatalf("expected git status to pass: %v", err)
	}
	if err := guard.Validate([]string{"go", "test", "./..."}); err != nil {
		t.Fatalf("expected go test to pass: %v", err)
	}
}

func TestExecGuardRejectsNonAllowlistedAndShellCommands(t *testing.T) {
	guard := NewCommandExecGuard()
	if err := guard.Validate([]string{"powershell", "-Command", "Get-ChildItem"}); err == nil {
		t.Fatalf("expected powershell to be rejected")
	}
	if err := guard.Validate([]string{"git", "commit", "-m", "unsafe"}); err == nil {
		t.Fatalf("expected git commit to be rejected")
	}
}
