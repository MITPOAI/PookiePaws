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
)

type ChatSessionSummary struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
}

type ChatMessage struct {
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

type ChatSession struct {
	ChatSessionSummary
	Messages []ChatMessage `json:"messages"`
}

type ChatPromptRequest struct {
	Prompt string `json:"prompt"`
}

type ChatDispatchResponse struct {
	Session          ChatSessionSummary    `json:"session"`
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
	mu       sync.RWMutex
	sessions map[string]*ChatSession
}

func newChatStore() *chatStore {
	return &chatStore{sessions: map[string]*ChatSession{}}
}

func (s *chatStore) Create() ChatSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	session := &ChatSession{
		ChatSessionSummary: ChatSessionSummary{
			ID:        generateID("chat"),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Messages: []ChatMessage{},
	}
	s.sessions[session.ID] = session
	return cloneChatSession(session)
}

func (s *chatStore) Get(id string) (ChatSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return ChatSession{}, false
	}
	return cloneChatSession(session), true
}

func (s *chatStore) List() []ChatSessionSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summaries := make([]ChatSessionSummary, 0, len(s.sessions))
	for _, session := range s.sessions {
		summary := session.ChatSessionSummary
		summary.MessageCount = len(session.Messages)
		summaries = append(summaries, summary)
	}
	return summaries
}

func (s *chatStore) AppendMessage(sessionID string, message ChatMessage) (ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return ChatSession{}, fmt.Errorf("chat session %s not found", sessionID)
	}
	message.SessionID = sessionID
	session.Messages = append(session.Messages, message)
	session.UpdatedAt = message.CreatedAt
	session.MessageCount = len(session.Messages)
	return cloneChatSession(session), nil
}

func (s *Server) handleChatSessions(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		writeJSON(writer, http.StatusOK, s.chat.List())
	case http.MethodPost:
		session := s.chat.Create()
		writeJSON(writer, http.StatusCreated, session)
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleChatSessionRoutes(writer http.ResponseWriter, request *http.Request) {
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/chat/sessions/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSONError(writer, fmt.Errorf("chat session route not found"), http.StatusNotFound)
		return
	}

	sessionID := parts[0]
	if len(parts) == 1 {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		session, ok := s.chat.Get(sessionID)
		if !ok {
			writeJSONError(writer, fmt.Errorf("chat session %s not found", sessionID), http.StatusNotFound)
			return
		}
		writeJSON(writer, http.StatusOK, session)
		return
	}

	if len(parts) == 2 && parts[1] == "messages" {
		switch request.Method {
		case http.MethodGet:
			session, ok := s.chat.Get(sessionID)
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
		return
	}

	writeJSONError(writer, fmt.Errorf("chat session route not found"), http.StatusNotFound)
}

func (s *Server) processChatPrompt(ctx context.Context, sessionID string, prompt string) (ChatDispatchResponse, error) {
	session, ok := s.chat.Get(sessionID)
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

	now := time.Now().UTC()
	userMessage := ChatMessage{
		ID:        generateID("msg"),
		SessionID: session.ID,
		Role:      "user",
		Kind:      "prompt",
		Content:   prompt,
		CreatedAt: now,
	}
	if _, err := s.chat.AppendMessage(session.ID, userMessage); err != nil {
		return ChatDispatchResponse{}, err
	}

	steps := []ChatStep{
		newChatStep(session.ID, "accepted", "Request accepted", "The control plane received your prompt and is preparing a safe route.", "info", ""),
		newChatStep(session.ID, "routing", "Routing with the brain", "Pookie is translating your goal into observable workflow steps.", "info", ""),
	}

	result, err := s.brain.DispatchPrompt(ctx, prompt)
	if err != nil {
		assistant := ChatMessage{
			ID:        generateID("msg"),
			SessionID: session.ID,
			Role:      "assistant",
			Kind:      "error",
			Content:   err.Error(),
			Status:    "failed",
			CreatedAt: time.Now().UTC(),
		}
		if _, appendErr := s.chat.AppendMessage(session.ID, assistant); appendErr != nil {
			return ChatDispatchResponse{}, appendErr
		}
		steps = append(steps, newChatStep(session.ID, "failed", "Routing paused", err.Error(), "error", ""))
		finalSession, _ := s.chat.Get(session.ID)
		return ChatDispatchResponse{
			Session:          finalSession.ChatSessionSummary,
			UserMessage:      userMessage,
			AssistantMessage: assistant,
			Steps:            steps,
		}, nil
	}

	assistant := buildAssistantChatMessage(session.ID, result)
	if _, err := s.chat.AppendMessage(session.ID, assistant); err != nil {
		return ChatDispatchResponse{}, err
	}

	if result.Blocked != nil {
		steps = append(steps, newChatStep(session.ID, "blocked", "Paused by the police layer", buildBlockedChatDetail(result), "warning", ""))
		if result.Alternative != nil && result.Alternative.Command != nil {
			steps = append(steps, newChatStep(session.ID, "alternative", "Safe alternative prepared", fmt.Sprintf("Pookie suggested %s as a safer next step.", firstNonEmpty(result.Alternative.Command.Name, result.Alternative.Command.Skill, "an alternative workflow")), "info", ""))
		}
	} else {
		workflowID := ""
		if result.Workflow != nil {
			workflowID = result.Workflow.ID
		}
		steps = append(steps, newChatStep(session.ID, "routed", "Workflow routed", fmt.Sprintf("Pookie selected %s for this request.", firstNonEmpty(result.Command.Skill, "the best matching skill")), "info", workflowID))
		if result.Workflow != nil {
			steps = append(steps, newChatStep(session.ID, "queued", "Workflow queued", fmt.Sprintf("%s is now %s.", firstNonEmpty(result.Workflow.Name, "Workflow"), result.Workflow.Status), "info", result.Workflow.ID))
		}
	}

	finalSession, _ := s.chat.Get(session.ID)
	return ChatDispatchResponse{
		Session:          finalSession.ChatSessionSummary,
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

func cloneChatSession(session *ChatSession) ChatSession {
	clone := ChatSession{
		ChatSessionSummary: session.ChatSessionSummary,
		Messages:           append([]ChatMessage(nil), session.Messages...),
	}
	clone.MessageCount = len(clone.Messages)
	return clone
}

func generateID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf)
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
