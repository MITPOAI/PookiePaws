package brain

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/persistence"
)

func TestPersistentMemoryRecordsWorkflowAndFlushesContext(t *testing.T) {
	root := t.TempDir()
	bus := engine.NewEventBus()
	subscription := bus.Subscribe(1)
	defer bus.Unsubscribe(subscription.ID)

	memory, err := NewPersistentMemory(root, fakeProviderFactory{
		provider: fakeProvider{response: "VIP launch planning completed with the correct audience and channel details preserved."},
	}, bus)
	if err != nil {
		t.Fatalf("create memory: %v", err)
	}

	workflow := engine.Workflow{
		ID:     "wf_1",
		Name:   "VIP launch draft",
		Skill:  "mitto-sms-drafter",
		Status: engine.WorkflowCompleted,
		Input: map[string]any{
			"campaign_name": "VIP launch",
			"segment":       "loyalists",
			"channel":       "sms",
		},
		Output: map[string]any{
			"draft_id": "draft_1",
		},
	}

	if err := memory.RecordWorkflow(context.Background(), workflow); err != nil {
		t.Fatalf("record workflow: %v", err)
	}

	snapshot, err := memory.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snapshot.Recent) != 1 {
		t.Fatalf("expected one memory entry, got %d", len(snapshot.Recent))
	}
	if snapshot.Narrative == "" {
		t.Fatalf("expected narrative to be populated")
	}
	if snapshot.Variables["input.campaign_name"] != "VIP launch" {
		t.Fatalf("expected campaign variable to be retained")
	}

	select {
	case event := <-subscription.C:
		if event.Type != engine.EventContextFlush {
			t.Fatalf("unexpected event type %q", event.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for context flush event")
	}
}

func TestDynamicServiceFlushListenerResetsWindow(t *testing.T) {
	bus := engine.NewEventBus()
	service := NewDynamicService(stubSecrets{}, nil, bus)
	service.window.Add("user", "Analyze our April competitor campaign")

	if got := len(service.window.Snapshot()); got != 1 {
		t.Fatalf("expected one turn, got %d", got)
	}

	if err := bus.Publish(context.Background(), engine.Event{Type: engine.EventContextFlush}); err != nil {
		t.Fatalf("publish flush event: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		if len(service.window.Snapshot()) == 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("window was not reset after flush event")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestPersistentMemoryReadsLegacyJSONWhenCompactConfigured(t *testing.T) {
	root := t.TempDir()
	legacyPath := PersistentMemoryPath(root, persistence.FormatJSON)
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"narrative":"legacy","variables":{"input.campaign_name":"Legacy"},"recent":[{"workflow_id":"wf_legacy","skill":"utm-validator","status":"completed","summary":"Legacy summary"}],"last_flush":"2026-04-14T12:00:00Z"}`), 0o644); err != nil {
		t.Fatalf("write legacy memory: %v", err)
	}

	memory, err := NewPersistentMemoryWithOptions(root, nil, nil, persistence.FormatCompactV1)
	if err != nil {
		t.Fatalf("new memory: %v", err)
	}

	snapshot, err := memory.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Narrative != "legacy" {
		t.Fatalf("expected legacy narrative, got %q", snapshot.Narrative)
	}
	if got := snapshot.Variables["input.campaign_name"]; got != "Legacy" {
		t.Fatalf("expected legacy variable, got %q", got)
	}
}
