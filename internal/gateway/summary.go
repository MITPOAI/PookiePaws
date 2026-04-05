package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/adapters"
	"github.com/mitpoai/pookiepaws/internal/engine"
)

// SummaryGenerator builds daily workflow summaries for webhook delivery.
type SummaryGenerator struct {
	store engine.StateStore
}

// NewSummaryGenerator creates a generator backed by the given state store.
func NewSummaryGenerator(store engine.StateStore) *SummaryGenerator {
	return &SummaryGenerator{store: store}
}

// GenerateDailySummary queries the last 24 hours of workflows and produces a
// WebhookPayload suitable for Slack or Discord delivery.
func (g *SummaryGenerator) GenerateDailySummary(ctx context.Context) (adapters.WebhookPayload, error) {
	workflows, err := g.store.ListWorkflows(ctx)
	if err != nil {
		return adapters.WebhookPayload{}, fmt.Errorf("list workflows: %w", err)
	}

	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	var completed, failed, pending, blocked int
	skills := map[string]int{}

	for _, wf := range workflows {
		if wf.CreatedAt.Before(cutoff) {
			continue
		}
		switch wf.Status {
		case engine.WorkflowCompleted:
			completed++
		case engine.WorkflowFailed:
			failed++
		case engine.WorkflowWaitingApproval, engine.WorkflowQueued, engine.WorkflowRunning:
			pending++
		case engine.WorkflowRejected:
			blocked++
		}
		if wf.Skill != "" {
			skills[wf.Skill]++
		}
	}

	total := completed + failed + pending + blocked

	// Build the summary text.
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Total workflows in the last 24 hours: %d\n", total))
	summary.WriteString(fmt.Sprintf("Completed: %d | Failed: %d | Pending: %d | Blocked: %d", completed, failed, pending, blocked))

	if len(skills) > 0 {
		summary.WriteString("\n\nSkill breakdown:")
		for skill, count := range skills {
			summary.WriteString(fmt.Sprintf("\n- %s: %d", skill, count))
		}
	}

	fields := map[string]string{
		"Completed": fmt.Sprintf("%d", completed),
		"Failed":    fmt.Sprintf("%d", failed),
		"Pending":   fmt.Sprintf("%d", pending),
		"Blocked":   fmt.Sprintf("%d", blocked),
	}

	return adapters.WebhookPayload{
		Title:     "PookiePaws Daily Summary",
		Summary:   summary.String(),
		Timestamp: time.Now().UTC(),
		Fields:    fields,
	}, nil
}
