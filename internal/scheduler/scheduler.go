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
// so, submits the mitpo-watchlist-refresh workflow.
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
