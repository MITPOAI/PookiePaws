package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/persistence"
)

type ConversationTurn struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type ConversationWindow struct {
	mu    sync.Mutex
	limit int
	turns []ConversationTurn
	path  string // optional on-disk persistence path
}

type MemoryEntry struct {
	WorkflowID string            `json:"workflow_id"`
	Name       string            `json:"name"`
	Skill      string            `json:"skill"`
	Status     string            `json:"status"`
	Summary    string            `json:"summary"`
	Variables  map[string]string `json:"variables,omitempty"`
	RecordedAt time.Time         `json:"recorded_at"`
}

type MemorySnapshot struct {
	Narrative string            `json:"narrative,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
	Recent    []MemoryEntry     `json:"recent,omitempty"`
	LastFlush time.Time         `json:"last_flush"`
}

type MemoryReader interface {
	Snapshot(ctx context.Context) (MemorySnapshot, error)
}

type PersistentMemory struct {
	path         string
	format       persistence.Format
	bus          engine.EventBus
	factory      ProviderFactory
	maxEntries   int
	maxVariables int
	mu           sync.Mutex
}

var _ engine.MemoryCompressor = (*PersistentMemory)(nil)
var _ MemoryReader = (*PersistentMemory)(nil)

func NewConversationWindow(limit int) *ConversationWindow {
	if limit <= 0 {
		limit = 6
	}
	return &ConversationWindow{limit: limit}
}

func (w *ConversationWindow) Add(role string, content string) {
	if w == nil {
		return
	}

	role = strings.TrimSpace(role)
	content = strings.TrimSpace(content)
	if role == "" || content == "" {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.turns = append(w.turns, ConversationTurn{
		Role:      role,
		Content:   content,
		Timestamp: time.Now().UTC(),
	})
	if len(w.turns) > w.limit {
		w.turns = append([]ConversationTurn(nil), w.turns[len(w.turns)-w.limit:]...)
	}
	w.saveLocked()
}

func (w *ConversationWindow) Snapshot() []ConversationTurn {
	if w == nil {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	out := make([]ConversationTurn, len(w.turns))
	copy(out, w.turns)
	return out
}

func (w *ConversationWindow) Reset() {
	if w == nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.turns = nil
	w.saveLocked()
}

// SetPath configures on-disk persistence. When set, the window saves after
// every Add and loads existing turns on startup.
func (w *ConversationWindow) SetPath(path string) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.path = path
	if path != "" {
		w.loadLocked()
	}
}

func (w *ConversationWindow) saveLocked() {
	if w.path == "" {
		return
	}
	data, err := json.Marshal(w.turns)
	if err != nil {
		return
	}
	tmp := w.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	os.Rename(tmp, w.path)
}

func (w *ConversationWindow) loadLocked() {
	data, err := os.ReadFile(w.path)
	if err != nil {
		return
	}
	var turns []ConversationTurn
	if json.Unmarshal(data, &turns) == nil && len(turns) > 0 {
		if len(turns) > w.limit {
			turns = turns[len(turns)-w.limit:]
		}
		w.turns = turns
	}
}

func NewPersistentMemory(runtimeRoot string, factory ProviderFactory, bus engine.EventBus) (*PersistentMemory, error) {
	return NewPersistentMemoryWithOptions(runtimeRoot, factory, bus, persistence.FormatCompactV1)
}

func NewPersistentMemoryWithOptions(runtimeRoot string, factory ProviderFactory, bus engine.EventBus, format persistence.Format) (*PersistentMemory, error) {
	format = persistence.NormalizeFormat(string(format))
	path := PersistentMemoryPath(runtimeRoot, format)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return &PersistentMemory{
		path:         path,
		format:       format,
		bus:          bus,
		factory:      factory,
		maxEntries:   16,
		maxVariables: 24,
	}, nil
}

func (m *PersistentMemory) Snapshot(_ context.Context) (MemorySnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.readLocked()
}

func (m *PersistentMemory) RecordWorkflow(ctx context.Context, workflow engine.Workflow) error {
	if m == nil {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	snapshot, err := m.readLocked()
	if err != nil {
		return err
	}

	variables := extractWorkflowVariables(workflow, m.maxVariables)
	summary := m.summarizeWorkflow(ctx, workflow, variables)
	recordedAt := time.Now().UTC()

	entry := MemoryEntry{
		WorkflowID: workflow.ID,
		Name:       workflow.Name,
		Skill:      workflow.Skill,
		Status:     string(workflow.Status),
		Summary:    summary,
		Variables:  variables,
		RecordedAt: recordedAt,
	}

	snapshot.Recent = append(snapshot.Recent, entry)
	if len(snapshot.Recent) > m.maxEntries {
		snapshot.Recent = append([]MemoryEntry(nil), snapshot.Recent[len(snapshot.Recent)-m.maxEntries:]...)
	}
	if snapshot.Variables == nil {
		snapshot.Variables = map[string]string{}
	}
	for key, value := range variables {
		snapshot.Variables[key] = value
	}
	if len(snapshot.Variables) > m.maxVariables {
		snapshot.Variables = trimVariableMap(snapshot.Variables, m.maxVariables)
	}
	snapshot.Narrative = buildNarrative(snapshot.Recent)
	snapshot.LastFlush = recordedAt

	if err := m.writeLocked(snapshot); err != nil {
		return err
	}
	if m.bus != nil {
		_ = m.bus.Publish(ctx, engine.Event{
			Type:       engine.EventContextFlush,
			WorkflowID: workflow.ID,
			Source:     "brain-memory",
			Payload: map[string]any{
				"skill":              workflow.Skill,
				"status":             workflow.Status,
				"summary":            summary,
				"critical_variables": len(variables),
			},
		})
	}
	return nil
}

func (m *PersistentMemory) summarizeWorkflow(ctx context.Context, workflow engine.Workflow, variables map[string]string) string {
	summary := fallbackWorkflowSummary(workflow, variables)
	if m == nil || m.factory == nil || !m.factory.Available() {
		return summary
	}

	compressCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	provider, err := m.factory.New(compressCtx)
	if err != nil {
		return summary
	}
	defer provider.Close()

	payload, err := json.MarshalIndent(map[string]any{
		"id":        workflow.ID,
		"name":      workflow.Name,
		"skill":     workflow.Skill,
		"status":    workflow.Status,
		"input":     workflow.Input,
		"output":    workflow.Output,
		"error":     workflow.Error,
		"variables": variables,
	}, "", "  ")
	if err != nil {
		return summary
	}

	response, err := provider.Complete(compressCtx, CompletionRequest{
		SystemPrompt: "Summarize the completed marketing workflow in at most two sentences. Keep it factual, retain named business entities and campaign variables, and avoid hype.",
		UserPrompt:   string(payload),
	})
	if err != nil {
		return summary
	}

	text := strings.TrimSpace(response.Raw)
	if text == "" {
		return summary
	}
	return shrinkText(text, 280)
}

func (m *PersistentMemory) readLocked() (MemorySnapshot, error) {
	for _, format := range persistence.PreferredReadOrder(m.format) {
		path := PersistentMemoryPath(filepath.Dir(filepath.Dir(filepath.Dir(m.path))), format)
		snapshot, err := readMemoryFile(path, format)
		if err == nil {
			if snapshot.Variables == nil {
				snapshot.Variables = map[string]string{}
			}
			return snapshot, nil
		}
		if os.IsNotExist(err) {
			continue
		}
		return MemorySnapshot{}, err
	}
	return MemorySnapshot{Variables: map[string]string{}}, nil
}

func (m *PersistentMemory) writeLocked(snapshot MemorySnapshot) error {
	if err := writeMemoryFile(m.path, m.format, snapshot); err != nil {
		return err
	}
	if m.format == persistence.FormatCompactV1 {
		_ = os.Remove(PersistentMemoryPath(filepath.Dir(filepath.Dir(filepath.Dir(m.path))), persistence.FormatJSON))
	}
	return nil
}

func extractWorkflowVariables(workflow engine.Workflow, max int) map[string]string {
	values := map[string]string{
		"workflow.skill":  workflow.Skill,
		"workflow.status": string(workflow.Status),
	}
	if strings.TrimSpace(workflow.Name) != "" {
		values["workflow.name"] = strings.TrimSpace(workflow.Name)
	}
	if strings.TrimSpace(workflow.Error) != "" {
		values["workflow.error"] = shrinkText(workflow.Error, 160)
	}

	flattenScalarMap("input", workflow.Input, values)
	flattenScalarMap("output", workflow.Output, values)
	return trimVariableMap(values, max)
}

func flattenScalarMap(prefix string, input map[string]any, out map[string]string) {
	if len(input) == 0 {
		return
	}

	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return variablePriority(keys[i]) > variablePriority(keys[j])
	})

	for _, key := range keys {
		flattenScalarValue(prefix+"."+key, input[key], out)
	}
}

func flattenScalarValue(path string, value any, out map[string]string) {
	switch cast := value.(type) {
	case nil:
		return
	case string:
		cast = strings.TrimSpace(cast)
		if cast != "" {
			out[path] = shrinkText(cast, 180)
		}
	case bool:
		out[path] = fmt.Sprintf("%t", cast)
	case float64, float32, int, int32, int64, uint, uint32, uint64:
		out[path] = fmt.Sprint(cast)
	case []any:
		items := make([]string, 0, len(cast))
		for _, item := range cast {
			if text := simpleScalar(item); text != "" {
				items = append(items, text)
			}
		}
		if len(items) > 0 {
			out[path] = shrinkText(strings.Join(items, ", "), 180)
		}
	case map[string]any:
		flattenScalarMap(path, cast, out)
	}
}

func simpleScalar(value any) string {
	switch cast := value.(type) {
	case string:
		return strings.TrimSpace(cast)
	case bool:
		return fmt.Sprintf("%t", cast)
	case float64, float32, int, int32, int64, uint, uint32, uint64:
		return fmt.Sprint(cast)
	default:
		return ""
	}
}

func trimVariableMap(values map[string]string, max int) map[string]string {
	if max <= 0 || len(values) <= max {
		return values
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := variablePriority(keys[i])
		right := variablePriority(keys[j])
		if left == right {
			return keys[i] < keys[j]
		}
		return left > right
	})

	trimmed := make(map[string]string, max)
	for _, key := range keys[:max] {
		trimmed[key] = values[key]
	}
	return trimmed
}

func variablePriority(key string) int {
	key = strings.ToLower(strings.TrimSpace(key))
	score := 1
	for _, token := range []string{"campaign", "audience", "segment", "competitor", "offer", "channel", "url", "seo", "keyword", "market", "region", "product"} {
		if strings.Contains(key, token) {
			score += 3
		}
	}
	if strings.Contains(key, "error") || strings.Contains(key, "status") {
		score += 2
	}
	return score
}

func buildNarrative(recent []MemoryEntry) string {
	if len(recent) == 0 {
		return ""
	}

	start := len(recent) - 3
	if start < 0 {
		start = 0
	}
	parts := make([]string, 0, len(recent)-start)
	for _, entry := range recent[start:] {
		if strings.TrimSpace(entry.Summary) != "" {
			parts = append(parts, entry.Summary)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func fallbackWorkflowSummary(workflow engine.Workflow, variables map[string]string) string {
	subject := firstNonEmpty(variables["input.campaign_name"], variables["input.segment"], workflow.Name, workflow.Skill, "workflow")
	switch workflow.Status {
	case engine.WorkflowCompleted:
		return fmt.Sprintf("%s completed through %s and the resulting state was persisted for future routing.", subject, workflow.Skill)
	case engine.WorkflowRejected:
		return fmt.Sprintf("%s was paused and then rejected after approval review, so the plan should be revised before retrying.", subject)
	case engine.WorkflowFailed:
		if workflow.Error != "" {
			return fmt.Sprintf("%s failed during %s because %s.", subject, workflow.Skill, shrinkText(workflow.Error, 160))
		}
		return fmt.Sprintf("%s failed during %s and needs a safer follow-up attempt.", subject, workflow.Skill)
	default:
		return fmt.Sprintf("%s moved through %s with status %s.", subject, workflow.Skill, workflow.Status)
	}
}

func shrinkText(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}
