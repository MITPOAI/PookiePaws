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
// JSON so pookie research status and pookie doctor can read it without
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

// Save writes the State atomically. Uses a PID-suffixed tmp file so two
// processes writing concurrently do not collide on the rename source.
func (s *StateStore) Save(st State) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("mkdir scheduler state: %w", err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := fmt.Sprintf("%s.%d.tmp", s.path, os.Getpid())
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write tmp state: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}
