package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mitpoai/pookiepaws/internal/brain"
	"github.com/mitpoai/pookiepaws/internal/engine"
)

type ChatSessionSummary = engine.SessionSummary
type ChatMessage = engine.SessionMessage
type ChatRun = engine.SessionRun
type ChatSession = engine.Session

type ChatStep struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	Stage      string    `json:"stage"`
	Title      string    `json:"title"`
	Detail     string    `json:"detail"`
	Severity   string    `json:"severity"`
	WorkflowID string    `json:"workflow_id,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

type ChatPromptRequest struct {
	Prompt string `json:"prompt"`
}

type ChatDispatchResponse struct {
	Session          ChatSessionSummary    `json:"session"`
	Run              *ChatRun              `json:"run,omitempty"`
	UserMessage      ChatMessage           `json:"user_message"`
	AssistantMessage ChatMessage           `json:"assistant_message"`
	Steps            []ChatStep            `json:"steps"`
	Result           *brain.DispatchResult `json:"result,omitempty"`
}

type ChatSocketEnvelope struct {
	Type    string                `json:"type"`
	Session *ChatSession          `json:"session,omitempty"`
	Message *ChatMessage          `json:"message,omitempty"`
	Step    *ChatStep             `json:"step,omitempty"`
	Audit   *AuditEntryView       `json:"audit,omitempty"`
	Result  *ChatDispatchResponse `json:"result,omitempty"`
	Error   string                `json:"error,omitempty"`
}

type chatStore struct {
	store engine.StateStore

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newChatStore(store engine.StateStore) *chatStore {
	return &chatStore{
		store: store,
		locks: map[string]*sync.Mutex{},
	}
}

func (s *chatStore) Create(ctx context.Context) (ChatSession, error) {
	now := time.Now().UTC()
	session := ChatSession{
		SessionSummary: ChatSessionSummary{
			ID:        generateID("chat"),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Messages: []ChatMessage{},
		Runs:     []ChatRun{},
	}
	if s.store == nil {
		return session, nil
	}
	if err := s.store.SaveSession(ctx, session); err != nil {
		return ChatSession{}, err
	}
	return session, nil
}

func (s *chatStore) Get(ctx context.Context, id string) (ChatSession, bool) {
	if s.store == nil {
		return ChatSession{}, false
	}
	session, err := s.store.GetSession(ctx, id)
	if errors.Is(err, engine.ErrNotFound) {
		return ChatSession{}, false
	}
	if err != nil {
		return ChatSession{}, false
	}
	session.MessageCount = len(session.Messages)
	return session, true
}

func (s *chatStore) List(ctx context.Context) ([]ChatSessionSummary, error) {
	if s.store == nil {
		return nil, nil
	}
	sessions, err := s.store.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]ChatSessionSummary, 0, len(sessions))
	for _, session := range sessions {
		summary := session.SessionSummary
		summary.MessageCount = len(session.Messages)
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func (s *chatStore) AppendMessage(ctx context.Context, sessionID string, message ChatMessage) (ChatSession, error) {
	return s.withSession(ctx, sessionID, func(session *ChatSession) error {
		message.SessionID = sessionID
		session.Messages = append(session.Messages, message)
		session.UpdatedAt = message.CreatedAt
		return nil
	})
}

func (s *chatStore) ReserveRun(ctx context.Context, sessionID string, prompt string) (ChatRun, error) {
	var run ChatRun
	_, err := s.withSession(ctx, sessionID, func(session *ChatSession) error {
		if current := latestActiveRun(session.Runs); current != nil {
			return httpStatusError{status: http.StatusConflict, err: fmt.Errorf("session %s already has an active run %s", sessionID, current.ID)}
		}
		now := time.Now().UTC()
		run = ChatRun{
			ID:         generateID("run"),
			SessionID:  sessionID,
			Prompt:     prompt,
			Status:     engine.SessionAccepted,
			AcceptedAt: now,
		}
		session.Runs = append(session.Runs, run)
		session.LastStatus = run.Status
		session.UpdatedAt = now
		return nil
	})
	return run, err
}

func (s *chatStore) MarkRunRunning(ctx context.Context, sessionID string, runID string) (ChatRun, error) {
	var run ChatRun
	_, err := s.withSession(ctx, sessionID, func(session *ChatSession) error {
		index := sessionRunIndex(session.Runs, runID)
		if index < 0 {
			return engine.ErrNotFound
		}
		session.Runs[index].Status = engine.SessionRunning
		if session.Runs[index].StartedAt.IsZero() {
			session.Runs[index].StartedAt = time.Now().UTC()
		}
		run = session.Runs[index]
		session.LastStatus = run.Status
		session.UpdatedAt = time.Now().UTC()
		return nil
	})
	return run, err
}

func (s *chatStore) CompleteRun(ctx context.Context, sessionID string, runID string, status engine.SessionStatus, workflowID string, skill string, promptTrace *brain.PromptTrace, altTrace *brain.PromptTrace, errText string, technicalErr string) (ChatRun, error) {
	var run ChatRun
	_, err := s.withSession(ctx, sessionID, func(session *ChatSession) error {
		index := sessionRunIndex(session.Runs, runID)
		if index < 0 {
			return engine.ErrNotFound
		}
		session.Runs[index].Status = status
		session.Runs[index].WorkflowID = strings.TrimSpace(workflowID)
		session.Runs[index].Skill = strings.TrimSpace(skill)
		session.Runs[index].Error = strings.TrimSpace(errText)
		session.Runs[index].TechnicalError = strings.TrimSpace(technicalErr)
		session.Runs[index].FinishedAt = time.Now().UTC()
		if session.Runs[index].StartedAt.IsZero() {
			session.Runs[index].StartedAt = session.Runs[index].AcceptedAt
		}
		session.Runs[index].Trace = translateTrace(promptTrace)
		session.Runs[index].AlternativeTrace = translateTrace(altTrace)
		run = session.Runs[index]
		session.LastStatus = run.Status
		session.UpdatedAt = run.FinishedAt
		return nil
	})
	return run, err
}

func (s *chatStore) withSession(ctx context.Context, sessionID string, mutate func(*ChatSession) error) (ChatSession, error) {
	if s.store == nil {
		return ChatSession{}, fmt.Errorf("chat store is not configured")
	}
	lock := s.lockFor(sessionID)
	lock.Lock()
	defer lock.Unlock()

	session, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, engine.ErrNotFound) {
			return ChatSession{}, httpStatusError{status: http.StatusNotFound, err: fmt.Errorf("chat session %s not found", sessionID)}
		}
		return ChatSession{}, err
	}
	if err := mutate(&session); err != nil {
		return ChatSession{}, err
	}
	session.MessageCount = len(session.Messages)
	if err := s.store.SaveSession(ctx, session); err != nil {
		return ChatSession{}, err
	}
	return session, nil
}

func (s *chatStore) lockFor(sessionID string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock := s.locks[sessionID]
	if lock == nil {
		lock = &sync.Mutex{}
		s.locks[sessionID] = lock
	}
	return lock
}

func (s *Server) handleChatSessions(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		sessions, err := s.chat.List(request.Context())
		if err != nil {
			writeJSONError(writer, err, http.StatusInternalServerError)
			return
		}
		writeJSON(writer, http.StatusOK, sessions)
	case http.MethodPost:
		session, err := s.chat.Create(request.Context())
		if err != nil {
			writeJSONError(writer, err, http.StatusInternalServerError)
			return
		}
		writeJSON(writer, http.StatusCreated, session)
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleChatSessionRoutes(writer http.ResponseWriter, request *http.Request) {
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/chat/sessions/")
	s.handleSessionRoutes(writer, request, path)
}

func (s *Server) handleSessionRoutes(writer http.ResponseWriter, request *http.Request, path string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSONError(writer, fmt.Errorf("session route not found"), http.StatusNotFound)
		return
	}

	sessionID := parts[0]
	if len(parts) == 1 {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		session, ok := s.chat.Get(request.Context(), sessionID)
		if !ok {
			writeJSONError(writer, fmt.Errorf("chat session %s not found", sessionID), http.StatusNotFound)
			return
		}
		writeJSON(writer, http.StatusOK, session)
		return
	}

	switch parts[1] {
	case "messages", "history":
		switch request.Method {
		case http.MethodGet:
			session, ok := s.chat.Get(request.Context(), sessionID)
			if !ok {
				writeJSONError(writer, fmt.Errorf("chat session %s not found", sessionID), http.StatusNotFound)
				return
			}
			writeJSON(writer, http.StatusOK, session.Messages)
		case http.MethodPost:
			var payload ChatPromptRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				writeJSONError(writer, err, http.StatusBadRequest)
				return
			}
			response, err := s.processChatPrompt(request.Context(), sessionID, payload.Prompt)
			if err != nil {
				var statusErr interface{ StatusCode() int }
				if errors.As(err, &statusErr) {
					writeJSONError(writer, err, statusErr.StatusCode())
					return
				}
				writeJSONError(writer, err, http.StatusBadRequest)
				return
			}
			writeJSON(writer, http.StatusOK, response)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	case "runs":
		if request.Method == http.MethodGet {
			session, ok := s.chat.Get(request.Context(), sessionID)
			if !ok {
				writeJSONError(writer, fmt.Errorf("chat session %s not found", sessionID), http.StatusNotFound)
				return
			}
			writeJSON(writer, http.StatusOK, session.Runs)
			return
		}
		if request.Method == http.MethodPost {
			var payload ChatPromptRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				writeJSONError(writer, err, http.StatusBadRequest)
				return
			}
			response, err := s.processChatPrompt(request.Context(), sessionID, payload.Prompt)
			if err != nil {
				var statusErr interface{ StatusCode() int }
				if errors.As(err, &statusErr) {
					writeJSONError(writer, err, statusErr.StatusCode())
					return
				}
				writeJSONError(writer, err, http.StatusBadRequest)
				return
			}
			writeJSON(writer, http.StatusOK, response)
			return
		}
		writer.WriteHeader(http.StatusMethodNotAllowed)
	case "status":
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		session, ok := s.chat.Get(request.Context(), sessionID)
		if !ok {
			writeJSONError(writer, fmt.Errorf("chat session %s not found", sessionID), http.StatusNotFound)
			return
		}
		if len(session.Runs) == 0 {
			writeJSON(writer, http.StatusOK, map[string]any{"session_id": session.ID, "status": ""})
			return
		}
		writeJSON(writer, http.StatusOK, session.Runs[len(session.Runs)-1])
	default:
		writeJSONError(writer, fmt.Errorf("session route not found"), http.StatusNotFound)
	}
}

func (s *Server) processChatPrompt(ctx context.Context, sessionID string, prompt string) (ChatDispatchResponse, error) {
	session, ok := s.chat.Get(ctx, sessionID)
	if !ok {
		return ChatDispatchResponse{}, httpStatusError{status: http.StatusNotFound, err: fmt.Errorf("chat session %s not found", sessionID)}
	}
	if s.brain == nil || !s.brain.Available() {
		return ChatDispatchResponse{}, httpStatusError{status: http.StatusServiceUnavailable, err: fmt.Errorf("brain required: configure an LLM provider in Settings")}
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ChatDispatchResponse{}, httpStatusError{status: http.StatusBadRequest, err: fmt.Errorf("prompt is required")}
	}

	run, err := s.chat.ReserveRun(ctx, session.ID, prompt)
	if err != nil {
		return ChatDispatchResponse{}, err
	}

	now := time.Now().UTC()
	userMessage := ChatMessage{
		ID:        generateID("msg"),
		SessionID: session.ID,
		Role:      "user",
		Kind:      "prompt",
		Content:   prompt,
		CreatedAt: now,
	}
	if _, err := s.chat.AppendMessage(ctx, session.ID, userMessage); err != nil {
		return ChatDispatchResponse{}, err
	}
	if run, err = s.chat.MarkRunRunning(ctx, session.ID, run.ID); err != nil {
		return ChatDispatchResponse{}, err
	}

	steps := []ChatStep{
		newChatStep(session.ID, "accepted", "Request accepted", "The control plane received your prompt and is preparing a safe route.", "info", ""),
		newChatStep(session.ID, "routing", "Routing with the brain", "Pookie is translating your goal into observable workflow steps.", "info", ""),
	}

	result, err := s.brain.DispatchPrompt(ctx, prompt)
	if err != nil {
		technicalErr := technicalDispatchError(err)
		trace := ensurePromptTrace(nil, prompt, brainModelStatus(s.brain), "")
		trace.Error = technicalErr
		assistant := ChatMessage{
			ID:        generateID("msg"),
			SessionID: session.ID,
			Role:      "assistant",
			Kind:      "error",
			Content:   err.Error(),
			Status:    "failed",
			CreatedAt: time.Now().UTC(),
		}
		if _, appendErr := s.chat.AppendMessage(ctx, session.ID, assistant); appendErr != nil {
			return ChatDispatchResponse{}, appendErr
		}
		run, _ = s.chat.CompleteRun(ctx, session.ID, run.ID, engine.SessionFailed, "", "", trace, nil, err.Error(), technicalErr)
		steps = append(steps, newChatStep(session.ID, "failed", "Routing paused", technicalErr, "error", ""))
		finalSession, _ := s.chat.Get(ctx, session.ID)
		return ChatDispatchResponse{
			Session:          finalSession.SessionSummary,
			Run:              &run,
			UserMessage:      userMessage,
			AssistantMessage: assistant,
			Steps:            steps,
		}, nil
	}
	result.PromptTrace = ensurePromptTrace(result.PromptTrace, prompt, result.Model, result.Raw)

	assistant := buildAssistantChatMessage(session.ID, result)
	if _, err := s.chat.AppendMessage(ctx, session.ID, assistant); err != nil {
		return ChatDispatchResponse{}, err
	}

	runStatus := engine.SessionCompleted
	runWorkflowID := ""
	if result.Workflow != nil {
		runWorkflowID = result.Workflow.ID
	}

	if result.Blocked != nil {
		runStatus = engine.SessionBlocked
		steps = append(steps, newChatStep(session.ID, "blocked", "Paused by the policy layer", buildBlockedChatDetail(result), "warning", ""))
		if result.Alternative != nil && result.Alternative.Command != nil {
			steps = append(steps, newChatStep(session.ID, "alternative", "Safe alternative prepared", fmt.Sprintf("Pookie suggested %s as a safer next step.", firstNonEmpty(result.Alternative.Command.Name, result.Alternative.Command.Skill, "an alternative workflow")), "info", ""))
		}
	} else {
		steps = append(steps, newChatStep(session.ID, "routed", "Workflow routed", fmt.Sprintf("Pookie selected %s for this request.", firstNonEmpty(result.Command.Skill, "the best matching skill")), "info", runWorkflowID))
		if result.Workflow != nil {
			steps = append(steps, newChatStep(session.ID, "queued", "Workflow queued", fmt.Sprintf("%s is now %s.", firstNonEmpty(result.Workflow.Name, "Workflow"), result.Workflow.Status), "info", result.Workflow.ID))
			if result.Workflow.Status == engine.WorkflowWaitingApproval {
				runStatus = engine.SessionAwaitingApproval
			}
		}
	}

	run, err = s.chat.CompleteRun(ctx, session.ID, run.ID, runStatus, runWorkflowID, result.Command.Skill, result.PromptTrace, result.AltTrace, "", "")
	if err != nil {
		return ChatDispatchResponse{}, err
	}

	finalSession, _ := s.chat.Get(ctx, session.ID)
	return ChatDispatchResponse{
		Session:          finalSession.SessionSummary,
		Run:              &run,
		UserMessage:      userMessage,
		AssistantMessage: assistant,
		Steps:            steps,
		Result:           &result,
	}, nil
}

func buildAssistantChatMessage(sessionID string, result brain.DispatchResult) ChatMessage {
	content := "I prepared the next step."
	kind := "assistant"
	status := "completed"
	workflowID := ""

	switch {
	case result.Blocked != nil:
		kind = "blocked"
		status = "blocked"
		content = buildBlockedChatDetail(result)
		if result.Alternative != nil && strings.TrimSpace(result.Alternative.Message) != "" {
			content = strings.TrimSpace(result.Alternative.Message)
		}
	case result.Workflow != nil:
		workflowID = result.Workflow.ID
		content = fmt.Sprintf("I routed this into %s using %s. You can watch the steps continue in the audit trail.", firstNonEmpty(result.Workflow.Name, "a workflow"), firstNonEmpty(result.Command.Skill, "the selected skill"))
	default:
		content = fmt.Sprintf("I routed this into %s.", firstNonEmpty(result.Command.Skill, "the selected skill"))
	}

	return ChatMessage{
		ID:         generateID("msg"),
		SessionID:  sessionID,
		Role:       "assistant",
		Kind:       kind,
		Content:    content,
		Status:     status,
		WorkflowID: workflowID,
		Model:      result.Model,
		Skill:      result.Command.Skill,
		CreatedAt:  time.Now().UTC(),
	}
}

func buildBlockedChatDetail(result brain.DispatchResult) string {
	if result.Blocked == nil {
		return "The request was paused."
	}
	reason := firstNonEmpty(result.Blocked.Reason, result.Blocked.Violation, "it crossed a security rule")
	return fmt.Sprintf("I paused that request because %s. Nothing was sent or changed.", reason)
}

func newChatStep(sessionID string, stage string, title string, detail string, severity string, workflowID string) ChatStep {
	return ChatStep{
		ID:         generateID("step"),
		SessionID:  sessionID,
		Stage:      stage,
		Title:      title,
		Detail:     detail,
		Severity:   severity,
		WorkflowID: workflowID,
		Timestamp:  time.Now().UTC(),
	}
}

func generateID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

func latestActiveRun(runs []ChatRun) *ChatRun {
	for index := len(runs) - 1; index >= 0; index-- {
		switch runs[index].Status {
		case engine.SessionAccepted, engine.SessionRunning:
			return &runs[index]
		}
	}
	return nil
}

func sessionRunIndex(runs []ChatRun, runID string) int {
	for index := range runs {
		if runs[index].ID == runID {
			return index
		}
	}
	return -1
}

func translateTrace(trace *brain.PromptTrace) *engine.SessionPromptTrace {
	if trace == nil {
		return nil
	}
	return &engine.SessionPromptTrace{
		Mode:         string(trace.Mode),
		SystemPrompt: trace.SystemPrompt,
		UserPrompt:   trace.UserPrompt,
		Model:        trace.Model,
		RawResponse:  trace.RawResponse,
		Error:        trace.Error,
		CreatedAt:    time.Now().UTC(),
	}
}

func ensurePromptTrace(trace *brain.PromptTrace, prompt string, model string, raw string) *brain.PromptTrace {
	if trace == nil {
		return &brain.PromptTrace{
			Mode:        brain.PromptModeOperator,
			UserPrompt:  strings.TrimSpace(prompt),
			Model:       strings.TrimSpace(model),
			RawResponse: strings.TrimSpace(raw),
		}
	}
	if strings.TrimSpace(trace.UserPrompt) == "" {
		trace.UserPrompt = strings.TrimSpace(prompt)
	}
	if strings.TrimSpace(trace.Model) == "" {
		trace.Model = strings.TrimSpace(model)
	}
	if strings.TrimSpace(trace.RawResponse) == "" {
		trace.RawResponse = strings.TrimSpace(raw)
	}
	if trace.Mode == "" {
		trace.Mode = brain.PromptModeOperator
	}
	return trace
}

func technicalDispatchError(err error) string {
	var friendly brain.FriendlyError
	if errors.As(err, &friendly) && friendly.Technical != nil {
		return strings.TrimSpace(friendly.Technical.Error())
	}
	return strings.TrimSpace(err.Error())
}

func brainModelStatus(dispatcher PromptDispatcher) string {
	if dispatcher == nil {
		return ""
	}
	return strings.TrimSpace(dispatcher.Status().Model)
}

type httpStatusError struct {
	status int
	err    error
}

func (e httpStatusError) Error() string {
	if e.err == nil {
		return http.StatusText(e.status)
	}
	return e.err.Error()
}

func (e httpStatusError) Unwrap() error {
	return e.err
}

func (e httpStatusError) StatusCode() int {
	return e.status
}
