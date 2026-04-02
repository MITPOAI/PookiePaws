package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func resolveSecretsPath(homeOverride string) (runtimeRoot, secretsPath string, err error) {
	if homeOverride != "" {
		root := filepath.Clean(homeOverride)
		return root, filepath.Join(root, ".security.json"), nil
	}
	if custom := os.Getenv("POOKIEPAWS_HOME"); custom != "" {
		root := filepath.Clean(custom)
		return root, filepath.Join(root, ".security.json"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	root := filepath.Join(home, ".pookiepaws")
	return root, filepath.Join(root, ".security.json"), nil
}

func saveSecurityConfig(runtimeRoot, secretsPath string, payload map[string]string) error {
	if runtimeRoot == "" || secretsPath == "" {
		return fmt.Errorf("runtime root and config path are required")
	}
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("serialise configuration: %w", err)
	}

	tmp := secretsPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := os.Rename(tmp, secretsPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace config file: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(secretsPath, 0o600); err != nil {
			return fmt.Errorf("lock config permissions: %w", err)
		}
	}
	return nil
}
