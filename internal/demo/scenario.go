package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/security"
	"github.com/mitpoai/pookiepaws/internal/skills"
	"github.com/mitpoai/pookiepaws/internal/state"
)

const latestScenarioSmokeFilename = "demo-smoke.json"
const latestLiveScenarioSmokeFilename = "demo-smoke-live.json"

type Scenario struct {
	Brand       string   `json:"brand"`
	Competitor  string   `json:"competitor"`
	Market      string   `json:"market"`
	FocusAreas  []string `json:"focus_areas"`
	BrandDomain string   `json:"brand_domain"`
}

type Step struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	Detail   string `json:"detail"`
	Duration string `json:"duration"`
}

type Result struct {
	Status             string    `json:"status"`
	Passed             bool      `json:"passed"`
	Mode               string    `json:"mode,omitempty"`
	LastRun            time.Time `json:"last_run"`
	Duration           string    `json:"duration,omitempty"`
	Summary            string    `json:"summary,omitempty"`
	ArtifactPath       string    `json:"artifact_path,omitempty"`
	Error              string    `json:"error,omitempty"`
	Provider           string    `json:"provider,omitempty"`
	FallbackReason     string    `json:"fallback_reason,omitempty"`
	SourceCount        int       `json:"source_count,omitempty"`
	SkippedCount       int       `json:"skipped_count,omitempty"`
	Warnings           []string  `json:"warnings,omitempty"`
	Scenario           Scenario  `json:"scenario"`
	AnalysisWorkflowID string    `json:"analysis_workflow_id,omitempty"`
	ExportWorkflowID   string    `json:"export_workflow_id,omitempty"`
	Checks             []Step    `json:"checks,omitempty"`
}

func DefaultScenario() Scenario {
	return Scenario{
		Brand:       "PookiePaws Reserve",
		Competitor:  "OpenClaw",
		Market:      "AU pet gifting",
		FocusAreas:  []string{"pricing", "positioning", "tone", "offer structure"},
		BrandDomain: "pookiepawsreserve.example",
	}
}

func RunScenarioSmoke(ctx context.Context, coord engine.WorkflowCoordinator, runtimeRoot, workspaceRoot string) (Result, error) {
	started := time.Now().UTC()
	scenario := DefaultScenario()
	result := Result{
		Status:   "failed",
		Passed:   false,
		Mode:     "deterministic",
		LastRun:  started,
		Scenario: scenario,
		Checks:   make([]Step, 0, 4),
	}

	if coord == nil {
		err := fmt.Errorf("workflow coordinator is required")
		result.Error = err.Error()
		_ = SaveLatest(runtimeRoot, result)
		return result, err
	}

	brief, briefStep, err := buildScenarioBrief(scenario)
	result.Checks = append(result.Checks, briefStep)
	if err != nil {
		result.Error = err.Error()
		_ = SaveLatest(runtimeRoot, result)
		return result, err
	}

	analysisWorkflow, analysisStep, err := runAnalysisWorkflow(ctx, coord, scenario)
	result.Checks = append(result.Checks, analysisStep)
	if err != nil {
		result.Error = err.Error()
		_ = SaveLatest(runtimeRoot, result)
		return result, err
	}
	result.AnalysisWorkflowID = analysisWorkflow.ID
	result.Provider = strings.TrimSpace(fmt.Sprint(analysisWorkflow.Output["provider"]))
	result.FallbackReason = strings.TrimSpace(fmt.Sprint(analysisWorkflow.Output["fallback_reason"]))
	result.SourceCount = intFromMap(analysisWorkflow.Output["coverage"], "kept")
	result.SkippedCount = intFromMap(analysisWorkflow.Output["coverage"], "skipped")
	result.Warnings = stringSliceValue(analysisWorkflow.Output["warnings"])

	reportMarkdown := renderScenarioMarkdown(scenario, brief, analysisWorkflow)
	exportWorkflowID, artifactPath, exportStep, err := runExportSkill(ctx, runtimeRoot, workspaceRoot, exportRequest{
		workflowID: "demo_smoke_export",
		name:       "Smoke: Export scenario brief",
		title:      fmt.Sprintf("%s vs %s Demo Smoke", scenario.Brand, scenario.Competitor),
		filename:   "smoke-scenario",
		content:    reportMarkdown,
		stepName:   "scenario.export",
	})
	result.Checks = append(result.Checks, exportStep)
	if err != nil {
		result.Error = err.Error()
		_ = SaveLatest(runtimeRoot, result)
		return result, err
	}
	result.ExportWorkflowID = exportWorkflowID
	result.ArtifactPath = artifactPath

	auditStarted := time.Now()
	entries, err := state.ReadRecentAuditEntries(filepath.Join(runtimeRoot, "state"), 8)
	if err != nil {
		result.Checks = append(result.Checks, Step{
			Name:     "scenario.audit",
			Passed:   false,
			Detail:   err.Error(),
			Duration: time.Since(auditStarted).Round(time.Millisecond).String(),
		})
		result.Error = err.Error()
		_ = SaveLatest(runtimeRoot, result)
		return result, err
	}
	result.Checks = append(result.Checks, Step{
		Name:     "scenario.audit",
		Passed:   true,
		Detail:   fmt.Sprintf("%d recent audit entries remain readable.", len(entries)),
		Duration: time.Since(auditStarted).Round(time.Millisecond).String(),
	})

	result.Passed = true
	result.Status = "passed"
	result.Duration = time.Since(started).Round(time.Millisecond).String()
	result.Summary = fmt.Sprintf(
		"Saved a deterministic demo brief for %s against %s in the %s market.",
		scenario.Brand,
		scenario.Competitor,
		scenario.Market,
	)

	if result.ArtifactPath == "" {
		err := fmt.Errorf("scenario export did not return an artifact path")
		result.Passed = false
		result.Status = "failed"
		result.Error = err.Error()
		_ = SaveLatest(runtimeRoot, result)
		return result, err
	}

	if err := SaveLatest(runtimeRoot, result); err != nil {
		result.Passed = false
		result.Status = "failed"
		result.Error = err.Error()
		return result, err
	}
	return result, nil
}

func buildScenarioBrief(scenario Scenario) (map[string]any, Step, error) {
	started := time.Now()
	brief := map[string]any{
		"brand":       scenario.Brand,
		"competitor":  scenario.Competitor,
		"market":      scenario.Market,
		"offer":       "premium pet gifting bundles with occasion-based packaging",
		"audience":    "gift buyers shopping for premium pet-owner moments",
		"positioning": "premium, warm, occasion-led gifting for pet lovers who want more than commodity toys",
	}
	return brief, Step{
		Name:     "scenario.brief",
		Passed:   true,
		Detail:   fmt.Sprintf("Created the deterministic brief for %s in %s.", scenario.Brand, scenario.Market),
		Duration: time.Since(started).Round(time.Millisecond).String(),
	}, nil
}

func runAnalysisWorkflow(ctx context.Context, coord engine.WorkflowCoordinator, scenario Scenario) (engine.Workflow, Step, error) {
	started := time.Now()
	workflow, err := coord.SubmitWorkflow(ctx, engine.WorkflowDefinition{
		Name:  "Smoke: Scenario business analysis",
		Skill: "mitpo-ba-researcher",
		Input: map[string]any{
			"company":     scenario.Brand,
			"market":      scenario.Market,
			"domains":     []string{scenario.BrandDomain, strings.ToLower(strings.ReplaceAll(scenario.Competitor, " ", "")) + ".example"},
			"focus_areas": append([]string(nil), scenario.FocusAreas...),
		},
	})
	if err != nil {
		return engine.Workflow{}, Step{
			Name:     "scenario.analysis",
			Passed:   false,
			Detail:   err.Error(),
			Duration: time.Since(started).Round(time.Millisecond).String(),
		}, err
	}
	if workflow.Status != engine.WorkflowCompleted {
		err = fmt.Errorf("analysis workflow finished with status %s", workflow.Status)
		return workflow, Step{
			Name:     "scenario.analysis",
			Passed:   false,
			Detail:   err.Error(),
			Duration: time.Since(started).Round(time.Millisecond).String(),
		}, err
	}
	detail := strings.TrimSpace(fmt.Sprint(workflow.Output["summary"]))
	if detail == "" {
		detail = "Scenario business analysis completed."
	}
	if provider := strings.TrimSpace(fmt.Sprint(workflow.Output["provider"])); provider != "" {
		detail = fmt.Sprintf("%s Provider: %s.", detail, provider)
	}
	return workflow, Step{
		Name:     "scenario.analysis",
		Passed:   true,
		Detail:   detail,
		Duration: time.Since(started).Round(time.Millisecond).String(),
	}, nil
}

type exportRequest struct {
	workflowID string
	name       string
	title      string
	filename   string
	content    string
	stepName   string
}

func runExportSkill(ctx context.Context, runtimeRoot, workspaceRoot string, export exportRequest) (string, string, Step, error) {
	started := time.Now()
	sandbox, err := security.NewWorkspaceSandbox(runtimeRoot, workspaceRoot)
	if err != nil {
		return "", "", Step{
			Name:     export.stepName,
			Passed:   false,
			Detail:   err.Error(),
			Duration: time.Since(started).Round(time.Millisecond).String(),
		}, err
	}

	registry, err := skills.NewDefaultRegistry()
	if err != nil {
		return "", "", Step{
			Name:     export.stepName,
			Passed:   false,
			Detail:   err.Error(),
			Duration: time.Since(started).Round(time.Millisecond).String(),
		}, err
	}
	skill, ok := registry.Get("mitpo-markdown-export")
	if !ok {
		err = fmt.Errorf("mitpo-markdown-export skill is not registered")
		return "", "", Step{
			Name:     export.stepName,
			Passed:   false,
			Detail:   err.Error(),
			Duration: time.Since(started).Round(time.Millisecond).String(),
		}, err
	}

	result, err := skill.Execute(ctx, engine.SkillRequest{
		Workflow: engine.Workflow{
			ID:     export.workflowID,
			Name:   export.name,
			Skill:  "mitpo-markdown-export",
			Status: engine.WorkflowRunning,
		},
		Input: map[string]any{
			"title":    export.title,
			"filename": export.filename,
			"content":  export.content,
		},
		Sandbox: sandbox,
		Now:     time.Now().UTC(),
	})
	if err != nil {
		return "", "", Step{
			Name:     export.stepName,
			Passed:   false,
			Detail:   err.Error(),
			Duration: time.Since(started).Round(time.Millisecond).String(),
		}, err
	}
	path := strings.TrimSpace(fmt.Sprint(result.Output["path"]))
	detail := "Scenario report exported."
	if path != "" {
		detail = "Saved demo report to " + path
	}
	return export.workflowID, path, Step{
		Name:     export.stepName,
		Passed:   true,
		Detail:   detail,
		Duration: time.Since(started).Round(time.Millisecond).String(),
	}, nil
}

func renderScenarioMarkdown(scenario Scenario, brief map[string]any, analysis engine.Workflow) string {
	analysisSummary := strings.TrimSpace(fmt.Sprint(analysis.Output["summary"]))
	if analysisSummary == "" {
		analysisSummary = "The deterministic business-analysis step completed with structured offline inputs."
	}
	return strings.TrimSpace(fmt.Sprintf(`
## Scenario Brief

- Brand: %s
- Competitor: %s
- Market: %s
- Offer: %s
- Audience: %s

## Comparison Notes

- %s should lean into premium gifting bundles and occasion-led packaging rather than generic commodity pet offers.
- %s is treated as a lower-friction competitor with a more price-forward and functional tone in this deterministic smoke scenario.
- The demo keeps the research offline and structured, so these notes are scenario inputs rather than live web claims.

## Structured Analysis Output

%s

Focus areas:
- %s

## Recommendations

- Lead with premium gifting language, warm tone, and curated bundle framing.
- Keep the hero offer simple: one occasion-led bundle, one light upsell, and one clear CTA.
- Avoid competing on price alone; compete on packaging, gifting intent, and emotional clarity.

## Next Actions

1. Draft a landing-page headline test for %s.
2. Draft a short retention SMS that reinforces bundle value without discount-heavy positioning.
3. Validate one campaign URL and one export path as part of the operator smoke flow.
`, scenario.Brand, scenario.Competitor, scenario.Market, brief["offer"], brief["audience"], scenario.Brand, scenario.Competitor, analysisSummary, strings.Join(scenario.FocusAreas, ", "), scenario.Brand))
}

func latestResultPath(runtimeRoot, mode string) string {
	filename := latestScenarioSmokeFilename
	if strings.EqualFold(strings.TrimSpace(mode), "live") {
		filename = latestLiveScenarioSmokeFilename
	}
	return filepath.Join(runtimeRoot, "state", "runtime", filename)
}

func LoadLatest(runtimeRoot string) (*Result, error) {
	return LoadLatestMode(runtimeRoot, "deterministic")
}

func LoadLatestMode(runtimeRoot, mode string) (*Result, error) {
	data, err := os.ReadFile(latestResultPath(runtimeRoot, mode))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if result.Mode == "" {
		result.Mode = mode
	}
	return &result, nil
}

func SaveLatest(runtimeRoot string, result Result) error {
	mode := strings.TrimSpace(result.Mode)
	if mode == "" {
		mode = "deterministic"
	}
	path := latestResultPath(runtimeRoot, mode)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
