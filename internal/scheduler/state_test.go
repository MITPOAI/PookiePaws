package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStateStore(filepath.Join(dir, "scheduler.json"))
	now := time.Now().UTC().Truncate(time.Second)
	want := State{
		LastTickAt:    now,
		LastSuccessAt: now,
		LastError:     "boom",
		LastWorkflow:  "wf-123",
		Schedule:      "hourly",
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.LastTickAt.Equal(want.LastTickAt) {
		t.Errorf("LastTickAt = %v, want %v", got.LastTickAt, want.LastTickAt)
	}
	if got.LastError != want.LastError {
		t.Errorf("LastError = %q", got.LastError)
	}
	if got.LastWorkflow != want.LastWorkflow {
		t.Errorf("LastWorkflow = %q", got.LastWorkflow)
	}
	if got.Schedule != want.Schedule {
		t.Errorf("Schedule = %q", got.Schedule)
	}
}

func TestStateLoadMissingReturnsZero(t *testing.T) {
	store := NewStateStore(filepath.Join(t.TempDir(), "missing.json"))
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.LastTickAt.IsZero() {
		t.Errorf("expected zero state, got %+v", got)
	}
}

func TestStateLoadCorruptReturnsZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scheduler.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStateStore(path)
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load on corrupt should not error, got %v", err)
	}
	if !got.LastTickAt.IsZero() {
		t.Errorf("expected zero state on corrupt, got %+v", got)
	}
}

func TestStateSaveCreatesParentDirs(t *testing.T) {
	nested := filepath.Join(t.TempDir(), "a", "b", "c", "scheduler.json")
	store := NewStateStore(nested)
	if err := store.Save(State{Schedule: "hourly"}); err != nil {
		t.Fatalf("Save into nested path: %v", err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("expected file at %s, got %v", nested, err)
	}
}

func TestDefaultStatePath(t *testing.T) {
	got := DefaultStatePath("/tmp/runtime")
	want := filepath.Join("/tmp/runtime", "state", "research", "scheduler.json")
	if got != want {
		t.Errorf("DefaultStatePath = %q, want %q", got, want)
	}
}
