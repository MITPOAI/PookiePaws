package engine

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type SubTurnManagerConfig struct {
	MaxDepth           int
	MaxConcurrent      int
	ConcurrencyTimeout time.Duration
	DefaultTimeout     time.Duration
	Bus                EventBus
}

type managedSubTurn struct {
	id         string
	parentID   string
	name       string
	depth      int
	timeout    time.Duration
	ctx        context.Context
	cancel     context.CancelFunc
	startedAt  time.Time
	finishedAt *time.Time
	state      SubTurnState
	output     map[string]any
	err        error
	done       chan struct{}
}

type StandardSubTurnManager struct {
	mu                 sync.RWMutex
	tasks              map[string]*managedSubTurn
	children           map[string]map[string]struct{}
	bus                EventBus
	maxDepth           int
	defaultTimeout     time.Duration
	concurrencyTimeout time.Duration
	sem                chan struct{}
	nextID             uint64
	closed             atomic.Bool
}

func NewSubTurnManager(cfg SubTurnManagerConfig) *StandardSubTurnManager {
	maxConcurrent := cfg.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 4
	}
	maxDepth := cfg.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 3
	}
	defaultTimeout := cfg.DefaultTimeout
	if defaultTimeout <= 0 {
		defaultTimeout = 30 * time.Second
	}
	concurrencyTimeout := cfg.ConcurrencyTimeout
	if concurrencyTimeout <= 0 {
		concurrencyTimeout = 10 * time.Second
	}

	return &StandardSubTurnManager{
		tasks:              make(map[string]*managedSubTurn),
		children:           make(map[string]map[string]struct{}),
		bus:                cfg.Bus,
		maxDepth:           maxDepth,
		defaultTimeout:     defaultTimeout,
		concurrencyTimeout: concurrencyTimeout,
		sem:                make(chan struct{}, maxConcurrent),
	}
}

func (m *StandardSubTurnManager) Spawn(ctx context.Context, spec SubTurnSpec, runner SubTurnRunner) (string, error) {
	if m.closed.Load() {
		return "", ErrAlreadyClosed
	}
	if runner == nil {
		return "", fmt.Errorf("subturn runner is required")
	}
	if spec.Depth > m.maxDepth {
		return "", fmt.Errorf("subturn depth %d exceeds max depth %d", spec.Depth, m.maxDepth)
	}

	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = m.defaultTimeout
	}

	acquireCtx, cancelAcquire := context.WithTimeout(ctx, m.concurrencyTimeout)
	defer cancelAcquire()

	select {
	case m.sem <- struct{}{}:
	case <-acquireCtx.Done():
		return "", fmt.Errorf("acquire subturn slot: %w", acquireCtx.Err())
	}

	id := fmt.Sprintf("subturn_%d", atomic.AddUint64(&m.nextID, 1))
	taskCtx, cancel := context.WithTimeout(context.Background(), timeout)
	startedAt := time.Now().UTC()

	task := &managedSubTurn{
		id:        id,
		parentID:  spec.ParentID,
		name:      spec.Name,
		depth:     spec.Depth,
		timeout:   timeout,
		ctx:       taskCtx,
		cancel:    cancel,
		startedAt: startedAt,
		state:     SubTurnStateRunning,
		done:      make(chan struct{}),
	}

	m.mu.Lock()
	m.tasks[id] = task
	if spec.ParentID != "" {
		if _, ok := m.children[spec.ParentID]; !ok {
			m.children[spec.ParentID] = make(map[string]struct{})
		}
		m.children[spec.ParentID][id] = struct{}{}
	}
	m.mu.Unlock()

	m.publishEvent(Event{
		Type:   EventSubTurnStarted,
		Source: "subturn-manager",
		Payload: map[string]any{
			"id":        id,
			"parent_id": spec.ParentID,
			"name":      spec.Name,
			"depth":     spec.Depth,
		},
	})

	go func() {
		defer func() {
			<-m.sem
			close(task.done)
		}()

		output, err := runner(taskCtx)
		finishedAt := time.Now().UTC()

		m.mu.Lock()
		defer m.mu.Unlock()

		task.output = output
		task.err = err
		task.finishedAt = &finishedAt

		switch {
		case err == context.Canceled || err == context.DeadlineExceeded || taskCtx.Err() != nil:
			task.state = SubTurnStateCanceled
		case err != nil:
			task.state = SubTurnStateFailed
		default:
			task.state = SubTurnStateCompleted
		}

		if task.parentID != "" {
			if parent, ok := m.tasks[task.parentID]; !ok || parent.state == SubTurnStateCanceled || parent.state == SubTurnStateFailed {
				task.state = SubTurnStateOrphaned
				m.publishEvent(Event{
					Type:   EventSubTurnOrphaned,
					Source: "subturn-manager",
					Payload: map[string]any{
						"id":        task.id,
						"parent_id": task.parentID,
					},
				})
			}
		}

		m.publishEvent(Event{
			Type:   EventSubTurnCompleted,
			Source: "subturn-manager",
			Payload: map[string]any{
				"id":        task.id,
				"parent_id": task.parentID,
				"state":     task.state,
				"error":     errorString(task.err),
			},
		})
	}()

	return id, nil
}

func (m *StandardSubTurnManager) Wait(ctx context.Context, id string) (SubTurnResult, error) {
	m.mu.RLock()
	task, ok := m.tasks[id]
	m.mu.RUnlock()
	if !ok {
		return SubTurnResult{}, ErrNotFound
	}

	select {
	case <-ctx.Done():
		return SubTurnResult{}, ctx.Err()
	case <-task.done:
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	return SubTurnResult{
		ID:         task.id,
		ParentID:   task.parentID,
		Status:     task.state,
		Output:     task.output,
		Err:        errorString(task.err),
		StartedAt:  task.startedAt,
		FinishedAt: derefTime(task.finishedAt),
	}, nil
}

func (m *StandardSubTurnManager) Cancel(id string) error {
	m.mu.RLock()
	task, ok := m.tasks[id]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	task.cancel()
	return nil
}

func (m *StandardSubTurnManager) CancelChildren(parentID string) error {
	m.mu.RLock()
	children := m.children[parentID]
	m.mu.RUnlock()
	if len(children) == 0 {
		return nil
	}

	for childID := range children {
		if err := m.Cancel(childID); err != nil && err != ErrNotFound {
			return err
		}
	}
	return nil
}

func (m *StandardSubTurnManager) Snapshot() []SubTurnStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := make([]SubTurnStatus, 0, len(m.tasks))
	for _, task := range m.tasks {
		snapshot = append(snapshot, SubTurnStatus{
			ID:         task.id,
			ParentID:   task.parentID,
			Name:       task.name,
			Depth:      task.depth,
			State:      task.state,
			StartedAt:  task.startedAt,
			FinishedAt: task.finishedAt,
			Err:        errorString(task.err),
			Timeout:    task.timeout,
		})
	}
	sort.Slice(snapshot, func(i, j int) bool {
		return snapshot[i].StartedAt.Before(snapshot[j].StartedAt)
	})
	return snapshot
}

func (m *StandardSubTurnManager) Close() error {
	if m.closed.Swap(true) {
		return ErrAlreadyClosed
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, task := range m.tasks {
		task.cancel()
	}
	return nil
}

func (m *StandardSubTurnManager) publishEvent(event Event) {
	if m.bus == nil {
		return
	}
	_ = m.bus.Publish(event)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}
