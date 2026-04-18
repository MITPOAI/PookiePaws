# `pookie research` CLI + Research Scheduler Daemon Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a top-level `pookie research` command (watchlists/refresh/schedule/status/recommendations) and a daemon-only scheduler that automatically triggers `mitpo-watchlist-refresh` according to the configured `research_schedule` (`manual|hourly|daily`).

**Architecture:** A new `internal/scheduler` package owns the periodic ticker loop, schedule decision logic, and a small JSON state file (`state/research/scheduler.json`). The scheduler is wired into `cmdStart` only — it has zero effect on `pookie run`, `version`, or `doctor`. The CLI subcommand operates directly on local filesystem state via `dossier.Service` (the same code path the gateway uses), so it works whether or not the daemon is running. The `refresh` subcommand builds a minimal stack and submits the workflow locally, mirroring the `cmdRun` pattern.

**Tech Stack:** Go 1.22, standard library, existing `internal/dossier`, `internal/engine`, `internal/security`, `internal/state` packages.

---

## File Structure

| Path | Responsibility |
|------|----------------|
| `internal/scheduler/scheduler.go` | NEW — `ResearchScheduler` type, `Run(ctx)` ticker loop |
| `internal/scheduler/decide.go` | NEW — pure `Decide(now, schedule, lastRun) Decision` function (testable in isolation) |
| `internal/scheduler/state.go` | NEW — `State` type + atomic JSON load/save at `state/research/scheduler.json` |
| `internal/scheduler/scheduler_test.go` | NEW — end-to-end with fake coordinator + clock |
| `internal/scheduler/decide_test.go` | NEW — table-driven schedule decision tests |
| `internal/scheduler/state_test.go` | NEW — round-trip, missing/corrupt file |
| `cmd/pookie/research.go` | NEW — `cmdResearch(args)` dispatcher + subcommand handlers |
| `cmd/pookie/research_test.go` | NEW — argument parsing, output for each subcommand |
| `cmd/pookie/main.go` | MODIFY — add `case "research": cmdResearch(...)`; wire scheduler into `cmdStart` |
| `cmd/pookie/stack.go` | MODIFY — expose `dossier.Service` on `appStack` so `cmdStart` and `cmdResearch` can share it |
| `internal/dossier/service.go` | MODIFY — add `GetWatchlist(ctx, id)`, `DeleteWatchlist(ctx, id)`, and `MaxLastRunAt(ctx) (*time.Time, error)` (the scheduler needs these) |
| `internal/dossier/service_test.go` | MODIFY — tests for new methods |
| `internal/state/audit.go` | NO CHANGE — use existing `AppendAudit` |
| `internal/engine/types.go` | MODIFY — add `EventResearchScheduled`, `EventResearchScheduleSkipped` constants |

---

## Constants and Conventions

- The watchlist-refresh skill ID is `mitpo-watchlist-refresh` (from `internal/skills/defaults/mitpo-watchlist-refresh/SKILL.md`).
- The vault key for the schedule mode is `research_schedule`, accepted values `manual|hourly|daily` (already validated by the gateway at `internal/gateway/server.go:1380`).
- Hourly = 60 minutes, daily = 24 hours. The decision function uses these as the minimum gap between runs, anchored to the last successful run.
- The scheduler ticks every **60 seconds**. This is a fixed compromise between responsiveness on schedule changes and avoiding wakeup churn.
- The scheduler **never runs in `manual` mode** — it only logs that mode is manual and idles.
- A refresh is "already running" if `coordinator.ListWorkflows(ctx)` returns any workflow with `Skill == "mitpo-watchlist-refresh"` and a non-terminal status (`Queued|Running|WaitingApproval`).

---

## Phase A — `dossier.Service` extensions

### Task A1: Add `GetWatchlist`, `DeleteWatchlist`, `MaxLastRunAt`

**Files:**
- Modify: `internal/dossier/service.go`
- Modify: `internal/dossier/service_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/dossier/service_test.go`:

```go
func TestGetWatchlistFound(t *testing.T) {
	svc := newTestService(t)
	saved, _ := svc.SaveWatchlists(context.Background(), []Watchlist{{ID: "wl-1", Name: "alpha"}})
	got, err := svc.GetWatchlist(context.Background(), saved[0].ID)
	if err != nil {
		t.Fatalf("GetWatchlist: %v", err)
	}
	if got.Name != "alpha" {
		t.Errorf("Name = %q", got.Name)
	}
}

func TestGetWatchlistMissing(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.GetWatchlist(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing watchlist")
	}
}

func TestDeleteWatchlist(t *testing.T) {
	svc := newTestService(t)
	saved, _ := svc.SaveWatchlists(context.Background(), []Watchlist{{ID: "wl-1", Name: "alpha"}})
	if err := svc.DeleteWatchlist(context.Background(), saved[0].ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	all, _ := svc.ListWatchlists(context.Background())
	if len(all) != 0 {
		t.Fatalf("expected 0 watchlists after delete, got %d", len(all))
	}
}

func TestDeleteWatchlistMissingIsNoop(t *testing.T) {
	svc := newTestService(t)
	if err := svc.DeleteWatchlist(context.Background(), "nope"); err != nil {
		t.Fatalf("expected nil error for missing delete, got %v", err)
	}
}

func TestMaxLastRunAt(t *testing.T) {
	svc := newTestService(t)
	t1 := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	_, _ = svc.SaveWatchlists(context.Background(), []Watchlist{
		{ID: "a", Name: "a", LastRunAt: &t1},
		{ID: "b", Name: "b", LastRunAt: &t2},
		{ID: "c", Name: "c"}, // never run
	})
	got, err := svc.MaxLastRunAt(context.Background())
	if err != nil {
		t.Fatalf("MaxLastRunAt: %v", err)
	}
	if got == nil || !got.Equal(t2) {
		t.Fatalf("MaxLastRunAt = %v, want %v", got, t2)
	}
}

func TestMaxLastRunAtNoneRun(t *testing.T) {
	svc := newTestService(t)
	_, _ = svc.SaveWatchlists(context.Background(), []Watchlist{{ID: "a", Name: "a"}})
	got, err := svc.MaxLastRunAt(context.Background())
	if err != nil {
		t.Fatalf("MaxLastRunAt: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil when no watchlist has run, got %v", got)
	}
}

// newTestService builds a Service rooted in t.TempDir(). If a helper already
// exists in this file, delete this duplicate.
func newTestService(t *testing.T) *Service {
	t.Helper()
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}
```

If `newTestService` already exists in the file, remove the duplicate at the bottom of the snippet above.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/dossier/... -run 'TestGetWatchlist|TestDeleteWatchlist|TestMaxLastRunAt' -v`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Implement methods**

In `internal/dossier/service.go`, add after `ListWatchlists`:

```go
// GetWatchlist returns the watchlist with the given ID. Returns an error if
// not found — callers that want a "missing is fine" semantic should use
// errors.Is(err, ErrWatchlistNotFound) once we add it. For now, any error
// from the underlying read is propagated.
func (s *Service) GetWatchlist(_ context.Context, id string) (Watchlist, error) {
	if id == "" {
		return Watchlist{}, fmt.Errorf("watchlist id is required")
	}
	all, err := s.ListWatchlists(context.Background())
	if err != nil {
		return Watchlist{}, err
	}
	for _, wl := range all {
		if wl.ID == id {
			return wl, nil
		}
	}
	return Watchlist{}, fmt.Errorf("watchlist %q not found", id)
}

// DeleteWatchlist removes a watchlist by ID. Missing IDs are a no-op (idempotent).
func (s *Service) DeleteWatchlist(_ context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("watchlist id is required")
	}
	path := filepath.Join(s.watchlistsDir, id+".json")
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("delete watchlist: %w", err)
	}
	return nil
}

// MaxLastRunAt returns the most recent LastRunAt across all watchlists, or
// nil if no watchlist has been run. Used by the scheduler to decide whether
// a refresh is due.
func (s *Service) MaxLastRunAt(ctx context.Context) (*time.Time, error) {
	all, err := s.ListWatchlists(ctx)
	if err != nil {
		return nil, err
	}
	var max *time.Time
	for _, wl := range all {
		if wl.LastRunAt == nil {
			continue
		}
		if max == nil || wl.LastRunAt.After(*max) {
			t := *wl.LastRunAt
			max = &t
		}
	}
	return max, nil
}
```

If `s.watchlistsDir` is not the field name used in service.go, locate the actual watchlists directory field (search for `watchlists` in the `Service` struct definition near line 24) and adapt.

- [ ] **Step 4: Add `errors` import if missing**

Make sure `internal/dossier/service.go` imports `errors`. The file likely already has it; if not, add it.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/dossier/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/dossier/service.go internal/dossier/service_test.go
git commit -m "feat(dossier): add GetWatchlist, DeleteWatchlist, MaxLastRunAt"
```

---

## Phase B — Scheduler package

### Task B1: Decision function

**Files:**
- Create: `internal/scheduler/decide.go`
- Create: `internal/scheduler/decide_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/scheduler/decide_test.go`:

```go
package scheduler

import (
	"testing"
	"time"
)

func TestDecide(t *testing.T) {
	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	hourAgo := now.Add(-1 * time.Hour)
	thirtyMinAgo := now.Add(-30 * time.Minute)
	dayAgo := now.Add(-24 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)

	cases := []struct {
		name     string
		schedule string
		lastRun  *time.Time
		want     Decision
	}{
		{"manual never runs", "manual", &hourAgo, Decision{Run: false, Reason: "schedule is manual"}},
		{"hourly never run before", "hourly", nil, Decision{Run: true, Reason: "no prior run"}},
		{"hourly due exactly at boundary", "hourly", &hourAgo, Decision{Run: true, Reason: "hourly interval elapsed"}},
		{"hourly not yet due", "hourly", &thirtyMinAgo, Decision{Run: false, Reason: "next due in ~30m0s"}},
		{"daily never run", "daily", nil, Decision{Run: true, Reason: "no prior run"}},
		{"daily exactly at boundary", "daily", &dayAgo, Decision{Run: true, Reason: "daily interval elapsed"}},
		{"daily not yet due", "daily", &twoHoursAgo, Decision{Run: false, Reason: "next due in ~22h0m0s"}},
		{"unknown mode treated as manual", "weekly", &dayAgo, Decision{Run: false, Reason: `unknown schedule "weekly", treating as manual`}},
		{"empty mode treated as manual", "", &dayAgo, Decision{Run: false, Reason: "schedule is manual"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Decide(now, tc.schedule, tc.lastRun)
			if got.Run != tc.want.Run || got.Reason != tc.want.Reason {
				t.Fatalf("Decide = %+v, want %+v", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./internal/scheduler/... -run TestDecide -v`
Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Implement `decide.go`**

Create `internal/scheduler/decide.go`:

```go
// Package scheduler implements the research watchlist refresh ticker that
// runs inside the daemon (`pookie start`). It is intentionally not started
// by one-shot CLI commands.
package scheduler

import (
	"fmt"
	"time"
)

// Decision is the output of a single scheduler tick: should we run, and why?
// The Reason is surfaced in audit events and in `pookie research status`.
type Decision struct {
	Run    bool
	Reason string
}

// Schedule modes accepted by the scheduler. Anything else is treated as manual.
const (
	ModeManual = "manual"
	ModeHourly = "hourly"
	ModeDaily  = "daily"
)

// Decide is the pure scheduling rule. Given the current time, the configured
// schedule mode, and the timestamp of the last successful run (nil if never),
// it returns whether a run is due and a human-readable reason.
func Decide(now time.Time, schedule string, lastRun *time.Time) Decision {
	switch schedule {
	case ModeManual, "":
		return Decision{Run: false, Reason: "schedule is manual"}
	case ModeHourly:
		return decideInterval(now, lastRun, time.Hour, "hourly")
	case ModeDaily:
		return decideInterval(now, lastRun, 24*time.Hour, "daily")
	default:
		return Decision{Run: false, Reason: fmt.Sprintf("unknown schedule %q, treating as manual", schedule)}
	}
}

func decideInterval(now time.Time, lastRun *time.Time, interval time.Duration, label string) Decision {
	if lastRun == nil {
		return Decision{Run: true, Reason: "no prior run"}
	}
	gap := now.Sub(*lastRun)
	if gap >= interval {
		return Decision{Run: true, Reason: label + " interval elapsed"}
	}
	remaining := interval - gap
	return Decision{Run: false, Reason: fmt.Sprintf("next due in ~%s", remaining.Round(time.Minute))}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/scheduler/... -run TestDecide -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/decide.go internal/scheduler/decide_test.go
git commit -m "feat(scheduler): pure schedule decision function (manual/hourly/daily)"
```

---

### Task B2: Scheduler state file

**Files:**
- Create: `internal/scheduler/state.go`
- Create: `internal/scheduler/state_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/scheduler/state_test.go`:

```go
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
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.LastTickAt.Equal(want.LastTickAt) || got.LastError != want.LastError || got.LastWorkflow != want.LastWorkflow {
		t.Fatalf("got %+v, want %+v", got, want)
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
	_ = os.WriteFile(path, []byte("not json"), 0o600)
	store := NewStateStore(path)
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load on corrupt should not error, got %v", err)
	}
	if !got.LastTickAt.IsZero() {
		t.Errorf("expected zero state on corrupt, got %+v", got)
	}
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./internal/scheduler/... -run TestState -v`
Expected: FAIL.

- [ ] **Step 3: Implement `state.go`**

Create `internal/scheduler/state.go`:

```go
package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State captures the scheduler's externally observable state. Persisted as
// JSON so `pookie research status` and `pookie doctor` can read it without
// linking the scheduler package.
type State struct {
	LastTickAt    time.Time `json:"last_tick_at"`
	LastSuccessAt time.Time `json:"last_success_at"`
	LastError     string    `json:"last_error,omitempty"`
	LastWorkflow  string    `json:"last_workflow,omitempty"`
	Schedule      string    `json:"schedule,omitempty"`
	NextDueAt     time.Time `json:"next_due_at,omitempty"`
}

// StateStore persists and loads State at a fixed path.
type StateStore struct {
	path string
}

// NewStateStore constructs a StateStore writing to the given path.
func NewStateStore(path string) *StateStore {
	return &StateStore{path: path}
}

// DefaultStatePath returns the conventional location under runtimeRoot.
func DefaultStatePath(runtimeRoot string) string {
	return filepath.Join(runtimeRoot, "state", "research", "scheduler.json")
}

// Load returns the persisted State or a zero State if the file is missing
// or corrupt. Corrupt files are intentionally treated as missing — losing
// scheduler bookkeeping is preferable to crashing the daemon.
func (s *StateStore) Load() (State, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("read scheduler state: %w", err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, nil
	}
	return st, nil
}

// Save writes the State atomically.
func (s *StateStore) Save(st State) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("mkdir scheduler state: %w", err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write tmp state: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/scheduler/... -v`
Expected: PASS for decide + state tests.

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/state.go internal/scheduler/state_test.go
git commit -m "feat(scheduler): atomic JSON state file"
```

---

### Task B3: Scheduler ticker loop

**Files:**
- Create: `internal/scheduler/scheduler.go`
- Create: `internal/scheduler/scheduler_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/scheduler/scheduler_test.go`:

```go
package scheduler

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type fakeCoord struct {
	mu          sync.Mutex
	submitted   []string
	listResult  []engine.Workflow
	submitErr   error
	submitID    int
	submitDelay time.Duration
}

func (f *fakeCoord) SubmitWorkflow(ctx context.Context, def engine.WorkflowDefinition) (engine.Workflow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.submitErr != nil {
		return engine.Workflow{}, f.submitErr
	}
	f.submitID++
	id := "wf-fake-" + itoa(f.submitID)
	f.submitted = append(f.submitted, def.Skill)
	return engine.Workflow{ID: id, Skill: def.Skill, Status: engine.WorkflowQueued}, nil
}

func (f *fakeCoord) ListWorkflows(ctx context.Context) ([]engine.Workflow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]engine.Workflow, len(f.listResult))
	copy(out, f.listResult)
	return out, nil
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

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

func TestSchedulerSkipsManual(t *testing.T) {
	coord := &fakeCoord{}
	secrets := &fakeSecrets{values: map[string]string{"research_schedule": "manual"}}
	store := NewStateStore(filepath.Join(t.TempDir(), "s.json"))

	sch := New(Config{
		Coordinator:    coord,
		Secrets:        secrets,
		StateStore:     store,
		MaxLastRunAt:   func(ctx context.Context) (*time.Time, error) { return nil, nil },
		Now:            func() time.Time { return time.Now().UTC() },
		TickInterval:   10 * time.Millisecond,
		Logger:         testLogger(t),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	sch.Run(ctx)

	if len(coord.submitted) != 0 {
		t.Fatalf("manual mode should not submit, got %v", coord.submitted)
	}
}

func TestSchedulerSubmitsWhenDue(t *testing.T) {
	coord := &fakeCoord{}
	secrets := &fakeSecrets{values: map[string]string{"research_schedule": "hourly"}}
	store := NewStateStore(filepath.Join(t.TempDir(), "s.json"))

	sch := New(Config{
		Coordinator:    coord,
		Secrets:        secrets,
		StateStore:     store,
		MaxLastRunAt:   func(ctx context.Context) (*time.Time, error) { return nil, nil },
		Now:            func() time.Time { return time.Now().UTC() },
		TickInterval:   10 * time.Millisecond,
		Logger:         testLogger(t),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	sch.Run(ctx)

	if len(coord.submitted) == 0 {
		t.Fatal("expected at least one submit")
	}
	if coord.submitted[0] != "mitpo-watchlist-refresh" {
		t.Errorf("unexpected skill: %s", coord.submitted[0])
	}
}

func TestSchedulerSuppressesWhenAlreadyRunning(t *testing.T) {
	coord := &fakeCoord{
		listResult: []engine.Workflow{
			{ID: "in-flight", Skill: "mitpo-watchlist-refresh", Status: engine.WorkflowRunning},
		},
	}
	secrets := &fakeSecrets{values: map[string]string{"research_schedule": "hourly"}}
	store := NewStateStore(filepath.Join(t.TempDir(), "s.json"))

	sch := New(Config{
		Coordinator:    coord,
		Secrets:        secrets,
		StateStore:     store,
		MaxLastRunAt:   func(ctx context.Context) (*time.Time, error) { return nil, nil },
		Now:            func() time.Time { return time.Now().UTC() },
		TickInterval:   10 * time.Millisecond,
		Logger:         testLogger(t),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	sch.Run(ctx)

	if len(coord.submitted) != 0 {
		t.Fatalf("should not submit while one is running, got %v", coord.submitted)
	}
}

func TestSchedulerPersistsState(t *testing.T) {
	coord := &fakeCoord{}
	secrets := &fakeSecrets{values: map[string]string{"research_schedule": "hourly"}}
	store := NewStateStore(filepath.Join(t.TempDir(), "s.json"))

	var ticks atomic.Int32
	sch := New(Config{
		Coordinator:  coord,
		Secrets:      secrets,
		StateStore:   store,
		MaxLastRunAt: func(ctx context.Context) (*time.Time, error) { return nil, nil },
		Now:          func() time.Time { ticks.Add(1); return time.Now().UTC() },
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

func testLogger(t *testing.T) Logger {
	t.Helper()
	return func(level, msg string, kvs ...any) { t.Logf("[%s] %s %v", level, msg, kvs) }
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./internal/scheduler/... -run 'TestScheduler' -v`
Expected: FAIL — `New`, `Config`, `Logger` undefined.

- [ ] **Step 3: Implement `scheduler.go`**

Create `internal/scheduler/scheduler.go`:

```go
package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

// Coordinator is the subset of engine.WorkflowCoordinator the scheduler uses.
// Defining it as an interface here keeps scheduler tests free of the full
// coordinator construction.
type Coordinator interface {
	SubmitWorkflow(ctx context.Context, def engine.WorkflowDefinition) (engine.Workflow, error)
	ListWorkflows(ctx context.Context) ([]engine.Workflow, error)
}

// Secrets is the subset of engine.SecretProvider the scheduler uses.
type Secrets interface {
	Get(name string) (string, error)
}

// Logger is a minimal level/message/kv logger. Wire to slog or similar in main.
type Logger func(level, msg string, kvs ...any)

// Config configures a Scheduler.
type Config struct {
	Coordinator  Coordinator
	Secrets      Secrets
	StateStore   *StateStore
	MaxLastRunAt func(ctx context.Context) (*time.Time, error)
	Now          func() time.Time
	TickInterval time.Duration
	Logger       Logger
}

// Scheduler periodically checks whether a watchlist refresh is due and, if
// so, submits the `mitpo-watchlist-refresh` workflow.
type Scheduler struct {
	cfg Config
	mu  sync.Mutex
}

// SkillName is the skill the scheduler submits.
const SkillName = "mitpo-watchlist-refresh"

// DefaultTickInterval is the wake-up cadence.
const DefaultTickInterval = 60 * time.Second

// New builds a Scheduler with sane defaults filled in.
func New(cfg Config) *Scheduler {
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	if cfg.TickInterval == 0 {
		cfg.TickInterval = DefaultTickInterval
	}
	if cfg.Logger == nil {
		cfg.Logger = func(string, string, ...any) {}
	}
	return &Scheduler{cfg: cfg}
}

// Run blocks until ctx is cancelled, ticking at TickInterval. Each tick is
// independent: a tick failure (vault read, store save, submit error) is
// logged and recorded in state but does not crash the loop.
func (s *Scheduler) Run(ctx context.Context) {
	s.tick(ctx) // run once immediately so first tick isn't TickInterval away
	t := time.NewTicker(s.cfg.TickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.cfg.Now()
	st, _ := s.cfg.StateStore.Load()
	st.LastTickAt = now

	schedule, _ := s.cfg.Secrets.Get("research_schedule")
	st.Schedule = schedule

	lastRun, lrErr := s.cfg.MaxLastRunAt(ctx)
	if lrErr != nil {
		s.cfg.Logger("warn", "scheduler: read last run", "err", lrErr)
	}

	d := Decide(now, schedule, lastRun)

	// Compute next due for diagnostics regardless of decision.
	st.NextDueAt = nextDue(now, schedule, lastRun)

	if !d.Run {
		s.cfg.Logger("debug", "scheduler tick: skip", "reason", d.Reason)
		_ = s.cfg.StateStore.Save(st)
		return
	}

	running, err := s.refreshAlreadyRunning(ctx)
	if err != nil {
		s.cfg.Logger("warn", "scheduler: list workflows", "err", err)
	}
	if running {
		s.cfg.Logger("debug", "scheduler tick: refresh already in flight, skipping")
		_ = s.cfg.StateStore.Save(st)
		return
	}

	wf, err := s.cfg.Coordinator.SubmitWorkflow(ctx, engine.WorkflowDefinition{
		Name:  "Scheduled watchlist refresh",
		Skill: SkillName,
		Input: map[string]any{"trigger": "scheduler", "scheduled_at": now.Format(time.RFC3339)},
	})
	if err != nil {
		st.LastError = err.Error()
		s.cfg.Logger("error", "scheduler: submit failed", "err", err)
		_ = s.cfg.StateStore.Save(st)
		return
	}
	st.LastError = ""
	st.LastSuccessAt = now
	st.LastWorkflow = wf.ID
	s.cfg.Logger("info", "scheduler: submitted refresh", "workflow", wf.ID, "reason", d.Reason)
	_ = s.cfg.StateStore.Save(st)
}

func (s *Scheduler) refreshAlreadyRunning(ctx context.Context) (bool, error) {
	wfs, err := s.cfg.Coordinator.ListWorkflows(ctx)
	if err != nil {
		return false, err
	}
	for _, wf := range wfs {
		if wf.Skill != SkillName {
			continue
		}
		switch wf.Status {
		case engine.WorkflowQueued, engine.WorkflowRunning, engine.WorkflowWaitingApproval:
			return true, nil
		}
	}
	return false, nil
}

func nextDue(now time.Time, schedule string, lastRun *time.Time) time.Time {
	switch schedule {
	case ModeHourly:
		if lastRun == nil {
			return now
		}
		return lastRun.Add(time.Hour)
	case ModeDaily:
		if lastRun == nil {
			return now
		}
		return lastRun.Add(24 * time.Hour)
	default:
		return time.Time{}
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/scheduler/... -v`
Expected: PASS for all scheduler tests.

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/scheduler.go internal/scheduler/scheduler_test.go
git commit -m "feat(scheduler): research watchlist refresh ticker with in-flight suppression"
```

---

## Phase C — Wire scheduler into `cmdStart`

### Task C1: Expose `dossier.Service` on `appStack`

**Files:**
- Modify: `cmd/pookie/stack.go`

- [ ] **Step 1: Read current `appStack` definition**

Run: `grep -n "appStack\|buildStack" cmd/pookie/stack.go | head -40`

Locate the `appStack` struct definition. It currently lacks an explicit `dossier *dossier.Service` field — the gateway constructs its own. We want one shared instance the scheduler can also use.

- [ ] **Step 2: Add field**

Add to the `appStack` struct (locate via the previous step):

```go
	dossier *dossier.Service
```

(Add the import `"github.com/mitpoai/pookiepaws/internal/dossier"` at the top if missing.)

- [ ] **Step 3: Construct in `buildStack`**

In `buildStack`, after the runtime root is established and before returning the stack, add:

```go
	dossierSvc, err := dossier.NewService(runtimeRoot)
	if err != nil {
		return nil, fmt.Errorf("init dossier service: %w", err)
	}
```

Then assign on the stack:

```go
	stack.dossier = dossierSvc
```

- [ ] **Step 4: Update gateway construction to share the instance**

Find where the gateway's `dossier.Service` is currently built (search `dossier.NewService` in `cmd/pookie/main.go` and `cmd/pookie/stack.go`). Replace the duplicate construction with `stack.dossier`.

If the gateway config field is named differently, adapt. Goal: a single `*dossier.Service` per process.

- [ ] **Step 5: Verify build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add cmd/pookie/stack.go cmd/pookie/main.go
git commit -m "refactor(stack): share single dossier.Service across gateway and CLI"
```

---

### Task C2: Start scheduler in `cmdStart`

**Files:**
- Modify: `cmd/pookie/main.go`

- [ ] **Step 1: Locate the right spot**

In `cmdStart` (around line 141), the order is: build stack → build gateway → start HTTP server in goroutine → block on signal. Insert the scheduler launch **after** the stack is built and **before** `httpServer.ListenAndServe()` so the scheduler is alive while the daemon serves.

- [ ] **Step 2: Add scheduler launch**

Add this block after `buildStack` returns successfully (replace `stack` with the actual variable name you used):

```go
	schedCtx, cancelSched := context.WithCancel(context.Background())
	defer cancelSched()
	go func() {
		sched := scheduler.New(scheduler.Config{
			Coordinator:  stack.coord,
			Secrets:      stack.secrets,
			StateStore:   scheduler.NewStateStore(scheduler.DefaultStatePath(runtimeRoot)),
			MaxLastRunAt: stack.dossier.MaxLastRunAt,
			Logger: func(level, msg string, kvs ...any) {
				fmt.Fprintf(os.Stderr, "[scheduler:%s] %s %v\n", level, msg, kvs)
			},
		})
		sched.Run(schedCtx)
	}()
```

Add the import `"github.com/mitpoai/pookiepaws/internal/scheduler"`.

In the existing graceful-shutdown block (where `httpServer.Shutdown` is called), call `cancelSched()` **before** the HTTP shutdown so any in-flight scheduler tick can settle:

```go
	cancelSched()
```

(The `defer cancelSched()` already covers the cleanup path; this line just ensures the scheduler stops promptly on signal.)

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 4: Manual smoke**

Run: `go run ./cmd/pookie start --addr 127.0.0.1:18800 &`

Wait 2 seconds, then: `cat ~/.local/share/pookiepaws/state/research/scheduler.json` (or the OS-equivalent runtime root).

Expected: file exists with `last_tick_at` set to a recent timestamp and `schedule` reflecting whatever's in the vault.

Stop the daemon with `kill %1`.

- [ ] **Step 5: Commit**

```bash
git add cmd/pookie/main.go
git commit -m "feat(start): launch research scheduler goroutine inside daemon"
```

---

## Phase D — `pookie research` CLI

### Task D1: Subcommand router and `watchlists list`

**Files:**
- Create: `cmd/pookie/research.go`
- Create: `cmd/pookie/research_test.go`
- Modify: `cmd/pookie/main.go` (add `case "research"`)

- [ ] **Step 1: Write failing tests**

Create `cmd/pookie/research_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/dossier"
)

func newDossierAt(t *testing.T) (*dossier.Service, string) {
	t.Helper()
	root := t.TempDir()
	svc, err := dossier.NewService(root)
	if err != nil {
		t.Fatalf("dossier.NewService: %v", err)
	}
	return svc, root
}

func TestResearchWatchlistsListEmpty(t *testing.T) {
	svc, _ := newDossierAt(t)
	var out bytes.Buffer
	err := runResearchWatchlistsList(context.Background(), svc, &out)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out.String(), "no watchlists") {
		t.Errorf("output: %q", out.String())
	}
}

func TestResearchWatchlistsListPopulated(t *testing.T) {
	svc, _ := newDossierAt(t)
	now := time.Now().UTC()
	_, _ = svc.SaveWatchlists(context.Background(), []dossier.Watchlist{
		{ID: "wl-1", Name: "alpha", Topic: "AI", LastRunAt: &now},
		{ID: "wl-2", Name: "beta", Topic: "Biz"},
	})
	var out bytes.Buffer
	err := runResearchWatchlistsList(context.Background(), svc, &out)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "alpha") || !strings.Contains(got, "beta") {
		t.Errorf("output missing watchlists: %q", got)
	}
}

func TestResearchWatchlistsApplyFromFile(t *testing.T) {
	svc, _ := newDossierAt(t)
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "wl.json")
	payload := `[{"id":"wl-1","name":"alpha","topic":"AI"}]`
	if err := writeFile(jsonPath, payload); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := runResearchWatchlistsApply(context.Background(), svc, jsonPath, nil, &out)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	all, _ := svc.ListWatchlists(context.Background())
	if len(all) != 1 || all[0].Name != "alpha" {
		t.Fatalf("watchlists: %+v", all)
	}
}

func TestResearchWatchlistsApplyFromStdin(t *testing.T) {
	svc, _ := newDossierAt(t)
	stdin := strings.NewReader(`[{"id":"wl-2","name":"beta","topic":"Biz"}]`)
	var out bytes.Buffer
	err := runResearchWatchlistsApply(context.Background(), svc, "", stdin, &out)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	all, _ := svc.ListWatchlists(context.Background())
	if len(all) != 1 || all[0].Name != "beta" {
		t.Fatalf("watchlists: %+v", all)
	}
}

func TestResearchWatchlistsApplyInvalidJSON(t *testing.T) {
	svc, _ := newDossierAt(t)
	err := runResearchWatchlistsApply(context.Background(), svc, "", strings.NewReader("not json"), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func writeFile(path, body string) error {
	return osWriteFile(path, []byte(body), 0o600)
}
```

(Add an `osWriteFile` shim in the same test file or replace with `os.WriteFile`. Use whichever the existing tests in this package use.)

- [ ] **Step 2: Verify it fails**

Run: `go test ./cmd/pookie/... -run TestResearchWatchlists -v`
Expected: FAIL — functions undefined.

- [ ] **Step 3: Implement `research.go`**

Create `cmd/pookie/research.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/mitpoai/pookiepaws/internal/dossier"
	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/scheduler"
)

func cmdResearch(args []string) {
	if len(args) == 0 {
		printResearchUsage(os.Stderr)
		os.Exit(2)
	}
	switch args[0] {
	case "watchlists":
		cmdResearchWatchlists(args[1:])
	case "refresh":
		cmdResearchRefresh(args[1:])
	case "schedule":
		cmdResearchSchedule(args[1:])
	case "status":
		cmdResearchStatus(args[1:])
	case "recommendations":
		cmdResearchRecommendations(args[1:])
	case "help", "--help", "-h":
		printResearchUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown research subcommand: %s\n", args[0])
		printResearchUsage(os.Stderr)
		os.Exit(2)
	}
}

func printResearchUsage(w io.Writer) {
	fmt.Fprint(w, `pookie research <subcommand>

  watchlists list                 Print configured watchlists
  watchlists apply --file <json>  Replace watchlists from JSON file (or --stdin)
  refresh                         Submit a watchlist refresh workflow now
  schedule --mode <m>             Set research schedule (manual|hourly|daily)
  status                          Show scheduler state
  recommendations [--status s]    List recommendations (draft|queued|submitted|discarded)
`)
}

// --- watchlists subcommand ---

func cmdResearchWatchlists(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: pookie research watchlists <list|apply>")
		os.Exit(2)
	}
	svc := mustDossierService()
	switch args[0] {
	case "list":
		if err := runResearchWatchlistsList(context.Background(), svc, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "list: %v\n", err)
			os.Exit(1)
		}
	case "apply":
		fs := flag.NewFlagSet("apply", flag.ExitOnError)
		file := fs.String("file", "", "JSON file containing a watchlist array")
		stdin := fs.Bool("stdin", false, "Read watchlists from stdin")
		_ = fs.Parse(args[1:])
		var input io.Reader
		if *stdin {
			input = os.Stdin
		}
		if err := runResearchWatchlistsApply(context.Background(), svc, *file, input, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "apply: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: pookie research watchlists <list|apply>")
		os.Exit(2)
	}
}

func runResearchWatchlistsList(ctx context.Context, svc *dossier.Service, out io.Writer) error {
	all, err := svc.ListWatchlists(ctx)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		fmt.Fprintln(out, "no watchlists configured")
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tTOPIC\tLAST RUN")
	for _, wl := range all {
		last := "-"
		if wl.LastRunAt != nil {
			last = wl.LastRunAt.Format(time.RFC3339)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", wl.ID, wl.Name, wl.Topic, last)
	}
	return tw.Flush()
}

func runResearchWatchlistsApply(ctx context.Context, svc *dossier.Service, file string, stdin io.Reader, out io.Writer) error {
	var data []byte
	var err error
	switch {
	case file != "":
		data, err = os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
	case stdin != nil:
		data, err = io.ReadAll(stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
	default:
		return fmt.Errorf("either --file or --stdin is required")
	}
	var watchlists []dossier.Watchlist
	if err := json.Unmarshal(data, &watchlists); err != nil {
		return fmt.Errorf("parse json: %w", err)
	}
	saved, err := svc.SaveWatchlists(ctx, watchlists)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}
	fmt.Fprintf(out, "applied %d watchlist(s)\n", len(saved))
	return nil
}

// --- refresh ---

func cmdResearchRefresh(args []string) {
	fs := flag.NewFlagSet("refresh", flag.ExitOnError)
	_ = fs.Parse(args)

	stack, err := buildStack(resolveRuntimeRoot(), resolveWorkspaceRoot())
	if err != nil {
		fmt.Fprintf(os.Stderr, "build stack: %v\n", err)
		os.Exit(1)
	}
	defer stack.Close()

	wf, err := stack.coord.SubmitWorkflow(context.Background(), engine.WorkflowDefinition{
		Name:  "Manual watchlist refresh",
		Skill: scheduler.SkillName,
		Input: map[string]any{"trigger": "cli"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "submit: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("submitted workflow %s\n", wf.ID)
}

// --- schedule ---

func cmdResearchSchedule(args []string) {
	fs := flag.NewFlagSet("schedule", flag.ExitOnError)
	mode := fs.String("mode", "", "Schedule mode (manual|hourly|daily)")
	_ = fs.Parse(args)

	switch *mode {
	case "manual", "hourly", "daily":
	case "":
		fmt.Fprintln(os.Stderr, "--mode is required (manual|hourly|daily)")
		os.Exit(2)
	default:
		fmt.Fprintf(os.Stderr, "invalid mode %q; use manual|hourly|daily\n", *mode)
		os.Exit(2)
	}

	if err := writeVaultSecret("research_schedule", *mode); err != nil {
		fmt.Fprintf(os.Stderr, "write secret: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("research_schedule = %s\n", *mode)
}

// --- status ---

func cmdResearchStatus(args []string) {
	_ = args
	store := scheduler.NewStateStore(scheduler.DefaultStatePath(resolveRuntimeRoot()))
	st, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load state: %v\n", err)
		os.Exit(1)
	}
	svc := mustDossierService()
	wls, _ := svc.ListWatchlists(context.Background())

	fmt.Printf("schedule:        %s\n", emptyDash(st.Schedule))
	fmt.Printf("watchlists:      %d\n", len(wls))
	fmt.Printf("last tick:       %s\n", formatTime(st.LastTickAt))
	fmt.Printf("last success:    %s\n", formatTime(st.LastSuccessAt))
	fmt.Printf("last workflow:   %s\n", emptyDash(st.LastWorkflow))
	fmt.Printf("next due:        %s\n", formatTime(st.NextDueAt))
	fmt.Printf("last error:      %s\n", emptyDash(st.LastError))
}

// --- recommendations ---

func cmdResearchRecommendations(args []string) {
	fs := flag.NewFlagSet("recommendations", flag.ExitOnError)
	status := fs.String("status", "", "Filter by status (draft|queued|submitted|discarded)")
	_ = fs.Parse(args)

	svc := mustDossierService()
	recs, err := svc.ListRecommendations(context.Background(), dossier.RecommendationStatus(*status), 100)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list: %v\n", err)
		os.Exit(1)
	}
	if len(recs) == 0 {
		fmt.Println("no recommendations")
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tDOSSIER\tSTATUS\tTITLE")
	for _, r := range recs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ID, r.DossierID, r.Status, r.Title)
	}
	_ = tw.Flush()
}

// --- shared helpers ---

func mustDossierService() *dossier.Service {
	svc, err := dossier.NewService(resolveRuntimeRoot())
	if err != nil {
		fmt.Fprintf(os.Stderr, "init dossier service: %v\n", err)
		os.Exit(1)
	}
	return svc
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
```

This file references three helpers that may not yet exist in the package: `resolveRuntimeRoot`, `resolveWorkspaceRoot`, and `writeVaultSecret`. Implement them in step 4 if missing.

- [ ] **Step 4: Add missing helpers**

Search: `grep -n "func resolveRoots\|func resolveRuntimeRoot\|func writeVaultSecret" cmd/pookie/*.go`

If `resolveRoots` exists but the single-result helpers do not, add to `cmd/pookie/stack.go`:

```go
func resolveRuntimeRoot() string {
	rt, _ := resolveRoots(currentHome())
	return rt
}

func resolveWorkspaceRoot() string {
	_, ws := resolveRoots(currentHome())
	return ws
}

func currentHome() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}
```

If `writeVaultSecret` doesn't exist, add to `cmd/pookie/stack.go`:

```go
// writeVaultSecret updates a single secret in the on-disk JSON vault.
// Reads-modifies-writes; preserves all other secrets.
func writeVaultSecret(key, value string) error {
	root := resolveRuntimeRoot()
	provider, err := security.NewJSONSecretProvider(root)
	if err != nil {
		return err
	}
	current := provider.AllSecrets() // assumes such a method; if not, see fallback below
	current[key] = value
	return provider.SaveAll(current)
}
```

If `JSONSecretProvider` doesn't expose `AllSecrets` / `SaveAll`, the fallback is to manipulate the JSON file directly:

```go
func writeVaultSecret(key, value string) error {
	path := filepath.Join(resolveRuntimeRoot(), "security.json")
	var current map[string]string
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &current)
	}
	if current == nil {
		current = map[string]string{}
	}
	current[key] = value
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
```

Pick whichever matches the actual `JSONSecretProvider` API after reading `internal/security/secrets.go`.

- [ ] **Step 5: Wire `research` into the main switch**

In `cmd/pookie/main.go`, add a case to the `switch os.Args[1]` block, alongside the other commands:

```go
		case "research":
			cmdResearch(os.Args[2:])
```

Also add `"research"` to `printUsage` if it lists commands.

- [ ] **Step 6: Verify build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 7: Run tests**

Run: `go test ./cmd/pookie/... -v -run TestResearch`
Expected: PASS for the watchlists list/apply cases.

- [ ] **Step 8: Manual smoke**

```bash
go run ./cmd/pookie research watchlists list
go run ./cmd/pookie research schedule --mode hourly
go run ./cmd/pookie research status
```

Expected: each prints structured output, no panics.

- [ ] **Step 9: Commit**

```bash
git add cmd/pookie/research.go cmd/pookie/research_test.go cmd/pookie/main.go cmd/pookie/stack.go
git commit -m "feat(cli): add 'pookie research' subcommand surface"
```

---

## Phase E — Diagnostics

### Task E1: Surface scheduler state in `doctor` and `/api/v1/status`

**Files:**
- Modify: the `cmdDoctor` handler (search `func cmdDoctor` in `cmd/pookie/`)
- Modify: `internal/gateway/server.go` — extend the `/api/v1/status` payload struct
- Modify: `internal/gateway/server_test.go` — assert new fields are present

- [ ] **Step 1: Extend doctor**

In `cmdDoctor`, before printing summary, load the scheduler state:

```go
	st, _ := scheduler.NewStateStore(scheduler.DefaultStatePath(resolveRuntimeRoot())).Load()
	fmt.Printf("research scheduler\n")
	fmt.Printf("  schedule:     %s\n", emptyDash(st.Schedule))
	fmt.Printf("  last tick:    %s\n", formatTime(st.LastTickAt))
	fmt.Printf("  next due:     %s\n", formatTime(st.NextDueAt))
	fmt.Printf("  last error:   %s\n", emptyDash(st.LastError))
```

Add the import if missing.

- [ ] **Step 2: Extend `/api/v1/status` payload**

In `internal/gateway/server.go`, locate the status response struct (search for the handler attached to `/api/v1/status`). Add a nested field:

```go
type schedulerStatus struct {
	Schedule      string    `json:"schedule"`
	LastTickAt    time.Time `json:"last_tick_at,omitempty"`
	LastSuccessAt time.Time `json:"last_success_at,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	NextDueAt     time.Time `json:"next_due_at,omitempty"`
}
```

Inject the runtime root into the gateway `Config` if not already (search `gateway.Config`); then in the status handler, load the state from `scheduler.DefaultStatePath(s.cfg.RuntimeRoot)`.

- [ ] **Step 3: Add a test for the new payload field**

In `internal/gateway/server_test.go`, add a test that pre-seeds a `scheduler.json` file under the test runtime root and asserts the `/api/v1/status` JSON contains the `scheduler` object.

```go
func TestStatusIncludesScheduler(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state", "research", "scheduler.json")
	_ = os.MkdirAll(filepath.Dir(statePath), 0o755)
	_ = os.WriteFile(statePath, []byte(`{"schedule":"hourly","last_tick_at":"2026-04-18T12:00:00Z"}`), 0o600)

	srv := newTestGateway(t, root) // helper that mirrors existing test setup
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status: %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), `"schedule":"hourly"`) {
		t.Fatalf("body missing scheduler: %s", resp.Body.String())
	}
}
```

If `newTestGateway` doesn't exist, model it on the existing `gateway/server_test.go` setup helpers — pass a runtime root parameter.

- [ ] **Step 4: Run all gateway tests**

Run: `go test ./internal/gateway/...`
Expected: all PASS, including the new test.

- [ ] **Step 5: Commit**

```bash
git add cmd/pookie/main.go internal/gateway/server.go internal/gateway/server_test.go
git commit -m "feat(diagnostics): expose research scheduler state in doctor and /api/v1/status"
```

---

## Phase F — Documentation

### Task F1: Update CHANGELOG and README sections

- [ ] **Step 1: Update CHANGELOG**

Append to the `[Unreleased]` section:

```markdown
### Added
- `pookie research` subcommand: `watchlists list|apply`, `refresh`, `schedule`,
  `status`, `recommendations`.
- Daemon-only research scheduler that triggers `mitpo-watchlist-refresh`
  according to `research_schedule` (`manual|hourly|daily`).
- `pookie doctor` and `/api/v1/status` now surface scheduler state.

### Changed
- `dossier.Service` gained `GetWatchlist`, `DeleteWatchlist`, and
  `MaxLastRunAt`.
```

- [ ] **Step 2: Update README**

Add a new section after the existing CLI overview:

```markdown
## Research automation

Pookie can periodically refresh your watchlists and surface dossier diffs.

    pookie research schedule --mode hourly
    pookie research watchlists apply --file watchlists.json
    pookie research status

The scheduler runs *only* inside `pookie start` — one-shot commands
(`run`, `version`, `doctor`) never trigger it. Schedule modes:
`manual` (default), `hourly`, `daily`.

State lives at `<runtime-root>/state/research/scheduler.json`.
```

- [ ] **Step 3: Final test pass**

Run: `go test ./...`
Expected: all green.

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md README.md
git commit -m "docs: research CLI and scheduler"
```

---

## Verification Summary

- `go test ./...` green across `internal/scheduler`, `internal/dossier`, `cmd/pookie`, `internal/gateway`.
- `go build ./...` clean.
- `pookie research watchlists list` works on an empty store and a populated one.
- `pookie research watchlists apply --file <json>` and `--stdin` both persist via `dossier.Service`.
- `pookie research schedule --mode hourly` writes to the vault; subsequent `pookie research status` shows `schedule: hourly`.
- `pookie research refresh` submits a workflow and prints its ID.
- `pookie start` launches the scheduler; with `research_schedule=hourly` and no recent run, a refresh workflow is submitted within 60 s.
- A second tick while the first refresh is still `Running` does NOT submit a duplicate.
- `pookie doctor` prints scheduler state.
- `/api/v1/status` JSON includes a `scheduler` object.
- `pookie run`, `pookie version`, `pookie status` (against a non-running daemon) do NOT trigger the scheduler.

## Out of scope (deferred)

- Cron-syntax schedules (only manual/hourly/daily this pass).
- Per-watchlist schedules (single global schedule for now).
- UI changes (Plan 3).
- Removal of the `research_watchlists` vault key as the editable source of truth (Plan 3 handles the migration).
