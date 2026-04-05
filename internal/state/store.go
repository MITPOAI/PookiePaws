package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type FileStore struct {
	root string
	mu   sync.Mutex
}

var _ engine.StateStore = (*FileStore)(nil)

func NewFileStore(root string) (*FileStore, error) {
	store := &FileStore{root: root}
	for _, dir := range []string{
		root,
		filepath.Join(root, "workflows"),
		filepath.Join(root, "approvals"),
		filepath.Join(root, "filepermissions"),
		filepath.Join(root, "messages"),
		filepath.Join(root, "sessions"),
		filepath.Join(root, "runtime"),
		filepath.Join(root, "audits"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (s *FileStore) SaveWorkflow(_ context.Context, workflow engine.Workflow) error {
	return s.writeJSON(filepath.Join(s.root, "workflows", workflow.ID+".json"), workflow)
}

func (s *FileStore) GetWorkflow(_ context.Context, id string) (engine.Workflow, error) {
	var workflow engine.Workflow
	err := s.readJSON(filepath.Join(s.root, "workflows", id+".json"), &workflow)
	if errors.Is(err, fs.ErrNotExist) {
		return engine.Workflow{}, engine.ErrNotFound
	}
	return workflow, err
}

func (s *FileStore) ListWorkflows(_ context.Context) ([]engine.Workflow, error) {
	var workflows []engine.Workflow
	if err := s.readDirJSON(filepath.Join(s.root, "workflows"), &workflows); err != nil {
		return nil, err
	}
	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].CreatedAt.After(workflows[j].CreatedAt)
	})
	return workflows, nil
}

func (s *FileStore) SaveApproval(_ context.Context, approval engine.Approval) error {
	return s.writeJSON(filepath.Join(s.root, "approvals", approval.ID+".json"), approval)
}

func (s *FileStore) GetApproval(_ context.Context, id string) (engine.Approval, error) {
	var approval engine.Approval
	err := s.readJSON(filepath.Join(s.root, "approvals", id+".json"), &approval)
	if errors.Is(err, fs.ErrNotExist) {
		return engine.Approval{}, engine.ErrNotFound
	}
	return approval, err
}

func (s *FileStore) ListApprovals(_ context.Context) ([]engine.Approval, error) {
	var approvals []engine.Approval
	if err := s.readDirJSON(filepath.Join(s.root, "approvals"), &approvals); err != nil {
		return nil, err
	}
	sort.Slice(approvals, func(i, j int) bool {
		return approvals[i].CreatedAt.After(approvals[j].CreatedAt)
	})
	return approvals, nil
}

func (s *FileStore) SaveFilePermission(_ context.Context, perm engine.FilePermission) error {
	return s.writeJSON(filepath.Join(s.root, "filepermissions", perm.ID+".json"), perm)
}

func (s *FileStore) GetFilePermission(_ context.Context, id string) (engine.FilePermission, error) {
	var perm engine.FilePermission
	err := s.readJSON(filepath.Join(s.root, "filepermissions", id+".json"), &perm)
	if errors.Is(err, fs.ErrNotExist) {
		return engine.FilePermission{}, engine.ErrNotFound
	}
	return perm, err
}

func (s *FileStore) ListFilePermissions(_ context.Context) ([]engine.FilePermission, error) {
	var perms []engine.FilePermission
	if err := s.readDirJSON(filepath.Join(s.root, "filepermissions"), &perms); err != nil {
		return nil, err
	}
	sort.Slice(perms, func(i, j int) bool {
		return perms[i].CreatedAt.After(perms[j].CreatedAt)
	})
	return perms, nil
}

func (s *FileStore) SaveStatus(_ context.Context, status engine.StatusSnapshot) error {
	return s.writeJSON(filepath.Join(s.root, "runtime", "status.json"), status)
}

func (s *FileStore) SaveMessage(_ context.Context, message engine.Message) error {
	return s.writeJSON(filepath.Join(s.root, "messages", message.ID+".json"), message)
}

func (s *FileStore) GetMessage(_ context.Context, id string) (engine.Message, error) {
	var message engine.Message
	err := s.readJSON(filepath.Join(s.root, "messages", id+".json"), &message)
	if errors.Is(err, fs.ErrNotExist) {
		return engine.Message{}, engine.ErrNotFound
	}
	return message, err
}

func (s *FileStore) ListMessages(_ context.Context) ([]engine.Message, error) {
	var messages []engine.Message
	if err := s.readDirJSON(filepath.Join(s.root, "messages"), &messages); err != nil {
		return nil, err
	}
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].CreatedAt.After(messages[j].CreatedAt)
	})
	return messages, nil
}

func (s *FileStore) SaveSession(_ context.Context, session engine.Session) error {
	return s.writeJSON(filepath.Join(s.root, "sessions", session.ID+".json"), session)
}

func (s *FileStore) GetSession(_ context.Context, id string) (engine.Session, error) {
	var session engine.Session
	err := s.readJSON(filepath.Join(s.root, "sessions", id+".json"), &session)
	if errors.Is(err, fs.ErrNotExist) {
		return engine.Session{}, engine.ErrNotFound
	}
	session.MessageCount = len(session.Messages)
	return session, err
}

func (s *FileStore) ListSessions(_ context.Context) ([]engine.Session, error) {
	var sessions []engine.Session
	if err := s.readDirJSON(filepath.Join(s.root, "sessions"), &sessions); err != nil {
		return nil, err
	}
	for index := range sessions {
		sessions[index].MessageCount = len(sessions[index].Messages)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

// maxAuditBytes is the size threshold (5 MB) that triggers rotation.
const maxAuditBytes = 5 * 1024 * 1024

// maxAuditRotations is the number of rotated files to keep.
const maxAuditRotations = 3

func (s *FileStore) AppendAudit(_ context.Context, event engine.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.root, "audits", "audit.jsonl")

	// Check rotation before appending.
	if info, statErr := os.Stat(path); statErr == nil && info.Size() >= maxAuditBytes {
		s.rotateAuditLocked(path)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	return err
}

// rotateAuditLocked shifts audit files: audit.jsonl → audit.1.jsonl → audit.2.jsonl → ...
// Oldest file beyond maxAuditRotations is deleted. Caller must hold s.mu.
func (s *FileStore) rotateAuditLocked(path string) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	// Remove the oldest file if it exists.
	oldest := filepath.Join(dir, fmt.Sprintf("%s.%d%s", stem, maxAuditRotations, ext))
	os.Remove(oldest)

	// Shift numbered files up: N-1 → N.
	for i := maxAuditRotations - 1; i >= 1; i-- {
		from := filepath.Join(dir, fmt.Sprintf("%s.%d%s", stem, i, ext))
		to := filepath.Join(dir, fmt.Sprintf("%s.%d%s", stem, i+1, ext))
		os.Rename(from, to)
	}

	// Rotate current file to .1.
	first := filepath.Join(dir, fmt.Sprintf("%s.1%s", stem, ext))
	os.Rename(path, first)
}

func (s *FileStore) writeJSON(path string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *FileStore) readJSON(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (s *FileStore) readDirJSON(path string, dest any) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	items := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(path, entry.Name()))
		if err != nil {
			return err
		}
		items = append(items, data)
	}

	payload := append([]byte{'['}, []byte{}...)
	for index, item := range items {
		if index > 0 {
			payload = append(payload, ',')
		}
		payload = append(payload, item...)
	}
	payload = append(payload, ']')
	if len(items) == 0 {
		payload = []byte("[]")
	}

	if err := json.Unmarshal(payload, dest); err != nil {
		return fmt.Errorf("decode directory payload %s: %w", path, err)
	}
	return nil
}
