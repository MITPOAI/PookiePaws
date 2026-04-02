package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

type WorkflowCoordinatorConfig struct {
	Bus         EventBus
	SubTurns    SubTurnManager
	Store       StateStore
	Skills      SkillRegistry
	Sandbox     Sandbox
	Secrets     SecretProvider
	Memory      MemoryCompressor
	Interceptor ExecutionInterceptor
	CRMAdapter  CRMAdapter
	SMSAdapter  SMSAdapter
	WhatsApp    ChannelAdapter
	RuntimeRoot string
	Workspace   string
}

type StandardWorkflowCoordinator struct {
	bus          EventBus
	subturns     SubTurnManager
	store        StateStore
	skills       SkillRegistry
	sandbox      Sandbox
	secrets      SecretProvider
	memory       MemoryCompressor
	interceptor  ExecutionInterceptor
	crmAdapter   CRMAdapter
	smsAdapter   SMSAdapter
	whatsApp     ChannelAdapter
	runtimeRoot  string
	workspace    string
	startedAt    time.Time
	nextID       uint64
	autoApproval atomic.Value // stores *AutoApprovalPolicy
}

func NewWorkflowCoordinator(cfg WorkflowCoordinatorConfig) (*StandardWorkflowCoordinator, error) {
	switch {
	case cfg.Bus == nil:
		return nil, fmt.Errorf("event bus is required")
	case cfg.SubTurns == nil:
		return nil, fmt.Errorf("subturn manager is required")
	case cfg.Store == nil:
		return nil, fmt.Errorf("state store is required")
	case cfg.Skills == nil:
		return nil, fmt.Errorf("skill registry is required")
	case cfg.Sandbox == nil:
		return nil, fmt.Errorf("sandbox is required")
	case cfg.Secrets == nil:
		return nil, fmt.Errorf("secret provider is required")
	}

	return &StandardWorkflowCoordinator{
		bus:         cfg.Bus,
		subturns:    cfg.SubTurns,
		store:       cfg.Store,
		skills:      cfg.Skills,
		sandbox:     cfg.Sandbox,
		secrets:     cfg.Secrets,
		memory:      cfg.Memory,
		interceptor: cfg.Interceptor,
		crmAdapter:  cfg.CRMAdapter,
		smsAdapter:  cfg.SMSAdapter,
		whatsApp:    cfg.WhatsApp,
		runtimeRoot: cfg.RuntimeRoot,
		workspace:   cfg.Workspace,
		startedAt:   time.Now().UTC(),
	}, nil
}

func (c *StandardWorkflowCoordinator) SubmitWorkflow(ctx context.Context, def WorkflowDefinition) (Workflow, error) {
	skill, ok := c.skills.Get(def.Skill)
	if !ok {
		return Workflow{}, fmt.Errorf("unknown skill %q", def.Skill)
	}
	var skillRisk string
	if c.interceptor != nil {
		decision, err := c.interceptor.Inspect(ctx, skill.Definition(), def.Input)
		if err != nil {
			return Workflow{}, err
		}
		skillRisk = decision.Risk
		if !decision.Allowed {
			c.publishAndAudit(ctx, Event{
				Type:   EventExecutionBlocked,
				Source: "workflow-coordinator",
				Payload: map[string]any{
					"skill":     def.Skill,
					"risk":      decision.Risk,
					"reason":    decision.Reason,
					"violation": decision.Violation,
				},
			})
			return Workflow{}, WorkflowBlockedError{
				Skill:    def.Skill,
				Decision: decision,
			}
		}
	}
	if err := skill.Validate(def.Input); err != nil {
		return Workflow{}, err
	}

	now := time.Now().UTC()
	workflow := Workflow{
		ID:        c.nextWorkflowID(),
		Name:      def.Name,
		Skill:     def.Skill,
		Status:    WorkflowQueued,
		Input:     def.Input,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if workflow.Name == "" {
		workflow.Name = workflow.Skill
	}

	if err := c.store.SaveWorkflow(ctx, workflow); err != nil {
		return Workflow{}, err
	}

	c.publishAndAudit(ctx, Event{
		Type:       EventWorkflowSubmitted,
		WorkflowID: workflow.ID,
		Source:     "workflow-coordinator",
		Payload: map[string]any{
			"skill": workflow.Skill,
			"name":  workflow.Name,
		},
	})

	workflow.Status = WorkflowRunning
	workflow.UpdatedAt = time.Now().UTC()
	if err := c.store.SaveWorkflow(ctx, workflow); err != nil {
		return Workflow{}, err
	}

	var result SkillResult
	subturnID, err := c.subturns.Spawn(ctx, SubTurnSpec{
		Name:     "skill:" + workflow.Skill,
		ParentID: workflow.ID,
		Depth:    1,
		Timeout:  30 * time.Second,
	}, func(runCtx context.Context) (map[string]any, error) {
		r, execErr := skill.Execute(runCtx, SkillRequest{
			Workflow: workflow,
			Input:    workflow.Input,
			Sandbox:  c.sandbox,
			Secrets:  c.secrets,
			Now:      time.Now().UTC(),
		})
		result = r
		return r.Output, execErr
	})
	if err != nil {
		return Workflow{}, err
	}

	subturnResult, err := c.subturns.Wait(ctx, subturnID)
	if err != nil {
		return Workflow{}, err
	}
	if subturnResult.Err != "" {
		workflow.Status = WorkflowFailed
		workflow.Error = subturnResult.Err
		workflow.UpdatedAt = time.Now().UTC()
		_ = c.store.SaveWorkflow(ctx, workflow)
		c.publishAndAudit(ctx, Event{
			Type:       EventWorkflowUpdated,
			WorkflowID: workflow.ID,
			Source:     "workflow-coordinator",
			Payload: map[string]any{
				"status": workflow.Status,
				"error":  workflow.Error,
			},
		})
		c.recordMemory(ctx, workflow)
		return workflow, fmt.Errorf(subturnResult.Err)
	}

	workflow.Output = result.Output
	pendingApproval := false

	for _, action := range result.Actions {
		if action.Adapter == "whatsapp" {
			var prepareErr error
			action, prepareErr = c.prepareMessageAction(ctx, workflow, action)
			if prepareErr != nil {
				workflow.Status = WorkflowFailed
				workflow.Error = prepareErr.Error()
				workflow.UpdatedAt = time.Now().UTC()
				_ = c.store.SaveWorkflow(ctx, workflow)
				c.publishAndAudit(ctx, Event{
					Type:       EventWorkflowUpdated,
					WorkflowID: workflow.ID,
					Source:     "workflow-coordinator",
					Payload: map[string]any{
						"status": workflow.Status,
						"error":  workflow.Error,
					},
				})
				c.recordMemory(ctx, workflow)
				return workflow, prepareErr
			}
			if messageID, _ := action.Payload["message_id"].(string); messageID != "" {
				if workflow.Output == nil {
					workflow.Output = map[string]any{}
				}
				workflow.Output["message_id"] = messageID
			}
		}

		// Smart Sandbox: auto-approve low-risk actions when the policy permits.
		if action.RequiresApproval && c.shouldAutoApprove(skillRisk) {
			action.RequiresApproval = false
			c.publishAndAudit(ctx, Event{
				Type:       EventAutoApproved,
				WorkflowID: workflow.ID,
				Source:     "auto-approval",
				Payload: map[string]any{
					"skill":     workflow.Skill,
					"adapter":   action.Adapter,
					"operation": action.Operation,
					"risk":      skillRisk,
					"reason":    "auto-approved by smart sandbox policy",
				},
			})
		}

		if action.RequiresApproval {
			pendingApproval = true
			approval := Approval{
				ID:         c.nextApprovalID(),
				WorkflowID: workflow.ID,
				Skill:      workflow.Skill,
				Adapter:    action.Adapter,
				Action:     action.Operation,
				State:      ApprovalPending,
				Payload:    c.secrets.RedactMap(action.Payload),
				CreatedAt:  time.Now().UTC(),
				UpdatedAt:  time.Now().UTC(),
			}
			if err := c.store.SaveApproval(ctx, approval); err != nil {
				return Workflow{}, err
			}
			c.publishAndAudit(ctx, Event{
				Type:       EventApprovalRequired,
				WorkflowID: workflow.ID,
				Source:     "workflow-coordinator",
				Payload: map[string]any{
					"approval_id": approval.ID,
					"adapter":     approval.Adapter,
					"action":      approval.Action,
				},
			})
			continue
		}

		if _, err := c.executeAction(ctx, workflow, action); err != nil {
			workflow.Status = WorkflowFailed
			workflow.Error = err.Error()
			workflow.UpdatedAt = time.Now().UTC()
			_ = c.store.SaveWorkflow(ctx, workflow)
			c.publishAndAudit(ctx, Event{
				Type:       EventWorkflowUpdated,
				WorkflowID: workflow.ID,
				Source:     "workflow-coordinator",
				Payload: map[string]any{
					"status": workflow.Status,
					"error":  workflow.Error,
				},
			})
			c.recordMemory(ctx, workflow)
			return workflow, err
		}
	}

	if pendingApproval {
		workflow.Status = WorkflowWaitingApproval
	} else {
		workflow.Status = WorkflowCompleted
	}
	workflow.UpdatedAt = time.Now().UTC()

	if err := c.store.SaveWorkflow(ctx, workflow); err != nil {
		return Workflow{}, err
	}
	c.publishAndAudit(ctx, Event{
		Type:       EventSkillCompleted,
		WorkflowID: workflow.ID,
		Source:     "workflow-coordinator",
		Payload: map[string]any{
			"skill":  workflow.Skill,
			"status": workflow.Status,
		},
	})
	if workflow.Status == WorkflowCompleted {
		c.recordMemory(ctx, workflow)
	}

	return workflow, nil
}

func (c *StandardWorkflowCoordinator) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	workflows, err := c.store.ListWorkflows(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].CreatedAt.After(workflows[j].CreatedAt)
	})
	return workflows, nil
}

func (c *StandardWorkflowCoordinator) ListApprovals(ctx context.Context) ([]Approval, error) {
	approvals, err := c.store.ListApprovals(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(approvals, func(i, j int) bool {
		return approvals[i].CreatedAt.After(approvals[j].CreatedAt)
	})
	return approvals, nil
}

func (c *StandardWorkflowCoordinator) Approve(ctx context.Context, id string) (Approval, error) {
	approval, err := c.store.GetApproval(ctx, id)
	if err != nil {
		return Approval{}, err
	}
	if approval.State != ApprovalPending {
		return approval, fmt.Errorf("approval %s is %s", id, approval.State)
	}

	workflow, err := c.store.GetWorkflow(ctx, approval.WorkflowID)
	if err != nil {
		return Approval{}, err
	}

	if approval.Adapter == "whatsapp" {
		_ = c.updateMessageState(ctx, approval.Payload, func(message *Message) {
			message.Status = MessageApproved
			message.LastError = ""
		})
	}

	result, err := c.executeAction(ctx, workflow, AdapterAction{
		Adapter:          approval.Adapter,
		Operation:        approval.Action,
		Payload:          approval.Payload,
		RequiresApproval: true,
	})
	if err != nil {
		return Approval{}, err
	}

	approval.State = ApprovalApproved
	approval.UpdatedAt = time.Now().UTC()
	if err := c.store.SaveApproval(ctx, approval); err != nil {
		return Approval{}, err
	}

	workflow.Status = WorkflowCompleted
	workflow.UpdatedAt = time.Now().UTC()
	if workflow.Output == nil {
		workflow.Output = make(map[string]any)
	}
	workflow.Output["last_adapter_result"] = result.Details
	if err := c.store.SaveWorkflow(ctx, workflow); err != nil {
		return Approval{}, err
	}

	c.publishAndAudit(ctx, Event{
		Type:       EventWorkflowUpdated,
		WorkflowID: workflow.ID,
		Source:     "workflow-coordinator",
		Payload: map[string]any{
			"status": workflow.Status,
		},
	})
	c.recordMemory(ctx, workflow)
	return approval, nil
}

func (c *StandardWorkflowCoordinator) Reject(ctx context.Context, id string) (Approval, error) {
	approval, err := c.store.GetApproval(ctx, id)
	if err != nil {
		return Approval{}, err
	}
	if approval.State != ApprovalPending {
		return approval, fmt.Errorf("approval %s is %s", id, approval.State)
	}

	approval.State = ApprovalRejected
	approval.UpdatedAt = time.Now().UTC()
	if err := c.store.SaveApproval(ctx, approval); err != nil {
		return Approval{}, err
	}

	workflow, err := c.store.GetWorkflow(ctx, approval.WorkflowID)
	if err != nil {
		return Approval{}, err
	}
	workflow.Status = WorkflowRejected
	workflow.UpdatedAt = time.Now().UTC()
	if err := c.store.SaveWorkflow(ctx, workflow); err != nil {
		return Approval{}, err
	}
	if approval.Adapter == "whatsapp" {
		_ = c.updateMessageState(ctx, approval.Payload, func(message *Message) {
			message.Status = MessageRejected
			message.LastError = "approval rejected"
		})
	}

	c.publishAndAudit(ctx, Event{
		Type:       EventWorkflowUpdated,
		WorkflowID: workflow.ID,
		Source:     "workflow-coordinator",
		Payload: map[string]any{
			"status": workflow.Status,
			"reason": "approval_rejected",
		},
	})
	c.recordMemory(ctx, workflow)
	return approval, nil
}

func (c *StandardWorkflowCoordinator) Status(ctx context.Context) (StatusSnapshot, error) {
	workflows, err := c.store.ListWorkflows(ctx)
	if err != nil {
		return StatusSnapshot{}, err
	}
	approvals, err := c.store.ListApprovals(ctx)
	if err != nil {
		return StatusSnapshot{}, err
	}

	pending := 0
	for _, approval := range approvals {
		if approval.State == ApprovalPending {
			pending++
		}
	}

	filePerms, err := c.store.ListFilePermissions(ctx)
	if err != nil {
		return StatusSnapshot{}, err
	}
	pendingFilePerms := 0
	for _, fp := range filePerms {
		if fp.State == ApprovalPending {
			pendingFilePerms++
		}
	}

	status := StatusSnapshot{
		RuntimeRoot:            c.runtimeRoot,
		WorkspaceRoot:          c.workspace,
		Workflows:              len(workflows),
		PendingApprovals:       pending,
		PendingFilePermissions: pendingFilePerms,
		SubTurns:               c.subturns.Snapshot(),
		EventBus:               c.bus.Snapshot(),
		StartedAt:              c.startedAt,
	}

	if err := c.store.SaveStatus(ctx, status); err != nil {
		return StatusSnapshot{}, err
	}
	return status, nil
}

func (c *StandardWorkflowCoordinator) ValidateSkill(_ context.Context, skillName string, input map[string]any) error {
	skill, ok := c.skills.Get(skillName)
	if !ok {
		return fmt.Errorf("unknown skill %q", skillName)
	}
	return skill.Validate(input)
}

func (c *StandardWorkflowCoordinator) SkillDefinitions() []SkillDefinition {
	return c.skills.List()
}

func (c *StandardWorkflowCoordinator) executeAction(ctx context.Context, workflow Workflow, action AdapterAction) (AdapterResult, error) {
	var (
		result AdapterResult
		err    error
	)

	switch action.Adapter {
	case "salesmanago":
		if c.crmAdapter == nil {
			err = fmt.Errorf("crm adapter not configured")
			c.publishAdapterFailure(ctx, workflow, action, err)
			return AdapterResult{}, err
		}
		result, err = c.crmAdapter.Execute(ctx, action, c.secrets)
	case "mitto":
		if c.smsAdapter == nil {
			err = fmt.Errorf("sms adapter not configured")
			c.publishAdapterFailure(ctx, workflow, action, err)
			return AdapterResult{}, err
		}
		result, err = c.smsAdapter.Execute(ctx, action, c.secrets)
	case "whatsapp":
		if c.whatsApp == nil {
			err = fmt.Errorf("whatsapp adapter not configured")
			c.publishAdapterFailure(ctx, workflow, action, err)
			return AdapterResult{}, err
		}
		sendResult, sendErr := c.whatsApp.Send(ctx, c.buildChannelSendRequest(workflow, action), c.secrets)
		if sendErr != nil {
			err = sendErr
			c.publishAdapterFailure(ctx, workflow, action, err)
			_ = c.updateMessageState(ctx, action.Payload, func(message *Message) {
				message.Status = MessageFailed
				message.LastError = err.Error()
			})
			return AdapterResult{}, err
		}
		result = AdapterResult{
			Adapter:   action.Adapter,
			Operation: action.Operation,
			Status:    sendResult.Status,
			Details: map[string]any{
				"message_id":   sendResult.MessageID,
				"external_id":  sendResult.ExternalID,
				"provider":     sendResult.Provider,
				"channel":      sendResult.Channel,
				"send_details": sendResult.Details,
			},
		}
		_ = c.updateMessageState(ctx, action.Payload, func(message *Message) {
			message.Status = MessageSent
			message.ExternalID = sendResult.ExternalID
			if message.Details == nil {
				message.Details = map[string]any{}
			}
			message.Details["send_result"] = sendResult.Details
			message.LastError = ""
		})
	default:
		err = fmt.Errorf("unknown adapter %q", action.Adapter)
		c.publishAdapterFailure(ctx, workflow, action, err)
		return AdapterResult{}, err
	}
	if err != nil {
		c.publishAdapterFailure(ctx, workflow, action, err)
		return AdapterResult{}, err
	}

	c.publishAndAudit(ctx, Event{
		Type:       EventAdapterExecuted,
		WorkflowID: workflow.ID,
		Source:     "workflow-coordinator",
		Payload: map[string]any{
			"adapter":   result.Adapter,
			"operation": result.Operation,
			"status":    result.Status,
			"details":   result.Details,
		},
	})
	return result, nil
}

func (c *StandardWorkflowCoordinator) publishAndAudit(ctx context.Context, event Event) {
	_ = c.bus.Publish(event)
	_ = c.store.AppendAudit(ctx, event)
}

func (c *StandardWorkflowCoordinator) publishAdapterFailure(ctx context.Context, workflow Workflow, action AdapterAction, err error) {
	c.publishAndAudit(ctx, Event{
		Type:       EventAdapterFailed,
		WorkflowID: workflow.ID,
		Source:     "workflow-coordinator",
		Payload: map[string]any{
			"adapter":   action.Adapter,
			"operation": action.Operation,
			"error":     err.Error(),
		},
	})
}

func (c *StandardWorkflowCoordinator) recordMemory(ctx context.Context, workflow Workflow) {
	if c.memory == nil {
		return
	}
	if err := c.memory.RecordWorkflow(ctx, workflow); err != nil {
		c.publishAndAudit(ctx, Event{
			Type:       EventContextCompressFailed,
			WorkflowID: workflow.ID,
			Source:     "workflow-coordinator",
			Payload: map[string]any{
				"skill":  workflow.Skill,
				"status": workflow.Status,
				"error":  err.Error(),
			},
		})
	}
}

func (c *StandardWorkflowCoordinator) nextWorkflowID() string {
	return fmt.Sprintf("wf_%d", atomic.AddUint64(&c.nextID, 1))
}

func (c *StandardWorkflowCoordinator) nextApprovalID() string {
	return fmt.Sprintf("ap_%d", atomic.AddUint64(&c.nextID, 1))
}

func (c *StandardWorkflowCoordinator) nextFilePermID() string {
	return fmt.Sprintf("fp_%d", atomic.AddUint64(&c.nextID, 1))
}

func (c *StandardWorkflowCoordinator) nextMessageID() string {
	return fmt.Sprintf("msg_%d", atomic.AddUint64(&c.nextID, 1))
}

// SetSandbox replaces the sandbox after construction, used to inject the
// permissioned wrapper without a circular dependency.
func (c *StandardWorkflowCoordinator) SetSandbox(s Sandbox) {
	c.sandbox = s
}

// SetAutoApprovalPolicy updates the auto-approval policy at runtime.
// Thread-safe via atomic.Value.
func (c *StandardWorkflowCoordinator) SetAutoApprovalPolicy(p AutoApprovalPolicy) {
	c.autoApproval.Store(&p)
}

// GetAutoApprovalPolicy returns the current auto-approval policy.
func (c *StandardWorkflowCoordinator) GetAutoApprovalPolicy() AutoApprovalPolicy {
	if v := c.autoApproval.Load(); v != nil {
		return *v.(*AutoApprovalPolicy)
	}
	return AutoApprovalPolicy{}
}

// shouldAutoApprove returns true when the auto-approval policy permits
// skipping the human approval modal for the given risk level.
func (c *StandardWorkflowCoordinator) shouldAutoApprove(skillRisk string) bool {
	v := c.autoApproval.Load()
	if v == nil {
		return false
	}
	policy := v.(*AutoApprovalPolicy)
	if !policy.Enabled {
		return false
	}
	return riskLevelOrdinal(skillRisk) <= riskLevelOrdinal(policy.MaxRisk)
}

func riskLevelOrdinal(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	default:
		return 99
	}
}

func (c *StandardWorkflowCoordinator) RequestFileAccess(ctx context.Context, path string, mode FileAccessMode, requester string) (FilePermission, error) {
	now := time.Now().UTC()
	perm := FilePermission{
		ID:        c.nextFilePermID(),
		Path:      path,
		Mode:      mode,
		State:     ApprovalPending,
		Requester: requester,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := c.store.SaveFilePermission(ctx, perm); err != nil {
		return FilePermission{}, err
	}

	c.publishAndAudit(ctx, Event{
		Type:   EventFileAccessRequested,
		Source: "workflow-coordinator",
		Payload: map[string]any{
			"permission_id": perm.ID,
			"path":          perm.Path,
			"mode":          string(perm.Mode),
			"requester":     perm.Requester,
		},
	})

	return perm, nil
}

func (c *StandardWorkflowCoordinator) WaitForDecision(ctx context.Context, permID string) (FilePermission, error) {
	sub := c.bus.Subscribe(16)
	defer c.bus.Unsubscribe(sub.ID)

	for {
		select {
		case <-ctx.Done():
			return FilePermission{}, ctx.Err()
		case event, ok := <-sub.C:
			if !ok {
				return FilePermission{}, fmt.Errorf("event bus closed")
			}
			if event.Type != EventFileAccessApproved && event.Type != EventFileAccessRejected {
				continue
			}
			if payloadPermID, _ := event.Payload["permission_id"].(string); payloadPermID != permID {
				continue
			}
			return c.store.GetFilePermission(ctx, permID)
		}
	}
}

func (c *StandardWorkflowCoordinator) ApproveFileAccess(ctx context.Context, id string) (FilePermission, error) {
	perm, err := c.store.GetFilePermission(ctx, id)
	if err != nil {
		return FilePermission{}, err
	}
	if perm.State != ApprovalPending {
		return perm, fmt.Errorf("file permission %s is %s", id, perm.State)
	}

	perm.State = ApprovalApproved
	perm.UpdatedAt = time.Now().UTC()
	if err := c.store.SaveFilePermission(ctx, perm); err != nil {
		return FilePermission{}, err
	}

	c.publishAndAudit(ctx, Event{
		Type:   EventFileAccessApproved,
		Source: "workflow-coordinator",
		Payload: map[string]any{
			"permission_id": perm.ID,
			"path":          perm.Path,
			"mode":          string(perm.Mode),
		},
	})

	return perm, nil
}

func (c *StandardWorkflowCoordinator) RejectFileAccess(ctx context.Context, id string) (FilePermission, error) {
	perm, err := c.store.GetFilePermission(ctx, id)
	if err != nil {
		return FilePermission{}, err
	}
	if perm.State != ApprovalPending {
		return perm, fmt.Errorf("file permission %s is %s", id, perm.State)
	}

	perm.State = ApprovalRejected
	perm.UpdatedAt = time.Now().UTC()
	if err := c.store.SaveFilePermission(ctx, perm); err != nil {
		return FilePermission{}, err
	}

	c.publishAndAudit(ctx, Event{
		Type:   EventFileAccessRejected,
		Source: "workflow-coordinator",
		Payload: map[string]any{
			"permission_id": perm.ID,
			"path":          perm.Path,
			"mode":          string(perm.Mode),
		},
	})

	return perm, nil
}

func (c *StandardWorkflowCoordinator) ListFilePermissions(ctx context.Context) ([]FilePermission, error) {
	perms, err := c.store.ListFilePermissions(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(perms, func(i, j int) bool {
		return perms[i].CreatedAt.After(perms[j].CreatedAt)
	})
	return perms, nil
}

func (c *StandardWorkflowCoordinator) Channels(_ context.Context) ([]ChannelProviderStatus, error) {
	channels := make([]ChannelProviderStatus, 0, 1)
	if c.whatsApp != nil {
		channels = append(channels, c.whatsApp.Status(c.secrets))
	}
	return channels, nil
}

func (c *StandardWorkflowCoordinator) TestChannel(ctx context.Context, channel string) (ChannelProviderStatus, error) {
	switch channel {
	case "whatsapp":
		if c.whatsApp == nil {
			return ChannelProviderStatus{}, fmt.Errorf("channel %q is not configured", channel)
		}
		return c.whatsApp.Test(ctx, c.secrets)
	default:
		return ChannelProviderStatus{}, fmt.Errorf("unknown channel %q", channel)
	}
}

func (c *StandardWorkflowCoordinator) SubmitMessage(ctx context.Context, req MessageRequest) (MessageSubmitResult, error) {
	if req.Channel == "" {
		req.Channel = "whatsapp"
	}
	if req.Provider == "" {
		req.Provider = "meta_cloud"
	}
	workflow, err := c.SubmitWorkflow(ctx, WorkflowDefinition{
		Name:  firstWorkflowName(req.Name, "Send WhatsApp message"),
		Skill: "whatsapp-message-drafter",
		Input: map[string]any{
			"provider":           req.Provider,
			"to":                 req.To,
			"type":               firstWorkflowName(req.Type, "text"),
			"text":               req.Text,
			"template_name":      req.TemplateName,
			"template_language":  req.TemplateLanguage,
			"template_variables": req.TemplateVariables,
			"test":               req.Test,
		},
	})
	if err != nil {
		return MessageSubmitResult{}, err
	}

	messageID, _ := workflow.Output["message_id"].(string)
	if messageID == "" {
		return MessageSubmitResult{}, fmt.Errorf("workflow %s did not produce a message record", workflow.ID)
	}
	message, err := c.store.GetMessage(ctx, messageID)
	if err != nil {
		return MessageSubmitResult{}, err
	}
	return MessageSubmitResult{Message: message, Workflow: workflow}, nil
}

func (c *StandardWorkflowCoordinator) GetMessage(ctx context.Context, id string) (Message, error) {
	return c.store.GetMessage(ctx, id)
}

func (c *StandardWorkflowCoordinator) ProcessChannelDelivery(ctx context.Context, event ChannelDeliveryEvent) (Message, error) {
	message, err := c.findMessageByEvent(ctx, event)
	if err != nil {
		return Message{}, err
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	message.UpdatedAt = time.Now().UTC()
	message.DeliveryEvents = append(message.DeliveryEvents, event)
	message.ExternalID = firstWorkflowName(message.ExternalID, event.ExternalID)
	message.Status = messageStatusFromDelivery(event.Status)
	if message.Details == nil {
		message.Details = map[string]any{}
	}
	message.Details["last_delivery_status"] = event.Status
	if err := c.store.SaveMessage(ctx, message); err != nil {
		return Message{}, err
	}

	c.publishAndAudit(ctx, Event{
		Type:   EventAdapterExecuted,
		Source: "channel-delivery",
		Payload: map[string]any{
			"adapter":      event.Provider,
			"operation":    "delivery_status",
			"status":       event.Status,
			"message_id":   message.ID,
			"external_id":  event.ExternalID,
			"channel":      event.Channel,
			"recipient":    event.Recipient,
			"delivery_raw": event.Raw,
		},
	})
	return message, nil
}

func (c *StandardWorkflowCoordinator) prepareMessageAction(ctx context.Context, workflow Workflow, action AdapterAction) (AdapterAction, error) {
	if action.Payload == nil {
		action.Payload = map[string]any{}
	}
	if existing, _ := action.Payload["message_id"].(string); existing != "" {
		return action, nil
	}

	recipient := fmt.Sprint(action.Payload["to"])
	if strings.TrimSpace(recipient) == "" {
		recipient = fmt.Sprint(action.Payload["recipient"])
	}
	message := Message{
		ID:               c.nextMessageID(),
		WorkflowID:       workflow.ID,
		Provider:         firstWorkflowName(fmt.Sprint(action.Payload["provider"]), "meta_cloud"),
		Channel:          "whatsapp",
		Direction:        "outbound",
		Recipient:        strings.TrimSpace(recipient),
		Type:             firstWorkflowName(fmt.Sprint(action.Payload["type"]), "text"),
		Text:             strings.TrimSpace(fmt.Sprint(action.Payload["text"])),
		TemplateName:     strings.TrimSpace(fmt.Sprint(action.Payload["template_name"])),
		TemplateLanguage: firstWorkflowName(strings.TrimSpace(fmt.Sprint(action.Payload["template_language"])), "en"),
		Status:           MessageQueued,
		Metadata:         c.secrets.RedactMap(action.Payload),
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	if action.RequiresApproval {
		message.Status = MessagePendingApproval
	}
	if err := c.store.SaveMessage(ctx, message); err != nil {
		return action, err
	}

	action.Payload["message_id"] = message.ID
	action.Payload["workflow_id"] = workflow.ID
	return action, nil
}

func (c *StandardWorkflowCoordinator) buildChannelSendRequest(workflow Workflow, action AdapterAction) ChannelSendRequest {
	req := ChannelSendRequest{
		MessageID:        strings.TrimSpace(fmt.Sprint(action.Payload["message_id"])),
		WorkflowID:       workflow.ID,
		Provider:         strings.TrimSpace(fmt.Sprint(action.Payload["provider"])),
		Channel:          "whatsapp",
		To:               strings.TrimSpace(fmt.Sprint(action.Payload["to"])),
		Type:             firstWorkflowName(strings.TrimSpace(fmt.Sprint(action.Payload["type"])), "text"),
		Text:             strings.TrimSpace(fmt.Sprint(action.Payload["text"])),
		TemplateName:     strings.TrimSpace(fmt.Sprint(action.Payload["template_name"])),
		TemplateLanguage: firstWorkflowName(strings.TrimSpace(fmt.Sprint(action.Payload["template_language"])), "en"),
		Test:             false,
	}
	if value, ok := action.Payload["test"].(bool); ok {
		req.Test = value
	}
	if raw, ok := action.Payload["template_variables"].(map[string]string); ok {
		req.TemplateVariables = raw
	} else if rawAny, ok := action.Payload["template_variables"].(map[string]any); ok {
		req.TemplateVariables = make(map[string]string, len(rawAny))
		for key, value := range rawAny {
			req.TemplateVariables[key] = strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return req
}

func (c *StandardWorkflowCoordinator) updateMessageState(ctx context.Context, payload map[string]any, mutate func(message *Message)) error {
	messageID := strings.TrimSpace(fmt.Sprint(payload["message_id"]))
	if messageID == "" {
		return nil
	}
	message, err := c.store.GetMessage(ctx, messageID)
	if err != nil {
		return err
	}
	mutate(&message)
	message.UpdatedAt = time.Now().UTC()
	return c.store.SaveMessage(ctx, message)
}

func (c *StandardWorkflowCoordinator) findMessageByEvent(ctx context.Context, event ChannelDeliveryEvent) (Message, error) {
	if strings.TrimSpace(event.MessageID) != "" {
		return c.store.GetMessage(ctx, strings.TrimSpace(event.MessageID))
	}

	messages, err := c.store.ListMessages(ctx)
	if err != nil {
		return Message{}, err
	}
	for _, message := range messages {
		if strings.TrimSpace(event.ExternalID) != "" && message.ExternalID == strings.TrimSpace(event.ExternalID) {
			return message, nil
		}
	}
	return Message{}, ErrNotFound
}

func messageStatusFromDelivery(status string) MessageStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "sent":
		return MessageSent
	case "delivered":
		return MessageDelivered
	case "read":
		return MessageRead
	case "failed":
		return MessageFailed
	default:
		return MessageSent
	}
}

func firstWorkflowName(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
