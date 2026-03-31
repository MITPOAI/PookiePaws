package engine

import (
	"context"
	"testing"
	"time"
)

func TestSubTurnManagerLifecycle(t *testing.T) {
	manager := NewSubTurnManager(SubTurnManagerConfig{
		MaxDepth:           2,
		MaxConcurrent:      1,
		ConcurrencyTimeout: time.Second,
		DefaultTimeout:     time.Second,
		Bus:                NewEventBus(),
	})
	defer manager.Close()

	id, err := manager.Spawn(context.Background(), SubTurnSpec{
		Name:    "test",
		Depth:   1,
		Timeout: time.Second,
	}, func(context.Context) (map[string]any, error) {
		return map[string]any{"ok": true}, nil
	})
	if err != nil {
		t.Fatalf("spawn failed: %v", err)
	}

	result, err := manager.Wait(context.Background(), id)
	if err != nil {
		t.Fatalf("wait failed: %v", err)
	}
	if result.Status != SubTurnStateCompleted {
		t.Fatalf("unexpected result status %q", result.Status)
	}
}

func TestSubTurnManagerDepthLimit(t *testing.T) {
	manager := NewSubTurnManager(SubTurnManagerConfig{
		MaxDepth:           1,
		MaxConcurrent:      1,
		ConcurrencyTimeout: time.Second,
		DefaultTimeout:     time.Second,
	})
	defer manager.Close()

	if _, err := manager.Spawn(context.Background(), SubTurnSpec{Name: "too-deep", Depth: 2}, func(context.Context) (map[string]any, error) {
		return nil, nil
	}); err == nil {
		t.Fatalf("expected depth limit error")
	}
}
