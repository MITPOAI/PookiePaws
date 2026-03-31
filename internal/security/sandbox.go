package security

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type WorkspaceSandbox struct {
	runtimeRoot   string
	workspaceRoot string
}

var _ engine.Sandbox = (*WorkspaceSandbox)(nil)

func NewWorkspaceSandbox(runtimeRoot, workspaceRoot string) (*WorkspaceSandbox, error) {
	resolvedRuntime, err := canonicalizeRoot(runtimeRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime root: %w", err)
	}
	resolvedWorkspace, err := canonicalizeRoot(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	if !isWithin(resolvedRuntime, resolvedWorkspace) {
		return nil, fmt.Errorf("workspace root %q must stay within runtime root %q", resolvedWorkspace, resolvedRuntime)
	}
	if err := os.MkdirAll(filepath.Join(resolvedWorkspace, "skills"), 0o755); err != nil {
		return nil, err
	}

	return &WorkspaceSandbox{
		runtimeRoot:   resolvedRuntime,
		workspaceRoot: resolvedWorkspace,
	}, nil
}

func (s *WorkspaceSandbox) RuntimeRoot() string {
	return s.runtimeRoot
}

func (s *WorkspaceSandbox) WorkspaceRoot() string {
	return s.workspaceRoot
}

func (s *WorkspaceSandbox) ResolveWithinWorkspace(path string) (string, error) {
	relativePath, err := normalizeRelativePath(path)
	if err != nil {
		return "", err
	}

	candidate := filepath.Join(s.workspaceRoot, relativePath)
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	if !isWithin(s.workspaceRoot, candidate) {
		return "", fmt.Errorf("path %q escapes workspace", path)
	}
	if err := ensureExistingPathContained(s.workspaceRoot, candidate); err != nil {
		return "", err
	}
	if err := rejectSymlinkSegments(s.workspaceRoot, candidate); err != nil {
		return "", err
	}
	return candidate, nil
}

func (s *WorkspaceSandbox) ReadFile(_ context.Context, path string) ([]byte, error) {
	resolved, err := s.ResolveWithinWorkspace(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Lstat(resolved)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("symlinks are not allowed in workspace reads: %q", path)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path %q is a directory", path)
	}
	return os.ReadFile(resolved)
}

func (s *WorkspaceSandbox) WriteFile(_ context.Context, path string, data []byte) error {
	resolved, err := s.ResolveWithinWorkspace(path)
	if err != nil {
		return err
	}

	parent := filepath.Dir(resolved)
	if err := ensureDirectoryTree(parent, s.workspaceRoot); err != nil {
		return err
	}

	if info, err := os.Lstat(resolved); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed in workspace writes: %q", path)
		}
		if info.IsDir() {
			return fmt.Errorf("path %q is a directory", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	tmp, err := os.CreateTemp(parent, ".pookiepaws-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, resolved)
}

func canonicalizeRoot(path string) (string, error) {
	cleaned := filepath.Clean(path)
	absolute, err := filepath.Abs(cleaned)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}

func normalizeRelativePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute paths are not allowed: %q", path)
	}

	cleaned := filepath.Clean(path)
	if cleaned == "." {
		return "", fmt.Errorf("path must point to a file")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes workspace", path)
	}
	return cleaned, nil
}

func ensureExistingPathContained(workspaceRoot, candidate string) error {
	existingParent, err := nearestExistingParent(candidate)
	if err != nil {
		return err
	}
	resolvedParent, err := filepath.EvalSymlinks(existingParent)
	if err != nil {
		return err
	}
	if !isWithin(workspaceRoot, resolvedParent) {
		return fmt.Errorf("path %q escapes workspace", candidate)
	}
	return nil
}

func rejectSymlinkSegments(workspaceRoot, candidate string) error {
	current := workspaceRoot
	relative, err := filepath.Rel(workspaceRoot, candidate)
	if err != nil {
		return err
	}
	if relative == "." {
		return nil
	}

	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path segment is not allowed: %q", current)
		}
	}
	return nil
}

func ensureDirectoryTree(targetDir, workspaceRoot string) error {
	relative, err := filepath.Rel(workspaceRoot, targetDir)
	if err != nil {
		return err
	}
	if relative == "." {
		return nil
	}

	current := workspaceRoot
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o755); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink directory segment is not allowed: %q", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("path segment %q is not a directory", current)
		}
	}
	return nil
}

func nearestExistingParent(path string) (string, error) {
	current := path
	for {
		if _, err := os.Stat(current); err == nil {
			return current, nil
		}
		next := filepath.Dir(current)
		if next == current {
			return "", fmt.Errorf("no existing parent for %q", path)
		}
		current = next
	}
}

func isWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && rel != "..")
}
