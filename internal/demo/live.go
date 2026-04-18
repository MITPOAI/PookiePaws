package demo

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/conv"
	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/research"
	"github.com/mitpoai/pookiepaws/internal/state"
)

func RunScenarioLiveSmoke(ctx context.Context, coord engine.WorkflowCoordinator, runtimeRoot, workspaceRoot string) (Result, error) {
	started := time.Now().UTC()
	scenario := DefaultScenario()
	result := Result{
		Status:   "failed",
		Passed:   false,
		Mode:     "live",
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

	analysisWorkflow, analysisStep, err := runLiveAnalysisWorkflow(ctx, coord, scenario)
	result.Checks = append(result.Checks, analysisStep)
	if err != nil {
		result.Error = err.Error()
		_ = SaveLatest(runtimeRoot, result)
		return result, err
	}
	result.AnalysisWorkflowID = analysisWorkflow.ID
	result.Provider = strings.TrimSpace(fmt.Sprint(analysisWorkflow.Output["provider"]))
	result.FallbackReason = strings.TrimSpace(fmt.Sprint(analysisWorkflow.Output["fallback_reason"]))
	result.Summary = strings.TrimSpace(fmt.Sprint(analysisWorkflow.Output["summary"]))
	result.SourceCount = intFromMap(analysisWorkflow.Output["coverage"], "kept")
	result.SkippedCount = intFromMap(analysisWorkflow.Output["coverage"], "skipped")
	result.Warnings = stringSliceValue(analysisWorkflow.Output["warnings"])

	reportMarkdown := renderLiveScenarioMarkdown(scenario, brief, analysisWorkflow.Output)
	exportWorkflowID, artifactPath, exportStep, err := runExportSkill(ctx, runtimeRoot, workspaceRoot, exportRequest{
		workflowID: "demo_live_smoke_export",
		name:       "Smoke: Export live research brief",
		title:      fmt.Sprintf("%s vs %s Live Research Smoke", scenario.Brand, scenario.Competitor),
		filename:   "live-research",
		content:    reportMarkdown,
		stepName:   "scenario.live.export",
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
			Name:     "scenario.live.audit",
			Passed:   false,
			Detail:   err.Error(),
			Duration: time.Since(auditStarted).Round(time.Millisecond).String(),
		})
		result.Error = err.Error()
		_ = SaveLatest(runtimeRoot, result)
		return result, err
	}
	result.Checks = append(result.Checks, Step{
		Name:     "scenario.live.audit",
		Passed:   true,
		Detail:   fmt.Sprintf("%d recent audit entries remain readable.", len(entries)),
		Duration: time.Since(auditStarted).Round(time.Millisecond).String(),
	})

	result.Passed = true
	result.Status = "passed"
	result.Duration = time.Since(started).Round(time.Millisecond).String()
	if result.Summary == "" {
		result.Summary = fmt.Sprintf("Saved a live research brief for %s against %s.", scenario.Brand, scenario.Competitor)
	}

	if result.ArtifactPath == "" {
		err := fmt.Errorf("live research export did not return an artifact path")
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

func runLiveAnalysisWorkflow(ctx context.Context, coord engine.WorkflowCoordinator, scenario Scenario) (engine.Workflow, Step, error) {
	started := time.Now()
	workflow, err := coord.SubmitWorkflow(ctx, engine.WorkflowDefinition{
		Name:  "Smoke: Live scenario business analysis",
		Skill: "mitpo-ba-researcher",
		Input: map[string]any{
			"company":     scenario.Brand,
			"competitors": []string{scenario.Competitor},
			"market":      scenario.Market,
			"focus_areas": append([]string(nil), scenario.FocusAreas...),
			"country":     "AU",
			"location":    "Australia",
			"max_sources": 6,
		},
	})
	if err != nil {
		return engine.Workflow{}, Step{
			Name:     "scenario.live.analysis",
			Passed:   false,
			Detail:   err.Error(),
			Duration: time.Since(started).Round(time.Millisecond).String(),
		}, err
	}
	if workflow.Status != engine.WorkflowCompleted {
		err = fmt.Errorf("analysis workflow finished with status %s", workflow.Status)
		return workflow, Step{
			Name:     "scenario.live.analysis",
			Passed:   false,
			Detail:   err.Error(),
			Duration: time.Since(started).Round(time.Millisecond).String(),
		}, err
	}
	detail := strings.TrimSpace(fmt.Sprint(workflow.Output["summary"]))
	if detail == "" {
		detail = "Live scenario business analysis completed."
	}
	discovered := intFromMap(workflow.Output["coverage"], "discovered")
	kept := intFromMap(workflow.Output["coverage"], "kept")
	skipped := intFromMap(workflow.Output["coverage"], "skipped")
	provider := strings.TrimSpace(fmt.Sprint(workflow.Output["provider"]))
	if provider != "" {
		detail = fmt.Sprintf("%s Provider: %s.", detail, provider)
	}
	detail = fmt.Sprintf("%s Discovered %d, kept %d, skipped %d.", detail, discovered, kept, skipped)
	return workflow, Step{
		Name:     "scenario.live.analysis",
		Passed:   true,
		Detail:   detail,
		Duration: time.Since(started).Round(time.Millisecond).String(),
	}, nil
}

func renderLiveScenarioMarkdown(scenario Scenario, brief map[string]any, output map[string]any) string {
	summary := strings.TrimSpace(fmt.Sprint(output["summary"]))
	if summary == "" {
		summary = "Live bounded research completed with public web sources."
	}
	findings := stringSliceValue(output["findings"])
	warnings := stringSliceValue(output["warnings"])
	sources := sourceViews(output["sources"])
	notes := competitorNoteViews(output["competitor_notes"])

	var builder strings.Builder
	builder.WriteString("## Scenario Brief\n\n")
	builder.WriteString(fmt.Sprintf("- Brand: %s\n- Competitor: %s\n- Market: %s\n- Offer: %s\n- Audience: %s\n\n",
		scenario.Brand, scenario.Competitor, scenario.Market, brief["offer"], brief["audience"]))

	builder.WriteString("## Executive Summary\n\n")
	builder.WriteString(summary)
	builder.WriteString("\n\n")

	if len(notes) > 0 {
		builder.WriteString("## Competitor Notes\n\n")
		for _, note := range notes {
			builder.WriteString(fmt.Sprintf("- %s\n", note))
		}
		builder.WriteString("\n")
	}

	if len(findings) > 0 {
		builder.WriteString("## Findings\n\n")
		for _, finding := range findings {
			builder.WriteString(fmt.Sprintf("- %s\n", finding))
		}
		builder.WriteString("\n")
	}

	builder.WriteString("## Recommendations\n\n")
	builder.WriteString("- Keep the PookiePaws positioning premium, warm, and occasion-led instead of price-led.\n")
	builder.WriteString("- Use one clear bundle offer and one simple upsell rather than a busy catalog.\n")
	builder.WriteString("- Mirror the competitor page types that performed best in the source set, but keep the tone more gift-oriented.\n\n")

	if len(sources) > 0 {
		builder.WriteString("## Sources\n\n")
		for _, source := range sources {
			builder.WriteString(fmt.Sprintf("- %s\n", source))
		}
		builder.WriteString("\n")
	}

	if len(warnings) > 0 {
		builder.WriteString("## Warnings\n\n")
		for _, warning := range warnings {
			builder.WriteString(fmt.Sprintf("- %s\n", warning))
		}
		builder.WriteString("\n")
	}

	builder.WriteString("## Next Actions\n\n")
	builder.WriteString(fmt.Sprintf("1. Draft a landing-page test for %s using the strongest positioning angle from this research.\n", scenario.Brand))
	builder.WriteString("2. Turn the best offer and pricing observations into one short SMS or email angle.\n")
	builder.WriteString("3. Re-run the live smoke with `debug=true` only when you need raw page content for troubleshooting.\n")
	return strings.TrimSpace(builder.String())
}

func sourceViews(value any) []string {
	switch typed := value.(type) {
	case []research.Source:
		result := make([]string, 0, len(typed))
		for _, source := range typed {
			line := fmt.Sprintf("%s (%s)", strings.TrimSpace(source.URL), firstNonEmptyString(source.PageType, source.Host))
			if title := strings.TrimSpace(source.Title); title != "" {
				line = fmt.Sprintf("%s - %s", title, line)
			}
			result = append(result, line)
		}
		return result
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		line := fmt.Sprintf("%s (%s)", strings.TrimSpace(conv.AsString(entry["url"])), firstNonEmptyString(conv.AsString(entry["page_type"]), conv.AsString(entry["host"])))
		if title := strings.TrimSpace(conv.AsString(entry["title"])); title != "" {
			line = fmt.Sprintf("%s - %s", title, line)
		}
		result = append(result, line)
	}
	return result
}

func competitorNoteViews(value any) []string {
	switch typed := value.(type) {
	case []research.CompetitorNote:
		result := make([]string, 0, len(typed))
		for _, note := range typed {
			if text := strings.TrimSpace(note.Note); text != "" {
				result = append(result, text)
			}
		}
		return result
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		line := strings.TrimSpace(conv.AsString(entry["note"]))
		if line == "" {
			continue
		}
		result = append(result, line)
	}
	return result
}

func stringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(conv.AsString(item)); text != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}

func intFromMap(value any, key string) int {
	if typed, ok := value.(research.Coverage); ok {
		switch key {
		case "discovered":
			return typed.Discovered
		case "scraped":
			return typed.Scraped
		case "kept":
			return typed.Kept
		case "skipped":
			return typed.Skipped
		case "queries":
			return typed.Queries
		}
	}
	entry, ok := value.(map[string]any)
	if !ok {
		return 0
	}
	switch typed := entry[key].(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return 0
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
