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
	EventWorkflowSubmitted     EventType = "workflow.submitted"
	EventWorkflowUpdated       EventType = "workflow.updated"
	EventSkillCompleted        EventType = "skill.completed"
	EventApprovalRequired      EventType = "approval.required"
	EventAdapterExecuted       EventType = "adapter.executed"
	EventAdapterFailed         EventType = "adapter.failed"
	EventSubTurnStarted        EventType = "subturn.started"
	EventSubTurnCompleted      EventType = "subturn.completed"
	EventSubTurnOrphaned       EventType = "subturn.orphaned"
	EventBrainCommand          EventType = "brain.command"
	EventBrainCommandError     EventType = "brain.command.error"
	EventContextFlush          EventType = "brain.context.flush"
	EventContextCompressFailed EventType = "brain.context.compress_failed"
	EventExecutionBlocked      EventType = "security.execution.blocked"

	EventFileAccessRequested EventType = "file.access.requested"
	EventFileAccessApproved  EventType = "file.access.approved"
	EventFileAccessRejected  EventType = "file.access.rejected"
	EventFileAccessDenied    EventType = "file.access.denied"

	EventChannelIncoming EventType = "channel.incoming"
	EventAutoApproved    EventType = "approval.auto_approved"
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
	Publish(ctx context.Context, event Event) error
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

// AutoApprovalPolicy controls whether low-risk adapter actions bypass the
// human approval modal and execute silently with an audit log entry.
type AutoApprovalPolicy struct {
	Enabled bool   `json:"enabled"`
	MaxRisk string `json:"max_risk"` // "low" or "medium"
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

type InterceptionDecision struct {
	Allowed                bool           `json:"allowed"`
	Risk                   string         `json:"risk,omitempty"`
	Reason                 string         `json:"reason,omitempty"`
	Violation              string         `json:"violation,omitempty"`
	SafeAlternativePrompt  string         `json:"safe_alternative_prompt,omitempty"`
	SafeAlternativeContext map[string]any `json:"safe_alternative_context,omitempty"`
}

type ExecutionInterceptor interface {
	Inspect(ctx context.Context, skill SkillDefinition, input map[string]any) (InterceptionDecision, error)
}

type WorkflowBlockedError struct {
	Skill    string
	Decision InterceptionDecision
}

func (e WorkflowBlockedError) Error() string {
	reason := e.Decision.Reason
	if reason == "" {
		reason = "workflow blocked by security policy"
	}
	if e.Skill == "" {
		return reason
	}
	return "workflow " + e.Skill + " blocked: " + reason
}

type SecretProvider interface {
	Get(name string) (string, error)
	RedactMap(payload map[string]any) map[string]any
}

type MemoryCompressor interface {
	RecordWorkflow(ctx context.Context, workflow Workflow) error
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

type ChannelSendRequest struct {
	MessageID         string            `json:"message_id,omitempty"`
	WorkflowID        string            `json:"workflow_id,omitempty"`
	Provider          string            `json:"provider,omitempty"`
	Channel           string            `json:"channel"`
	To                string            `json:"to"`
	Type              string            `json:"type"`
	Text              string            `json:"text,omitempty"`
	TemplateName      string            `json:"template_name,omitempty"`
	TemplateLanguage  string            `json:"template_language,omitempty"`
	TemplateVariables map[string]string `json:"template_variables,omitempty"`
	Metadata          map[string]any    `json:"metadata,omitempty"`
	Test              bool              `json:"test,omitempty"`
}

type ChannelSendResult struct {
	MessageID  string         `json:"message_id,omitempty"`
	ExternalID string         `json:"external_id,omitempty"`
	Provider   string         `json:"provider"`
	Channel    string         `json:"channel"`
	Status     string         `json:"status"`
	Details    map[string]any `json:"details,omitempty"`
}

type ChannelDeliveryEvent struct {
	Provider   string         `json:"provider"`
	Channel    string         `json:"channel"`
	MessageID  string         `json:"message_id,omitempty"`
	ExternalID string         `json:"external_id,omitempty"`
	Recipient  string         `json:"recipient,omitempty"`
	Status     string         `json:"status"`
	Timestamp  time.Time      `json:"timestamp"`
	Raw        map[string]any `json:"raw,omitempty"`
}

type ChannelProviderStatus struct {
	Provider     string   `json:"provider"`
	Channel      string   `json:"channel"`
	Configured   bool     `json:"configured"`
	Healthy      bool     `json:"healthy"`
	Message      string   `json:"message,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type MessageStatus string

const (
	MessageQueued          MessageStatus = "queued"
	MessagePendingApproval MessageStatus = "pending_approval"
	MessageApproved        MessageStatus = "approved"
	MessageSent            MessageStatus = "sent"
	MessageDelivered       MessageStatus = "delivered"
	MessageRead            MessageStatus = "read"
	MessageFailed          MessageStatus = "failed"
	MessageRejected        MessageStatus = "rejected"
)

type Message struct {
	ID               string                 `json:"id"`
	WorkflowID       string                 `json:"workflow_id,omitempty"`
	Provider         string                 `json:"provider"`
	Channel          string                 `json:"channel"`
	Direction        string                 `json:"direction"`
	Recipient        string                 `json:"recipient"`
	Type             string                 `json:"type"`
	Text             string                 `json:"text,omitempty"`
	TemplateName     string                 `json:"template_name,omitempty"`
	TemplateLanguage string                 `json:"template_language,omitempty"`
	Status           MessageStatus          `json:"status"`
	ExternalID       string                 `json:"external_id,omitempty"`
	Metadata         map[string]any         `json:"metadata,omitempty"`
	Details          map[string]any         `json:"details,omitempty"`
	LastError        string                 `json:"last_error,omitempty"`
	DeliveryEvents   []ChannelDeliveryEvent `json:"delivery_events,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type SessionStatus string

const (
	SessionAccepted         SessionStatus = "accepted"
	SessionRunning          SessionStatus = "running"
	SessionAwaitingApproval SessionStatus = "awaiting_approval"
	SessionCompleted        SessionStatus = "completed"
	SessionFailed           SessionStatus = "failed"
	SessionBlocked          SessionStatus = "blocked"
)

type SessionPromptTrace struct {
	Mode         string    `json:"mode"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
	UserPrompt   string    `json:"user_prompt,omitempty"`
	Model        string    `json:"model,omitempty"`
	RawResponse  string    `json:"raw_response,omitempty"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type SessionMessage struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	Role       string    `json:"role"`
	Kind       string    `json:"kind"`
	Content    string    `json:"content"`
	Status     string    `json:"status,omitempty"`
	WorkflowID string    `json:"workflow_id,omitempty"`
	Model      string    `json:"model,omitempty"`
	Skill      string    `json:"skill,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type SessionRun struct {
	ID               string              `json:"id"`
	SessionID        string              `json:"session_id"`
	Prompt           string              `json:"prompt"`
	Status           SessionStatus       `json:"status"`
	WorkflowID       string              `json:"workflow_id,omitempty"`
	Skill            string              `json:"skill,omitempty"`
	Error            string              `json:"error,omitempty"`
	AcceptedAt       time.Time           `json:"accepted_at"`
	StartedAt        time.Time           `json:"started_at,omitempty"`
	FinishedAt       time.Time           `json:"finished_at,omitempty"`
	Trace            *SessionPromptTrace `json:"trace,omitempty"`
	AlternativeTrace *SessionPromptTrace `json:"alternative_trace,omitempty"`
}

type SessionSummary struct {
	ID           string        `json:"id"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	MessageCount int           `json:"message_count"`
	LastStatus   SessionStatus `json:"last_status,omitempty"`
}

type Session struct {
	SessionSummary
	Messages []SessionMessage `json:"messages"`
	Runs     []SessionRun     `json:"runs,omitempty"`
}

type MessageRequest struct {
	Name              string            `json:"name,omitempty"`
	Provider          string            `json:"provider,omitempty"`
	Channel           string            `json:"channel"`
	To                string            `json:"to"`
	Type              string            `json:"type"`
	Text              string            `json:"text,omitempty"`
	TemplateName      string            `json:"template_name,omitempty"`
	TemplateLanguage  string            `json:"template_language,omitempty"`
	TemplateVariables map[string]string `json:"template_variables,omitempty"`
	Test              bool              `json:"test,omitempty"`
}

type MessageSubmitResult struct {
	Message  Message  `json:"message"`
	Workflow Workflow `json:"workflow"`
}

type CRMAdapter interface {
	Name() string
	Execute(ctx context.Context, action AdapterAction, secrets SecretProvider) (AdapterResult, error)
}

type SMSAdapter interface {
	Name() string
	Execute(ctx context.Context, action AdapterAction, secrets SecretProvider) (AdapterResult, error)
}

// ChannelIncomingMessage represents an inbound message received from a channel
// provider webhook (e.g. WhatsApp incoming text from a customer).
type ChannelIncomingMessage struct {
	Provider  string         `json:"provider"`
	Channel   string         `json:"channel"`
	MessageID string         `json:"message_id"`
	From      string         `json:"from"`
	FromName  string         `json:"from_name,omitempty"`
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Raw       map[string]any `json:"raw,omitempty"`
}

type ChannelAdapter interface {
	Name() string
	Channel() string
	Status(secrets SecretProvider) ChannelProviderStatus
	Test(ctx context.Context, secrets SecretProvider) (ChannelProviderStatus, error)
	Send(ctx context.Context, req ChannelSendRequest, secrets SecretProvider) (ChannelSendResult, error)
	ParseDeliveryEvents(payload map[string]any) []ChannelDeliveryEvent
	ParseIncomingMessages(payload map[string]any) []ChannelIncomingMessage
}

// MarketingChannel is the unified interface for all marketing channel plugins.
// It standardises how PookiePaws interacts with outbound channels, CRM systems,
// research tools, and export services. Community developers implement this
// interface to extend the agent with new integrations.
type MarketingChannel interface {
	// Name returns the unique adapter identifier (e.g. "resend", "hubspot").
	Name() string
	// Kind returns the channel category: "crm", "sms", "email", "whatsapp", "research", "export".
	Kind() string
	// Status reports whether the channel is configured with valid secrets.
	Status(secrets SecretProvider) ChannelProviderStatus
	// Test verifies the channel is reachable and credentials are valid.
	Test(ctx context.Context, secrets SecretProvider) (ChannelProviderStatus, error)
	// Execute runs a channel operation. The AdapterAction.Operation field
	// selects the specific action (e.g. "send_email", "create_contact", "scrape").
	Execute(ctx context.Context, action AdapterAction, secrets SecretProvider) (AdapterResult, error)
	// SecretKeys returns the configuration keys this channel needs from the vault.
	SecretKeys() []string
}

// MarketingChannelRegistry stores and retrieves registered channel plugins.
type MarketingChannelRegistry interface {
	Register(channel MarketingChannel)
	Get(name string) (MarketingChannel, bool)
	List() []MarketingChannel
	ByKind(kind string) []MarketingChannel
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
	SaveMessage(ctx context.Context, message Message) error
	GetMessage(ctx context.Context, id string) (Message, error)
	ListMessages(ctx context.Context) ([]Message, error)
	SaveSession(ctx context.Context, session Session) error
	GetSession(ctx context.Context, id string) (Session, error)
	ListSessions(ctx context.Context) ([]Session, error)
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
	Channels(ctx context.Context) ([]ChannelProviderStatus, error)
	TestChannel(ctx context.Context, channel string) (ChannelProviderStatus, error)
	SubmitMessage(ctx context.Context, req MessageRequest) (MessageSubmitResult, error)
	GetMessage(ctx context.Context, id string) (Message, error)
	ProcessChannelDelivery(ctx context.Context, event ChannelDeliveryEvent) (Message, error)
}
