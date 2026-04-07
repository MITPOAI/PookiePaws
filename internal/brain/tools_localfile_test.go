package brain

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mitpoai/pookiepaws/internal/security"
)

func newFileSandbox(t *testing.T) *security.WorkspaceSandbox {
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

func TestReadLocalFileToolHappyPath(t *testing.T) {
	sb := newFileSandbox(t)
	if err := sb.WriteFile(context.Background(), "brand-guidelines.txt", []byte("Be bold.")); err != nil {
		t.Fatalf("write: %v", err)
	}
	tool := &ReadLocalFileTool{Sandbox: sb}
	result, err := tool.Execute(context.Background(), map[string]any{"path": "brand-guidelines.txt"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result["content"] != "Be bold." {
		t.Errorf("unexpected content: %v", result["content"])
	}
}

func TestReadLocalFileToolPathEscape(t *testing.T) {
	tool := &ReadLocalFileTool{Sandbox: newFileSandbox(t)}
	_, err := tool.Execute(context.Background(), map[string]any{"path": "../outside.txt"})
	if err == nil {
		t.Fatal("expected error for path escape")
	}
}

func TestReadLocalFileToolMissingPath(t *testing.T) {
	tool := &ReadLocalFileTool{Sandbox: newFileSandbox(t)}
	_, err := tool.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestReadLocalFileToolDefinition(t *testing.T) {
	def := (&ReadLocalFileTool{}).Definition()
	if def.Function.Name != "read_local_file" {
		t.Errorf("wrong name: %s", def.Function.Name)
	}
	if _, ok := def.Function.Parameters.Properties["path"]; !ok {
		t.Error("missing path property")
	}
}
