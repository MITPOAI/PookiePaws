package scheduler

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type fakeCoord struct {
	mu         sync.Mutex
	submitted  []string
	listResult []engine.Workflow
	submitErr  error
	submitID   int
}

func (f *fakeCoord) SubmitWorkflow(_ context.Context, def engine.WorkflowDefinition) (engine.Workflow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.submitErr != nil {
		return engine.Workflow{}, f.submitErr
	}
	f.submitID++
	id := fmt.Sprintf("wf-fake-%d", f.submitID)
	f.submitted = append(f.submitted, def.Skill)
	return engine.Workflow{ID: id, Skill: def.Skill, Status: engine.WorkflowQueued}, nil
}

func (f *fakeCoord) ListWorkflows(_ context.Context) ([]engine.Workflow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]engine.Workflow, len(f.listResult))
	copy(out, f.listResult)
	return out, nil
}

func (f *fakeCoord) ListWorkflowsByStatus(_ context.Context, statuses ...engine.WorkflowStatus) ([]engine.Workflow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(statuses) == 0 {
		out := make([]engine.Workflow, len(f.listResult))
		copy(out, f.listResult)
		return out, nil
	}
	set := make(map[engine.WorkflowStatus]struct{}, len(statuses))
	for _, s := range statuses {
		set[s] = struct{}{}
	}
	out := make([]engine.Workflow, 0, len(f.listResult))
	for _, wf := range f.listResult {
		if _, ok := set[wf.Status]; ok {
			out = append(out, wf)
		}
	}
	return out, nil
}

func (f *fakeCoord) submittedSkills() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.submitted))
	copy(out, f.submitted)
	return out
}

type fakeSecrets struct {
	values map[string]string
}

func (f *fakeSecrets) Get(name string) (string, error) {
	v, ok := f.values[name]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func testLogger(t *testing.T) Logger {
	t.Helper()
	return func(level, msg string, kvs ...any) { t.Logf("[%s] %s %v", level, msg, kvs) }
}

func TestSchedulerSkipsManual(t *testing.T) {
	coord := &fakeCoord{}
	secrets := &fakeSecrets{values: map[string]string{"research_schedule": "manual"}}
	store := NewStateStore(filepath.Join(t.TempDir(), "s.json"))

	sch := New(Config{
		Coordinator:  coord,
		Secrets:      secrets,
		StateStore:   store,
		MaxLastRunAt: func(_ context.Context) (*time.Time, error) { return nil, nil },
		Now:          func() time.Time { return time.Now().UTC() },
		TickInterval: 10 * time.Millisecond,
		Logger:       testLogger(t),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	sch.Run(ctx)

	if got := coord.submittedSkills(); len(got) != 0 {
		t.Fatalf("manual mode should not submit, got %v", got)
	}
}

func TestSchedulerSubmitsWhenDue(t *testing.T) {
	coord := &fakeCoord{}
	secrets := &fakeSecrets{values: map[string]string{"research_schedule": "hourly"}}
	store := NewStateStore(filepath.Join(t.TempDir(), "s.json"))

	sch := New(Config{
		Coordinator:  coord,
		Secrets:      secrets,
		StateStore:   store,
		MaxLastRunAt: func(_ context.Context) (*time.Time, error) { return nil, nil },
		Now:          func() time.Time { return time.Now().UTC() },
		TickInterval: 10 * time.Millisecond,
		Logger:       testLogger(t),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	sch.Run(ctx)

	got := coord.submittedSkills()
	if len(got) == 0 {
		t.Fatal("expected at least one submit")
	}
	if got[0] != SkillName {
		t.Errorf("unexpected skill: %s", got[0])
	}
}

func TestSchedulerSuppressesWhenAlreadyRunning(t *testing.T) {
	coord := &fakeCoord{
		listResult: []engine.Workflow{
			{ID: "in-flight", Skill: SkillName, Status: engine.WorkflowRunning},
		},
	}
	secrets := &fakeSecrets{values: map[string]string{"research_schedule": "hourly"}}
	store := NewStateStore(filepath.Join(t.TempDir(), "s.json"))

	sch := New(Config{
		Coordinator:  coord,
		Secrets:      secrets,
		StateStore:   store,
		MaxLastRunAt: func(_ context.Context) (*time.Time, error) { return nil, nil },
		Now:          func() time.Time { return time.Now().UTC() },
		TickInterval: 10 * time.Millisecond,
		Logger:       testLogger(t),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	sch.Run(ctx)

	if got := coord.submittedSkills(); len(got) != 0 {
		t.Fatalf("should not submit while one is running, got %v", got)
	}
}

func TestSchedulerPersistsState(t *testing.T) {
	coord := &fakeCoord{}
	secrets := &fakeSecrets{values: map[string]string{"research_schedule": "hourly"}}
	store := NewStateStore(filepath.Join(t.TempDir(), "s.json"))

	sch := New(Config{
		Coordinator:  coord,
		Secrets:      secrets,
		StateStore:   store,
		MaxLastRunAt: func(_ context.Context) (*time.Time, error) { return nil, nil },
		Now:          func() time.Time { return time.Now().UTC() },
		TickInterval: 10 * time.Millisecond,
		Logger:       testLogger(t),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	sch.Run(ctx)

	st, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if st.LastTickAt.IsZero() {
		t.Fatal("expected LastTickAt to be set")
	}
	if st.Schedule != "hourly" {
		t.Errorf("Schedule = %q", st.Schedule)
	}
}

func TestSchedulerSubmitErrorRecorded(t *testing.T) {
	coord := &fakeCoord{submitErr: errors.New("boom")}
	secrets := &fakeSecrets{values: map[string]string{"research_schedule": "hourly"}}
	store := NewStateStore(filepath.Join(t.TempDir(), "s.json"))

	sch := New(Config{
		Coordinator:  coord,
		Secrets:      secrets,
		StateStore:   store,
		MaxLastRunAt: func(_ context.Context) (*time.Time, error) { return nil, nil },
		Now:          func() time.Time { return time.Now().UTC() },
		TickInterval: 10 * time.Millisecond,
		Logger:       testLogger(t),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	sch.Run(ctx)

	st, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if st.LastError == "" {
		t.Fatal("expected LastError to be recorded after submit failure")
	}
}
