package engine

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyClosed = errors.New("already closed")
)

type EventType string

const (
	EventWorkflowSubmitted EventType = "workflow.submitted"
	EventWorkflowUpdated   EventType = "workflow.updated"
	EventSkillCompleted    EventType = "skill.completed"
	EventApprovalRequired  EventType = "approval.required"
	EventAdapterExecuted   EventType = "adapter.executed"
	EventAdapterFailed     EventType = "adapter.failed"
	EventSubTurnStarted    EventType = "subturn.started"
	EventSubTurnCompleted  EventType = "subturn.completed"
	EventSubTurnOrphaned   EventType = "subturn.orphaned"
	EventBrainCommand      EventType = "brain.command"
	EventBrainCommandError EventType = "brain.command.error"

	EventFileAccessRequested EventType = "file.access.requested"
	EventFileAccessApproved  EventType = "file.access.approved"
	EventFileAccessRejected  EventType = "file.access.rejected"
	EventFileAccessDenied    EventType = "file.access.denied"
)

type Event struct {
	ID         string         `json:"id"`
	Type       EventType      `json:"type"`
	Time       time.Time      `json:"time"`
	WorkflowID string         `json:"workflow_id,omitempty"`
	Source     string         `json:"source,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
}

type EventSubscription struct {
	ID uint64
	C  <-chan Event
}

type EventBusSnapshot struct {
	Subscribers int               `json:"subscribers"`
	Published   uint64            `json:"published"`
	Closed      bool              `json:"closed"`
	Dropped     map[EventType]int `json:"dropped"`
}

type EventBus interface {
	Subscribe(buffer int) EventSubscription
	Unsubscribe(id uint64)
	Publish(event Event) error
	Snapshot() EventBusSnapshot
	Close()
}

type SubTurnState string

const (
	SubTurnStateRunning   SubTurnState = "running"
	SubTurnStateCompleted SubTurnState = "completed"
	SubTurnStateFailed    SubTurnState = "failed"
	SubTurnStateCanceled  SubTurnState = "canceled"
	SubTurnStateOrphaned  SubTurnState = "orphaned"
)

type SubTurnSpec struct {
	Name     string
	ParentID string
	Depth    int
	Timeout  time.Duration
}

type SubTurnResult struct {
	ID         string         `json:"id"`
	ParentID   string         `json:"parent_id,omitempty"`
	Status     SubTurnState   `json:"status"`
	Output     map[string]any `json:"output,omitempty"`
	Err        string         `json:"err,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at"`
}

type SubTurnStatus struct {
	ID         string        `json:"id"`
	ParentID   string        `json:"parent_id,omitempty"`
	Name       string        `json:"name"`
	Depth      int           `json:"depth"`
	State      SubTurnState  `json:"state"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt *time.Time    `json:"finished_at,omitempty"`
	Err        string        `json:"err,omitempty"`
	Timeout    time.Duration `json:"timeout"`
}

type SubTurnRunner func(context.Context) (map[string]any, error)

type SubTurnManager interface {
	Spawn(ctx context.Context, spec SubTurnSpec, runner SubTurnRunner) (string, error)
	Wait(ctx context.Context, id string) (SubTurnResult, error)
	Cancel(id string) error
	CancelChildren(parentID string) error
	Snapshot() []SubTurnStatus
	Close() error
}

type WorkflowStatus string

const (
	WorkflowQueued          WorkflowStatus = "queued"
	WorkflowRunning         WorkflowStatus = "running"
	WorkflowWaitingApproval WorkflowStatus = "waiting_approval"
	WorkflowCompleted       WorkflowStatus = "completed"
	WorkflowFailed          WorkflowStatus = "failed"
	WorkflowRejected        WorkflowStatus = "rejected"
)

type WorkflowDefinition struct {
	Name  string         `json:"name"`
	Skill string         `json:"skill"`
	Input map[string]any `json:"input"`
}

type Workflow struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Skill     string         `json:"skill"`
	Status    WorkflowStatus `json:"status"`
	Input     map[string]any `json:"input,omitempty"`
	Output    map[string]any `json:"output,omitempty"`
	Error     string         `json:"error,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type ApprovalState string

const (
	ApprovalPending  ApprovalState = "pending"
	ApprovalApproved ApprovalState = "approved"
	ApprovalRejected ApprovalState = "rejected"
	ApprovalExpired  ApprovalState = "expired"
)

type Approval struct {
	ID         string         `json:"id"`
	WorkflowID string         `json:"workflow_id"`
	Skill      string         `json:"skill"`
	Adapter    string         `json:"adapter"`
	Action     string         `json:"action"`
	State      ApprovalState  `json:"state"`
	Payload    map[string]any `json:"payload,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	ExpiresAt  *time.Time     `json:"expires_at,omitempty"`
}

type FileAccessMode string

const (
	FileAccessRead  FileAccessMode = "read"
	FileAccessWrite FileAccessMode = "write"
)

type FilePermission struct {
	ID         string         `json:"id"`
	WorkflowID string         `json:"workflow_id,omitempty"`
	Path       string         `json:"path"`
	Mode       FileAccessMode `json:"mode"`
	State      ApprovalState  `json:"state"`
	Requester  string         `json:"requester"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	ExpiresAt  *time.Time     `json:"expires_at,omitempty"`
}

type StatusSnapshot struct {
	RuntimeRoot            string           `json:"runtime_root"`
	WorkspaceRoot          string           `json:"workspace_root"`
	Workflows              int              `json:"workflows"`
	PendingApprovals       int              `json:"pending_approvals"`
	PendingFilePermissions int              `json:"pending_file_permissions"`
	SubTurns               []SubTurnStatus  `json:"subturns"`
	EventBus               EventBusSnapshot `json:"event_bus"`
	StartedAt              time.Time        `json:"started_at"`
}

type Sandbox interface {
	RuntimeRoot() string
	WorkspaceRoot() string
	ResolveWithinWorkspace(path string) (string, error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte) error
}

type ExecGuard interface {
	Validate(command []string) error
}

type SecretProvider interface {
	Get(name string) (string, error)
	RedactMap(payload map[string]any) map[string]any
}

type SkillDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Tools       []string    `json:"tools,omitempty"`
	Events      []EventType `json:"events,omitempty"`
	Prompt      string      `json:"prompt,omitempty"`
}

type SkillRequest struct {
	Workflow Workflow
	Input    map[string]any
	Sandbox  Sandbox
	Secrets  SecretProvider
	Now      time.Time
}

type AdapterAction struct {
	Adapter          string         `json:"adapter"`
	Operation        string         `json:"operation"`
	Payload          map[string]any `json:"payload,omitempty"`
	RequiresApproval bool           `json:"requires_approval"`
}

type SkillResult struct {
	Output  map[string]any `json:"output,omitempty"`
	Actions []AdapterAction
}

type Skill interface {
	Definition() SkillDefinition
	Validate(input map[string]any) error
	Execute(ctx context.Context, req SkillRequest) (SkillResult, error)
}

type SkillRegistry interface {
	Get(name string) (Skill, bool)
	List() []SkillDefinition
}

type AdapterResult struct {
	Adapter   string         `json:"adapter"`
	Operation string         `json:"operation"`
	Status    string         `json:"status"`
	Details   map[string]any `json:"details,omitempty"`
}

type CRMAdapter interface {
	Name() string
	Execute(ctx context.Context, action AdapterAction, secrets SecretProvider) (AdapterResult, error)
}

type SMSAdapter interface {
	Name() string
	Execute(ctx context.Context, action AdapterAction, secrets SecretProvider) (AdapterResult, error)
}

type StateStore interface {
	SaveWorkflow(ctx context.Context, workflow Workflow) error
	GetWorkflow(ctx context.Context, id string) (Workflow, error)
	ListWorkflows(ctx context.Context) ([]Workflow, error)
	SaveApproval(ctx context.Context, approval Approval) error
	GetApproval(ctx context.Context, id string) (Approval, error)
	ListApprovals(ctx context.Context) ([]Approval, error)
	SaveFilePermission(ctx context.Context, perm FilePermission) error
	GetFilePermission(ctx context.Context, id string) (FilePermission, error)
	ListFilePermissions(ctx context.Context) ([]FilePermission, error)
	SaveStatus(ctx context.Context, status StatusSnapshot) error
	AppendAudit(ctx context.Context, event Event) error
}

type WorkflowCoordinator interface {
	SubmitWorkflow(ctx context.Context, def WorkflowDefinition) (Workflow, error)
	ListWorkflows(ctx context.Context) ([]Workflow, error)
	ListApprovals(ctx context.Context) ([]Approval, error)
	Approve(ctx context.Context, id string) (Approval, error)
	Reject(ctx context.Context, id string) (Approval, error)
	Status(ctx context.Context) (StatusSnapshot, error)
	ValidateSkill(ctx context.Context, skillName string, input map[string]any) error
	SkillDefinitions() []SkillDefinition
	ApproveFileAccess(ctx context.Context, id string) (FilePermission, error)
	RejectFileAccess(ctx context.Context, id string) (FilePermission, error)
	ListFilePermissions(ctx context.Context) ([]FilePermission, error)
}
