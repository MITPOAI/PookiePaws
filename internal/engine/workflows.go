package engine

import (
	"context"
	"fmt"
	"sort"
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
	CRMAdapter  CRMAdapter
	SMSAdapter  SMSAdapter
	RuntimeRoot string
	Workspace   string
}

type StandardWorkflowCoordinator struct {
	bus         EventBus
	subturns    SubTurnManager
	store       StateStore
	skills      SkillRegistry
	sandbox     Sandbox
	secrets     SecretProvider
	crmAdapter  CRMAdapter
	smsAdapter  SMSAdapter
	runtimeRoot string
	workspace   string
	startedAt   time.Time
	nextID      uint64
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
		crmAdapter:  cfg.CRMAdapter,
		smsAdapter:  cfg.SMSAdapter,
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
		return workflow, fmt.Errorf(subturnResult.Err)
	}

	workflow.Output = result.Output
	pendingApproval := false

	for _, action := range result.Actions {
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

	c.publishAndAudit(ctx, Event{
		Type:       EventWorkflowUpdated,
		WorkflowID: workflow.ID,
		Source:     "workflow-coordinator",
		Payload: map[string]any{
			"status": workflow.Status,
			"reason": "approval_rejected",
		},
	})
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

func (c *StandardWorkflowCoordinator) nextWorkflowID() string {
	return fmt.Sprintf("wf_%d", atomic.AddUint64(&c.nextID, 1))
}

func (c *StandardWorkflowCoordinator) nextApprovalID() string {
	return fmt.Sprintf("ap_%d", atomic.AddUint64(&c.nextID, 1))
}

func (c *StandardWorkflowCoordinator) nextFilePermID() string {
	return fmt.Sprintf("fp_%d", atomic.AddUint64(&c.nextID, 1))
}

// SetSandbox replaces the sandbox after construction, used to inject the
// permissioned wrapper without a circular dependency.
func (c *StandardWorkflowCoordinator) SetSandbox(s Sandbox) {
	c.sandbox = s
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
