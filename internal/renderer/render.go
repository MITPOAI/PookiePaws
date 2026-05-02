package renderer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type RenderOptions struct {
	ScriptPath string
	Python     string
}

func SavePlan(path string, plan EditPlan) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func LoadPlan(path string) (EditPlan, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return EditPlan{}, err
	}
	var plan EditPlan
	if err := json.Unmarshal(b, &plan); err != nil {
		return EditPlan{}, err
	}
	if plan.Width <= 0 || plan.Height <= 0 {
		plan.Width, plan.Height = DimensionsForFormat(plan.Format)
	}
	if plan.FPS <= 0 {
		plan.FPS = 30
	}
	if len(plan.Scenes) == 0 {
		return EditPlan{}, errors.New("edit plan must include at least one scene")
	}
	return plan, nil
}

func Render(ctx context.Context, planPath, outputPath string, options RenderOptions) error {
	if _, err := LoadPlan(planPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	script := options.ScriptPath
	if script == "" {
		script = filepath.Join("scripts", "media", "render.py")
	}
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("render script not found at %s: %w", script, err)
	}
	python := options.Python
	if python == "" {
		python = os.Getenv("PYTHON")
	}
	if python == "" {
		python = "python"
	}

	cmd := exec.CommandContext(ctx, python, script, "--plan", planPath, "--out", outputPath)
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if err := cmd.Run(); err != nil {
		if python == "python" {
			fallback := exec.CommandContext(ctx, "python3", script, "--plan", planPath, "--out", outputPath)
			combined.Reset()
			fallback.Stdout = &combined
			fallback.Stderr = &combined
			if fallbackErr := fallback.Run(); fallbackErr == nil {
				return nil
			}
		}
		return fmt.Errorf("%w\n%s", err, combined.String())
	}
	return nil
}
