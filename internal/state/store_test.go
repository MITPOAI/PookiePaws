package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/persistence"
)

func TestFileStoreCompactRoundTripAndJSONFallback(t *testing.T) {
	root := t.TempDir()

	jsonStore, err := NewFileStoreWithOptions(root, Options{Format: persistence.FormatJSON})
	if err != nil {
		t.Fatalf("json store: %v", err)
	}
	compactStore, err := NewFileStoreWithOptions(root, Options{Format: persistence.FormatCompactV1})
	if err != nil {
		t.Fatalf("compact store: %v", err)
	}

	now := time.Now().UTC()
	workflowJSON := engine.Workflow{
		ID:        "wf_json",
		Name:      "JSON workflow",
		Skill:     "utm-validator",
		Status:    engine.WorkflowCompleted,
		Input:     map[string]any{"url": "https://example.com"},
		Output:    map[string]any{"valid": true},
		CreatedAt: now.Add(-time.Minute),
		UpdatedAt: now.Add(-time.Minute),
	}
	workflowCompact := engine.Workflow{
		ID:        "wf_compact",
		Name:      "Compact workflow",
		Skill:     "utm-validator",
		Status:    engine.WorkflowCompleted,
		Input:     map[string]any{"url": "https://example.com/compact"},
		Output:    map[string]any{"valid": true},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := jsonStore.SaveWorkflow(context.Background(), workflowJSON); err != nil {
		t.Fatalf("save json workflow: %v", err)
	}
	if err := compactStore.SaveWorkflow(context.Background(), workflowCompact); err != nil {
		t.Fatalf("save compact workflow: %v", err)
	}

	listed, err := compactStore.ListWorkflows(context.Background())
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 workflows, got %d", len(listed))
	}

	loadedJSON, err := compactStore.GetWorkflow(context.Background(), workflowJSON.ID)
	if err != nil {
		t.Fatalf("load json fallback workflow: %v", err)
	}
	if loadedJSON.Name != workflowJSON.Name {
		t.Fatalf("expected json fallback workflow name %q, got %q", workflowJSON.Name, loadedJSON.Name)
	}

	loadedCompact, err := jsonStore.GetWorkflow(context.Background(), workflowCompact.ID)
	if err != nil {
		t.Fatalf("load compact fallback workflow: %v", err)
	}
	if loadedCompact.Name != workflowCompact.Name {
		t.Fatalf("expected compact fallback workflow name %q, got %q", workflowCompact.Name, loadedCompact.Name)
	}
}

func TestFileStoreCompactAuditReadBack(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStoreWithOptions(root, Options{Format: persistence.FormatCompactV1})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	older := time.Now().UTC().Add(-time.Minute)
	newer := older.Add(30 * time.Second)
	for _, event := range []engine.Event{
		{ID: "evt_1", Type: engine.EventWorkflowSubmitted, WorkflowID: "wf_1", Time: older, Source: "test", Payload: map[string]any{"skill": "utm-validator"}},
		{ID: "evt_2", Type: engine.EventSkillCompleted, WorkflowID: "wf_1", Time: newer, Source: "test", Payload: map[string]any{"status": "completed"}},
	} {
		if err := store.AppendAudit(context.Background(), event); err != nil {
			t.Fatalf("append audit: %v", err)
		}
	}

	events, err := ReadRecentAuditEntries(root, 10)
	if err != nil {
		t.Fatalf("read recent audit entries: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[1].ID != "evt_2" {
		t.Fatalf("expected newest event to be evt_2, got %s", events[1].ID)
	}
}

func TestCompactAuditCorruptionFailsSafely(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStoreWithOptions(root, Options{Format: persistence.FormatCompactV1})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if err := store.AppendAudit(context.Background(), engine.Event{
		ID:   "evt_1",
		Type: engine.EventWorkflowSubmitted,
		Time: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append audit: %v", err)
	}

	path := filepath.Join(root, "audits", activeAuditChunk)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read chunk: %v", err)
	}
	if len(data) < auditChunkHeaderSize+2 {
		t.Fatalf("expected audit chunk payload")
	}
	if err := os.WriteFile(path, data[:len(data)-2], 0o644); err != nil {
		t.Fatalf("truncate chunk: %v", err)
	}

	if _, err := ReadRecentAuditEntries(root, 10); err == nil {
		t.Fatalf("expected corruption read to fail")
	}
}

func BenchmarkAuditAppendJSON(b *testing.B) {
	benchmarkAuditAppend(b, persistence.FormatJSON)
}

func BenchmarkAuditAppendCompact(b *testing.B) {
	benchmarkAuditAppend(b, persistence.FormatCompactV1)
}

func benchmarkAuditAppend(b *testing.B, format persistence.Format) {
	root := b.TempDir()
	store, err := NewFileStoreWithOptions(root, Options{Format: format})
	if err != nil {
		b.Fatalf("new store: %v", err)
	}
	event := engine.Event{
		ID:         "evt_1",
		Type:       engine.EventAdapterExecuted,
		Time:       time.Now().UTC(),
		WorkflowID: "wf_bench",
		Source:     "benchmark",
		Payload: map[string]any{
			"adapter":   "mitto",
			"operation": "send_sms",
			"status":    "ok",
			"details": map[string]any{
				"campaign": "vip_launch",
				"segment":  "loyalists",
			},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		event.ID = "evt_" + time.Now().UTC().Format("150405.000000000")
		event.Time = time.Now().UTC()
		if err := store.AppendAudit(context.Background(), event); err != nil {
			b.Fatalf("append audit: %v", err)
		}
	}
}
