package browser

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"
)

type WorkflowResult struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	DryRun    bool      `json:"dry_run"`
	Steps     int       `json:"steps"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

func RunWorkflow(ctx context.Context, path string, dryRun bool) (WorkflowResult, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowResult{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return WorkflowResult{}, err
	}
	steps := countWorkflowSteps(string(b))
	result := WorkflowResult{
		Name:      workflowName(string(b)),
		Path:      path,
		DryRun:    dryRun,
		Steps:     steps,
		CreatedAt: time.Now().UTC(),
	}
	if dryRun {
		result.Message = "workflow parsed for dry-run only; no browser was opened and no posting/uploading was attempted"
		return result, nil
	}
	return result, errors.New("Playwright browser execution is scaffolded for the MVP; run with --dry-run until the adapter is implemented")
}

func Open(ctx context.Context, url string, dryRun bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(url) == "" {
		return errors.New("browser open requires --url")
	}
	if dryRun {
		return nil
	}
	return errors.New("Playwright browser open is scaffolded for the MVP; use --dry-run for transparent previews")
}

func Record(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("browser recording is planned but not implemented in the MVP")
}

func countWorkflowSteps(raw string) int {
	count := 0
	for _, line := range strings.Split(raw, "\n") {
		if strings.Contains(line, "action:") {
			count++
		}
	}
	return count
}

func workflowName(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "name:")), `"`)
		}
	}
	return "browser_workflow"
}
