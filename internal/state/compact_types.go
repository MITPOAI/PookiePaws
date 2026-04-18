package state

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/persistence"
)

const (
	schemaWorkflow       = "workflow"
	schemaApproval       = "approval"
	schemaFilePermission = "file_permission"
	schemaMessage        = "message"
	schemaStatus         = "status_snapshot"
	schemaEvent          = "audit_event"
)

func encodeWorkflowCompact(workflow engine.Workflow) ([]byte, error) {
	writer := persistence.NewWriter()
	writer.String(workflow.ID)
	writer.String(workflow.Name)
	writer.String(workflow.Skill)
	writer.Uvarint(uint64(workflowStatusCode(workflow.Status)))
	if err := writeJSONField(writer, workflow.Input); err != nil {
		return nil, err
	}
	if err := writeJSONField(writer, workflow.Output); err != nil {
		return nil, err
	}
	writer.String(workflow.Error)
	writer.Time(workflow.CreatedAt)
	writer.Time(workflow.UpdatedAt)
	payload, err := writer.Finish()
	if err != nil {
		return nil, err
	}
	return persistence.EncodeRecord(schemaWorkflow, 1, payload)
}

func decodeWorkflowCompact(data []byte) (engine.Workflow, error) {
	payload, _, err := persistence.DecodeRecord(data, schemaWorkflow)
	if err != nil {
		return engine.Workflow{}, err
	}
	reader := persistence.NewReader(payload)
	var workflow engine.Workflow
	if workflow.ID, err = reader.String(); err != nil {
		return engine.Workflow{}, err
	}
	if workflow.Name, err = reader.String(); err != nil {
		return engine.Workflow{}, err
	}
	if workflow.Skill, err = reader.String(); err != nil {
		return engine.Workflow{}, err
	}
	statusCode, err := reader.Uvarint()
	if err != nil {
		return engine.Workflow{}, err
	}
	workflow.Status = workflowStatusFromCode(statusCode)
	if err := readJSONField(reader, &workflow.Input); err != nil {
		return engine.Workflow{}, err
	}
	if err := readJSONField(reader, &workflow.Output); err != nil {
		return engine.Workflow{}, err
	}
	if workflow.Error, err = reader.String(); err != nil {
		return engine.Workflow{}, err
	}
	if workflow.CreatedAt, err = reader.Time(); err != nil {
		return engine.Workflow{}, err
	}
	if workflow.UpdatedAt, err = reader.Time(); err != nil {
		return engine.Workflow{}, err
	}
	if reader.Remaining() != 0 {
		return engine.Workflow{}, fmt.Errorf("workflow compact payload has trailing bytes")
	}
	return workflow, nil
}

func encodeApprovalCompact(approval engine.Approval) ([]byte, error) {
	writer := persistence.NewWriter()
	writer.String(approval.ID)
	writer.String(approval.WorkflowID)
	writer.String(approval.Skill)
	writer.String(approval.Adapter)
	writer.String(approval.Action)
	writer.Uvarint(uint64(approvalStateCode(approval.State)))
	if err := writeJSONField(writer, approval.Payload); err != nil {
		return nil, err
	}
	writer.Time(approval.CreatedAt)
	writer.Time(approval.UpdatedAt)
	writer.OptionalTime(approval.ExpiresAt)
	payload, err := writer.Finish()
	if err != nil {
		return nil, err
	}
	return persistence.EncodeRecord(schemaApproval, 1, payload)
}

func decodeApprovalCompact(data []byte) (engine.Approval, error) {
	payload, _, err := persistence.DecodeRecord(data, schemaApproval)
	if err != nil {
		return engine.Approval{}, err
	}
	reader := persistence.NewReader(payload)
	var approval engine.Approval
	if approval.ID, err = reader.String(); err != nil {
		return engine.Approval{}, err
	}
	if approval.WorkflowID, err = reader.String(); err != nil {
		return engine.Approval{}, err
	}
	if approval.Skill, err = reader.String(); err != nil {
		return engine.Approval{}, err
	}
	if approval.Adapter, err = reader.String(); err != nil {
		return engine.Approval{}, err
	}
	if approval.Action, err = reader.String(); err != nil {
		return engine.Approval{}, err
	}
	stateCode, err := reader.Uvarint()
	if err != nil {
		return engine.Approval{}, err
	}
	approval.State = approvalStateFromCode(stateCode)
	if err := readJSONField(reader, &approval.Payload); err != nil {
		return engine.Approval{}, err
	}
	if approval.CreatedAt, err = reader.Time(); err != nil {
		return engine.Approval{}, err
	}
	if approval.UpdatedAt, err = reader.Time(); err != nil {
		return engine.Approval{}, err
	}
	if approval.ExpiresAt, err = reader.OptionalTime(); err != nil {
		return engine.Approval{}, err
	}
	if reader.Remaining() != 0 {
		return engine.Approval{}, fmt.Errorf("approval compact payload has trailing bytes")
	}
	return approval, nil
}

func encodeFilePermissionCompact(perm engine.FilePermission) ([]byte, error) {
	writer := persistence.NewWriter()
	writer.String(perm.ID)
	writer.String(perm.WorkflowID)
	writer.String(perm.Path)
	writer.Uvarint(uint64(fileAccessModeCode(perm.Mode)))
	writer.Uvarint(uint64(approvalStateCode(perm.State)))
	writer.String(perm.Requester)
	writer.Time(perm.CreatedAt)
	writer.Time(perm.UpdatedAt)
	writer.OptionalTime(perm.ExpiresAt)
	payload, err := writer.Finish()
	if err != nil {
		return nil, err
	}
	return persistence.EncodeRecord(schemaFilePermission, 1, payload)
}

func decodeFilePermissionCompact(data []byte) (engine.FilePermission, error) {
	payload, _, err := persistence.DecodeRecord(data, schemaFilePermission)
	if err != nil {
		return engine.FilePermission{}, err
	}
	reader := persistence.NewReader(payload)
	var perm engine.FilePermission
	if perm.ID, err = reader.String(); err != nil {
		return engine.FilePermission{}, err
	}
	if perm.WorkflowID, err = reader.String(); err != nil {
		return engine.FilePermission{}, err
	}
	if perm.Path, err = reader.String(); err != nil {
		return engine.FilePermission{}, err
	}
	modeCode, err := reader.Uvarint()
	if err != nil {
		return engine.FilePermission{}, err
	}
	perm.Mode = fileAccessModeFromCode(modeCode)
	stateCode, err := reader.Uvarint()
	if err != nil {
		return engine.FilePermission{}, err
	}
	perm.State = approvalStateFromCode(stateCode)
	if perm.Requester, err = reader.String(); err != nil {
		return engine.FilePermission{}, err
	}
	if perm.CreatedAt, err = reader.Time(); err != nil {
		return engine.FilePermission{}, err
	}
	if perm.UpdatedAt, err = reader.Time(); err != nil {
		return engine.FilePermission{}, err
	}
	if perm.ExpiresAt, err = reader.OptionalTime(); err != nil {
		return engine.FilePermission{}, err
	}
	if reader.Remaining() != 0 {
		return engine.FilePermission{}, fmt.Errorf("file permission compact payload has trailing bytes")
	}
	return perm, nil
}

func encodeMessageCompact(message engine.Message) ([]byte, error) {
	writer := persistence.NewWriter()
	writer.String(message.ID)
	writer.String(message.WorkflowID)
	writer.String(message.Provider)
	writer.String(message.Channel)
	writer.String(message.Direction)
	writer.String(message.Recipient)
	writer.String(message.Type)
	writer.String(message.Text)
	writer.String(message.TemplateName)
	writer.String(message.TemplateLanguage)
	writer.Uvarint(uint64(messageStatusCode(message.Status)))
	writer.String(message.ExternalID)
	if err := writeJSONField(writer, message.Metadata); err != nil {
		return nil, err
	}
	if err := writeJSONField(writer, message.Details); err != nil {
		return nil, err
	}
	writer.String(message.LastError)
	if err := writeJSONField(writer, message.DeliveryEvents); err != nil {
		return nil, err
	}
	writer.Time(message.CreatedAt)
	writer.Time(message.UpdatedAt)
	payload, err := writer.Finish()
	if err != nil {
		return nil, err
	}
	return persistence.EncodeRecord(schemaMessage, 1, payload)
}

func decodeMessageCompact(data []byte) (engine.Message, error) {
	payload, _, err := persistence.DecodeRecord(data, schemaMessage)
	if err != nil {
		return engine.Message{}, err
	}
	reader := persistence.NewReader(payload)
	var message engine.Message
	if message.ID, err = reader.String(); err != nil {
		return engine.Message{}, err
	}
	if message.WorkflowID, err = reader.String(); err != nil {
		return engine.Message{}, err
	}
	if message.Provider, err = reader.String(); err != nil {
		return engine.Message{}, err
	}
	if message.Channel, err = reader.String(); err != nil {
		return engine.Message{}, err
	}
	if message.Direction, err = reader.String(); err != nil {
		return engine.Message{}, err
	}
	if message.Recipient, err = reader.String(); err != nil {
		return engine.Message{}, err
	}
	if message.Type, err = reader.String(); err != nil {
		return engine.Message{}, err
	}
	if message.Text, err = reader.String(); err != nil {
		return engine.Message{}, err
	}
	if message.TemplateName, err = reader.String(); err != nil {
		return engine.Message{}, err
	}
	if message.TemplateLanguage, err = reader.String(); err != nil {
		return engine.Message{}, err
	}
	statusCode, err := reader.Uvarint()
	if err != nil {
		return engine.Message{}, err
	}
	message.Status = messageStatusFromCode(statusCode)
	if message.ExternalID, err = reader.String(); err != nil {
		return engine.Message{}, err
	}
	if err := readJSONField(reader, &message.Metadata); err != nil {
		return engine.Message{}, err
	}
	if err := readJSONField(reader, &message.Details); err != nil {
		return engine.Message{}, err
	}
	if message.LastError, err = reader.String(); err != nil {
		return engine.Message{}, err
	}
	if err := readJSONField(reader, &message.DeliveryEvents); err != nil {
		return engine.Message{}, err
	}
	if message.CreatedAt, err = reader.Time(); err != nil {
		return engine.Message{}, err
	}
	if message.UpdatedAt, err = reader.Time(); err != nil {
		return engine.Message{}, err
	}
	if reader.Remaining() != 0 {
		return engine.Message{}, fmt.Errorf("message compact payload has trailing bytes")
	}
	return message, nil
}

func encodeStatusCompact(status engine.StatusSnapshot) ([]byte, error) {
	writer := persistence.NewWriter()
	writer.String(status.RuntimeRoot)
	writer.String(status.WorkspaceRoot)
	writer.Uvarint(uint64(status.Workflows))
	writer.Uvarint(uint64(status.PendingApprovals))
	writer.Uvarint(uint64(status.PendingFilePermissions))
	if err := writeJSONField(writer, status.SubTurns); err != nil {
		return nil, err
	}
	writer.Uvarint(status.EventBus.Published)
	writer.Bool(status.EventBus.Closed)
	writer.Uvarint(uint64(status.EventBus.Subscribers))
	if err := writeDroppedMap(writer, status.EventBus.Dropped); err != nil {
		return nil, err
	}
	writer.Time(status.StartedAt)
	payload, err := writer.Finish()
	if err != nil {
		return nil, err
	}
	return persistence.EncodeRecord(schemaStatus, 1, payload)
}

func decodeStatusCompact(data []byte) (engine.StatusSnapshot, error) {
	payload, _, err := persistence.DecodeRecord(data, schemaStatus)
	if err != nil {
		return engine.StatusSnapshot{}, err
	}
	reader := persistence.NewReader(payload)
	var status engine.StatusSnapshot
	if status.RuntimeRoot, err = reader.String(); err != nil {
		return engine.StatusSnapshot{}, err
	}
	if status.WorkspaceRoot, err = reader.String(); err != nil {
		return engine.StatusSnapshot{}, err
	}
	workflows, err := reader.Uvarint()
	if err != nil {
		return engine.StatusSnapshot{}, err
	}
	status.Workflows = int(workflows)
	pendingApprovals, err := reader.Uvarint()
	if err != nil {
		return engine.StatusSnapshot{}, err
	}
	status.PendingApprovals = int(pendingApprovals)
	pendingFilePermissions, err := reader.Uvarint()
	if err != nil {
		return engine.StatusSnapshot{}, err
	}
	status.PendingFilePermissions = int(pendingFilePermissions)
	if err := readJSONField(reader, &status.SubTurns); err != nil {
		return engine.StatusSnapshot{}, err
	}
	if status.EventBus.Published, err = reader.Uvarint(); err != nil {
		return engine.StatusSnapshot{}, err
	}
	if status.EventBus.Closed, err = reader.Bool(); err != nil {
		return engine.StatusSnapshot{}, err
	}
	subscribers, err := reader.Uvarint()
	if err != nil {
		return engine.StatusSnapshot{}, err
	}
	status.EventBus.Subscribers = int(subscribers)
	if status.EventBus.Dropped, err = readDroppedMap(reader); err != nil {
		return engine.StatusSnapshot{}, err
	}
	if status.StartedAt, err = reader.Time(); err != nil {
		return engine.StatusSnapshot{}, err
	}
	if reader.Remaining() != 0 {
		return engine.StatusSnapshot{}, fmt.Errorf("status compact payload has trailing bytes")
	}
	return status, nil
}

func encodeEventCompact(event engine.Event) ([]byte, error) {
	writer := persistence.NewWriter()
	writer.String(event.ID)
	writer.Uvarint(uint64(eventTypeCode(event.Type)))
	writer.Time(event.Time)
	writer.String(event.WorkflowID)
	writer.String(event.Source)
	if err := writeJSONField(writer, event.Payload); err != nil {
		return nil, err
	}
	payload, err := writer.Finish()
	if err != nil {
		return nil, err
	}
	return persistence.EncodeRecord(schemaEvent, 1, payload)
}

func decodeEventCompact(data []byte) (engine.Event, error) {
	payload, _, err := persistence.DecodeRecord(data, schemaEvent)
	if err != nil {
		return engine.Event{}, err
	}
	reader := persistence.NewReader(payload)
	var event engine.Event
	if event.ID, err = reader.String(); err != nil {
		return engine.Event{}, err
	}
	typeCode, err := reader.Uvarint()
	if err != nil {
		return engine.Event{}, err
	}
	event.Type = eventTypeFromCode(typeCode)
	if event.Time, err = reader.Time(); err != nil {
		return engine.Event{}, err
	}
	if event.WorkflowID, err = reader.String(); err != nil {
		return engine.Event{}, err
	}
	if event.Source, err = reader.String(); err != nil {
		return engine.Event{}, err
	}
	if err := readJSONField(reader, &event.Payload); err != nil {
		return engine.Event{}, err
	}
	if reader.Remaining() != 0 {
		return engine.Event{}, fmt.Errorf("event compact payload has trailing bytes")
	}
	return event, nil
}

func writeJSONField(writer *persistence.Writer, value any) error {
	if value == nil {
		writer.Bytes(nil)
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	writer.Bytes(data)
	return nil
}

func readJSONField[T any](reader *persistence.Reader, dest *T) error {
	data, err := reader.Bytes()
	if err != nil {
		return err
	}
	if len(data) == 0 {
		var zero T
		*dest = zero
		return nil
	}
	return json.Unmarshal(data, dest)
}

func writeDroppedMap(writer *persistence.Writer, dropped map[engine.EventType]int) error {
	if len(dropped) == 0 {
		writer.Uvarint(0)
		return nil
	}
	keys := make([]string, 0, len(dropped))
	for key := range dropped {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	writer.Uvarint(uint64(len(keys)))
	for _, key := range keys {
		writer.Uvarint(uint64(eventTypeCode(engine.EventType(key))))
		writer.Uvarint(uint64(dropped[engine.EventType(key)]))
	}
	return nil
}

func readDroppedMap(reader *persistence.Reader) (map[engine.EventType]int, error) {
	count, err := reader.Uvarint()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return map[engine.EventType]int{}, nil
	}
	dropped := make(map[engine.EventType]int, int(count))
	for i := uint64(0); i < count; i++ {
		typeCode, err := reader.Uvarint()
		if err != nil {
			return nil, err
		}
		value, err := reader.Uvarint()
		if err != nil {
			return nil, err
		}
		dropped[eventTypeFromCode(typeCode)] = int(value)
	}
	return dropped, nil
}

func workflowStatusCode(status engine.WorkflowStatus) int {
	switch status {
	case engine.WorkflowQueued:
		return 1
	case engine.WorkflowRunning:
		return 2
	case engine.WorkflowWaitingApproval:
		return 3
	case engine.WorkflowCompleted:
		return 4
	case engine.WorkflowFailed:
		return 5
	case engine.WorkflowRejected:
		return 6
	default:
		return 0
	}
}

func workflowStatusFromCode(code uint64) engine.WorkflowStatus {
	switch code {
	case 1:
		return engine.WorkflowQueued
	case 2:
		return engine.WorkflowRunning
	case 3:
		return engine.WorkflowWaitingApproval
	case 4:
		return engine.WorkflowCompleted
	case 5:
		return engine.WorkflowFailed
	case 6:
		return engine.WorkflowRejected
	default:
		return engine.WorkflowStatus("")
	}
}

func approvalStateCode(state engine.ApprovalState) int {
	switch state {
	case engine.ApprovalPending:
		return 1
	case engine.ApprovalApproved:
		return 2
	case engine.ApprovalRejected:
		return 3
	case engine.ApprovalExpired:
		return 4
	default:
		return 0
	}
}

func approvalStateFromCode(code uint64) engine.ApprovalState {
	switch code {
	case 1:
		return engine.ApprovalPending
	case 2:
		return engine.ApprovalApproved
	case 3:
		return engine.ApprovalRejected
	case 4:
		return engine.ApprovalExpired
	default:
		return engine.ApprovalState("")
	}
}

func fileAccessModeCode(mode engine.FileAccessMode) int {
	switch mode {
	case engine.FileAccessRead:
		return 1
	case engine.FileAccessWrite:
		return 2
	default:
		return 0
	}
}

func fileAccessModeFromCode(code uint64) engine.FileAccessMode {
	switch code {
	case 1:
		return engine.FileAccessRead
	case 2:
		return engine.FileAccessWrite
	default:
		return engine.FileAccessMode("")
	}
}

func messageStatusCode(status engine.MessageStatus) int {
	switch status {
	case engine.MessageQueued:
		return 1
	case engine.MessagePendingApproval:
		return 2
	case engine.MessageApproved:
		return 3
	case engine.MessageSent:
		return 4
	case engine.MessageDelivered:
		return 5
	case engine.MessageRead:
		return 6
	case engine.MessageFailed:
		return 7
	case engine.MessageRejected:
		return 8
	default:
		return 0
	}
}

func messageStatusFromCode(code uint64) engine.MessageStatus {
	switch code {
	case 1:
		return engine.MessageQueued
	case 2:
		return engine.MessagePendingApproval
	case 3:
		return engine.MessageApproved
	case 4:
		return engine.MessageSent
	case 5:
		return engine.MessageDelivered
	case 6:
		return engine.MessageRead
	case 7:
		return engine.MessageFailed
	case 8:
		return engine.MessageRejected
	default:
		return engine.MessageStatus("")
	}
}

func eventTypeCode(eventType engine.EventType) int {
	switch eventType {
	case engine.EventWorkflowSubmitted:
		return 1
	case engine.EventWorkflowUpdated:
		return 2
	case engine.EventSkillCompleted:
		return 3
	case engine.EventApprovalRequired:
		return 4
	case engine.EventAdapterExecuted:
		return 5
	case engine.EventAdapterFailed:
		return 6
	case engine.EventSubTurnStarted:
		return 7
	case engine.EventSubTurnCompleted:
		return 8
	case engine.EventSubTurnOrphaned:
		return 9
	case engine.EventBrainCommand:
		return 10
	case engine.EventBrainCommandError:
		return 11
	case engine.EventContextFlush:
		return 12
	case engine.EventContextCompressFailed:
		return 13
	case engine.EventExecutionBlocked:
		return 14
	case engine.EventFileAccessRequested:
		return 15
	case engine.EventFileAccessApproved:
		return 16
	case engine.EventFileAccessRejected:
		return 17
	case engine.EventFileAccessDenied:
		return 18
	case engine.EventChannelIncoming:
		return 19
	case engine.EventAutoApproved:
		return 20
	default:
		return 0
	}
}

func eventTypeFromCode(code uint64) engine.EventType {
	switch code {
	case 1:
		return engine.EventWorkflowSubmitted
	case 2:
		return engine.EventWorkflowUpdated
	case 3:
		return engine.EventSkillCompleted
	case 4:
		return engine.EventApprovalRequired
	case 5:
		return engine.EventAdapterExecuted
	case 6:
		return engine.EventAdapterFailed
	case 7:
		return engine.EventSubTurnStarted
	case 8:
		return engine.EventSubTurnCompleted
	case 9:
		return engine.EventSubTurnOrphaned
	case 10:
		return engine.EventBrainCommand
	case 11:
		return engine.EventBrainCommandError
	case 12:
		return engine.EventContextFlush
	case 13:
		return engine.EventContextCompressFailed
	case 14:
		return engine.EventExecutionBlocked
	case 15:
		return engine.EventFileAccessRequested
	case 16:
		return engine.EventFileAccessApproved
	case 17:
		return engine.EventFileAccessRejected
	case 18:
		return engine.EventFileAccessDenied
	case 19:
		return engine.EventChannelIncoming
	case 20:
		return engine.EventAutoApproved
	default:
		return engine.EventType("")
	}
}

func normalizeEventTime(event *engine.Event) {
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
}
