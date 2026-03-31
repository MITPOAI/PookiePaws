package main

import (
	"os"
	"path/filepath"
)

// resolveRoots returns (runtimeRoot, workspaceRoot) using the following
// priority: explicit flag > POOKIEPAWS_HOME env var > ~/.pookiepaws.
// All paths are constructed with filepath.Join so they resolve correctly on
// Windows (C:\Users\...) and Unix systems alike.
func resolveRoots(homeOverride string) (runtimeRoot, workspaceRoot string, err error) {
	if homeOverride != "" {
		return homeOverride, filepath.Join(homeOverride, "workspace"), nil
	}
	if custom := os.Getenv("POOKIEPAWS_HOME"); custom != "" {
		return custom, filepath.Join(custom, "workspace"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	root := filepath.Join(home, ".pookiepaws")
	return root, filepath.Join(root, "workspace"), nil
}
