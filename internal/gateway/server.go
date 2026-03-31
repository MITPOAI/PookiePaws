package gateway

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/brain"
	"github.com/mitpoai/pookiepaws/internal/engine"
)

//go:embed ui/*
var uiFS embed.FS

type Config struct {
	Coordinator engine.WorkflowCoordinator
	EventBus    engine.EventBus
	Brain       PromptDispatcher
	Vault       Vault
	Address     string
}

type PromptDispatcher interface {
	Available() bool
	DispatchPrompt(ctx context.Context, prompt string) (brain.DispatchResult, error)
	Status() brain.Status
}

type Vault interface {
	Get(name string) (string, error)
	Update(values map[string]string) error
}

type WorkflowTemplate struct {
	Name        string         `json:"name"`
	Skill       string         `json:"skill"`
	Description string         `json:"description"`
	Input       map[string]any `json:"input"`
}

type ProviderVaultStatus struct {
	Configured      bool   `json:"configured"`
	Provider        string `json:"provider,omitempty"`
	Mode            string `json:"mode,omitempty"`
	ModelConfigured bool   `json:"model_configured,omitempty"`
}

type IntegrationVaultStatus struct {
	Configured bool `json:"configured"`
}

type VaultStatus struct {
	CanWrite     bool                   `json:"can_write"`
	LoopbackOnly bool                   `json:"loopback_only"`
	Brain        ProviderVaultStatus    `json:"brain"`
	Salesmanago  IntegrationVaultStatus `json:"salesmanago"`
	Mitto        IntegrationVaultStatus `json:"mitto"`
}

type ConsoleSnapshot struct {
	Status    engine.StatusSnapshot    `json:"status"`
	Brain     brain.Status             `json:"brain"`
	Vault     VaultStatus              `json:"vault"`
	Workflows []engine.Workflow        `json:"workflows"`
	Approvals []engine.Approval        `json:"approvals"`
	Skills    []engine.SkillDefinition `json:"skills"`
	Templates []WorkflowTemplate       `json:"templates"`
}

type VaultUpdateRequest struct {
	LLMBaseURL         string `json:"llm_base_url,omitempty"`
	LLMModel           string `json:"llm_model,omitempty"`
	LLMAPIKey          string `json:"llm_api_key,omitempty"`
	SalesmanagoAPIKey  string `json:"salesmanago_api_key,omitempty"`
	SalesmanagoBaseURL string `json:"salesmanago_base_url,omitempty"`
	SalesmanagoOwner   string `json:"salesmanago_owner,omitempty"`
	MittoAPIKey        string `json:"mitto_api_key,omitempty"`
	MittoBaseURL       string `json:"mitto_base_url,omitempty"`
	MittoFrom          string `json:"mitto_from,omitempty"`
}

func (r VaultUpdateRequest) ToMap() map[string]string {
	values := map[string]string{}
	appendIfValue(values, "llm_base_url", r.LLMBaseURL)
	appendIfValue(values, "llm_model", r.LLMModel)
	appendIfValue(values, "llm_api_key", r.LLMAPIKey)
	appendIfValue(values, "salesmanago_api_key", r.SalesmanagoAPIKey)
	appendIfValue(values, "salesmanago_base_url", r.SalesmanagoBaseURL)
	appendIfValue(values, "salesmanago_owner", r.SalesmanagoOwner)
	appendIfValue(values, "mitto_api_key", r.MittoAPIKey)
	appendIfValue(values, "mitto_base_url", r.MittoBaseURL)
	appendIfValue(values, "mitto_from", r.MittoFrom)
	return values
}

type CanvasPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type WorkflowCanvasNode struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Label    string         `json:"label,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
	Position CanvasPosition `json:"position"`
}

type WorkflowCanvasEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type WorkflowPlanRequest struct {
	Goal  string               `json:"goal,omitempty"`
	Nodes []WorkflowCanvasNode `json:"nodes"`
	Edges []WorkflowCanvasEdge `json:"edges,omitempty"`
}

type WorkflowPlanResponse struct {
	Mode             string                     `json:"mode"`
	Summary          string                     `json:"summary"`
	Workflow         *engine.WorkflowDefinition `json:"workflow,omitempty"`
	BrainPrompt      string                     `json:"brain_prompt,omitempty"`
	Warnings         []string                   `json:"warnings,omitempty"`
	NeedsBrain       bool                       `json:"needs_brain"`
	ApprovalRequired bool                       `json:"approval_required"`
}

type AuditEntryView struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	WorkflowID string    `json:"workflow_id,omitempty"`
	Source     string    `json:"source,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	Title      string    `json:"title"`
	Detail     string    `json:"detail"`
	Severity   string    `json:"severity"`
	ApprovalID string    `json:"approval_id,omitempty"`
}

type Server struct {
	coordinator engine.WorkflowCoordinator
	eventBus    engine.EventBus
	brain       PromptDispatcher
	vault       Vault
	address     string
	mux         *http.ServeMux
}

func NewServer(cfg Config) *Server {
	server := &Server{
		coordinator: cfg.Coordinator,
		eventBus:    cfg.EventBus,
		brain:       cfg.Brain,
		vault:       cfg.Vault,
		address:     cfg.Address,
		mux:         http.NewServeMux(),
	}
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	uiAssets, err := fs.Sub(uiFS, "ui")
	if err != nil {
		panic(err)
	}

	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.FS(uiAssets))))
	s.mux.HandleFunc("/api/v1/console", s.handleConsole)
	s.mux.HandleFunc("/api/v1/status", s.handleStatus)
	s.mux.HandleFunc("/api/v1/events", s.handleEvents)
	s.mux.HandleFunc("/api/v1/workflows", s.handleWorkflows)
	s.mux.HandleFunc("/api/v1/workflows/plan", s.handleWorkflowPlan)
	s.mux.HandleFunc("/api/v1/approvals", s.handleApprovals)
	s.mux.HandleFunc("/api/v1/approvals/", s.handleApprovalAction)
	s.mux.HandleFunc("/api/v1/skills", s.handleSkills)
	s.mux.HandleFunc("/api/v1/skills/validate", s.handleValidateSkill)
	s.mux.HandleFunc("/api/v1/brain/dispatch", s.handleBrainDispatch)
	s.mux.HandleFunc("/api/v1/settings/vault", s.handleSettingsVault)
}

func (s *Server) handleIndex(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	data, err := uiFS.ReadFile("ui/index.html")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = writer.Write(data)
}

func (s *Server) handleStatus(writer http.ResponseWriter, request *http.Request) {
	status, err := s.coordinator.Status(request.Context())
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	writeJSON(writer, http.StatusOK, status)
}

func (s *Server) handleConsole(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	status, err := s.coordinator.Status(request.Context())
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	workflows, err := s.coordinator.ListWorkflows(request.Context())
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	approvals, err := s.coordinator.ListApprovals(request.Context())
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}

	brainStatus := brain.Status{
		Enabled:  false,
		Provider: "OpenAI-compatible",
		Mode:     "disabled",
	}
	if s.brain != nil {
		brainStatus = s.brain.Status()
	}

	writeJSON(writer, http.StatusOK, ConsoleSnapshot{
		Status:    status,
		Brain:     brainStatus,
		Vault:     s.currentVaultStatus(),
		Workflows: workflows,
		Approvals: approvals,
		Skills:    s.coordinator.SkillDefinitions(),
		Templates: defaultWorkflowTemplates(),
	})
}

func (s *Server) handleEvents(writer http.ResponseWriter, request *http.Request) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		http.Error(writer, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	subscription := s.eventBus.Subscribe(64)
	defer s.eventBus.Unsubscribe(subscription.ID)

	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("X-Accel-Buffering", "no")

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-request.Context().Done():
			return
		case <-ticker.C:
			_, _ = writer.Write([]byte(": keepalive\n\n"))
			flusher.Flush()
		case event, ok := <-subscription.C:
			if !ok {
				return
			}
			entry := summarizeEvent(event)
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			_, _ = writer.Write([]byte("data: "))
			_, _ = writer.Write(data)
			_, _ = writer.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}

func (s *Server) handleWorkflows(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		workflows, err := s.coordinator.ListWorkflows(request.Context())
		if err != nil {
			writeJSONError(writer, err, http.StatusInternalServerError)
			return
		}
		writeJSON(writer, http.StatusOK, workflows)
	case http.MethodPost:
		var definition engine.WorkflowDefinition
		if err := json.NewDecoder(request.Body).Decode(&definition); err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		workflow, err := s.coordinator.SubmitWorkflow(request.Context(), definition)
		if err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		writeJSON(writer, http.StatusCreated, workflow)
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWorkflowPlan(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload WorkflowPlanRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeJSONError(writer, err, http.StatusBadRequest)
		return
	}

	response, err := buildWorkflowPlan(payload)
	if err != nil {
		writeJSONError(writer, err, http.StatusBadRequest)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (s *Server) handleApprovals(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	approvals, err := s.coordinator.ListApprovals(request.Context())
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	writeJSON(writer, http.StatusOK, approvals)
}

func (s *Server) handleApprovalAction(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(request.URL.Path, "/api/v1/approvals/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		writeJSONError(writer, fmt.Errorf("approval route not found"), http.StatusNotFound)
		return
	}

	id := parts[0]
	action := parts[1]

	switch action {
	case "approve":
		approval, err := s.coordinator.Approve(request.Context(), id)
		if err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		writeJSON(writer, http.StatusOK, approval)
	case "reject":
		approval, err := s.coordinator.Reject(request.Context(), id)
		if err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		writeJSON(writer, http.StatusOK, approval)
	default:
		writer.WriteHeader(http.StatusNotFound)
	}
}

func (s *Server) handleSkills(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(writer, http.StatusOK, s.coordinator.SkillDefinitions())
}

func (s *Server) handleValidateSkill(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Skill string         `json:"skill"`
		Input map[string]any `json:"input"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeJSONError(writer, err, http.StatusBadRequest)
		return
	}

	if err := s.coordinator.ValidateSkill(request.Context(), payload.Skill, payload.Input); err != nil {
		writeJSONError(writer, err, http.StatusBadRequest)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"valid": true})
}

func (s *Server) handleBrainDispatch(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.brain == nil || !s.brain.Available() {
		writeJSONError(writer, fmt.Errorf("brain required: configure an LLM provider in Settings"), http.StatusServiceUnavailable)
		return
	}

	var payload struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeJSONError(writer, err, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.Prompt) == "" {
		writeJSONError(writer, fmt.Errorf("prompt is required"), http.StatusBadRequest)
		return
	}

	result, err := s.brain.DispatchPrompt(request.Context(), payload.Prompt)
	if err != nil {
		writeJSONError(writer, err, http.StatusBadRequest)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) handleSettingsVault(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		writeJSON(writer, http.StatusOK, s.currentVaultStatus())
	case http.MethodPut:
		if s.vault == nil {
			writeJSONError(writer, fmt.Errorf("vault is not configured"), http.StatusServiceUnavailable)
			return
		}
		if !s.isLoopbackBound() {
			writeJSONError(writer, fmt.Errorf("vault writes are allowed only when the server is bound to loopback"), http.StatusForbidden)
			return
		}

		var payload VaultUpdateRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		values := payload.ToMap()
		if len(values) == 0 {
			writeJSONError(writer, fmt.Errorf("at least one non-empty setting is required"), http.StatusBadRequest)
			return
		}
		if err := s.vault.Update(values); err != nil {
			writeJSONError(writer, err, http.StatusInternalServerError)
			return
		}
		writeJSON(writer, http.StatusOK, s.currentVaultStatus())
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) currentVaultStatus() VaultStatus {
	status := VaultStatus{
		CanWrite:     s.isLoopbackBound(),
		LoopbackOnly: true,
		Brain: ProviderVaultStatus{
			Provider: "OpenAI-compatible",
			Mode:     "disabled",
		},
	}

	if s.brain != nil {
		brainStatus := s.brain.Status()
		status.Brain.Configured = brainStatus.Enabled
		status.Brain.Provider = firstNonEmpty(brainStatus.Provider, "OpenAI-compatible")
		status.Brain.Mode = firstNonEmpty(brainStatus.Mode, "disabled")
		status.Brain.ModelConfigured = strings.TrimSpace(brainStatus.Model) != ""
	}

	if s.vault == nil {
		return status
	}

	hasLLMBase := s.hasVaultValue("llm_base_url")
	hasLLMModel := s.hasVaultValue("llm_model")
	if !status.Brain.Configured && hasLLMBase && hasLLMModel {
		status.Brain.Configured = true
		status.Brain.Mode = inferBrainModeFromURL(s.vaultValue("llm_base_url"))
	}
	if !status.Brain.ModelConfigured && hasLLMModel {
		status.Brain.ModelConfigured = true
	}
	status.Salesmanago.Configured = s.hasVaultValue("salesmanago_api_key") && s.hasVaultValue("salesmanago_base_url")
	status.Mitto.Configured = s.hasVaultValue("mitto_api_key") && s.hasVaultValue("mitto_base_url") && s.hasVaultValue("mitto_from")
	return status
}

func (s *Server) hasVaultValue(name string) bool {
	if s.vault == nil {
		return false
	}
	value, err := s.vault.Get(name)
	return err == nil && strings.TrimSpace(value) != ""
}

func (s *Server) vaultValue(name string) string {
	if s.vault == nil {
		return ""
	}
	value, err := s.vault.Get(name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func (s *Server) isLoopbackBound() bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(s.address))
	if err != nil {
		return false
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func buildWorkflowPlan(request WorkflowPlanRequest) (WorkflowPlanResponse, error) {
	if len(request.Nodes) == 0 && strings.TrimSpace(request.Goal) == "" {
		return WorkflowPlanResponse{}, fmt.Errorf("a goal or at least one canvas node is required")
	}

	nodeTypes := make([]string, 0, len(request.Nodes))
	for _, node := range request.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			return WorkflowPlanResponse{}, fmt.Errorf("canvas node id is required")
		}
		nodeType := normalizeNodeType(node.Type)
		if nodeType == "" {
			return WorkflowPlanResponse{}, fmt.Errorf("canvas node type is required")
		}
		switch nodeType {
		case "goal", "research", "compare", "validate", "draft_sms", "approval", "send":
		default:
			return WorkflowPlanResponse{}, fmt.Errorf("unsupported canvas node type %q", node.Type)
		}
		nodeTypes = append(nodeTypes, nodeType)
	}

	hasApproval := containsNodeType(nodeTypes, "approval")
	hasSend := containsNodeType(nodeTypes, "send")
	hasResearch := containsNodeType(nodeTypes, "research")
	hasCompare := containsNodeType(nodeTypes, "compare")
	hasValidate := containsNodeType(nodeTypes, "validate")
	hasDraftSMS := containsNodeType(nodeTypes, "draft_sms")

	if hasSend && !hasApproval {
		return WorkflowPlanResponse{}, fmt.Errorf("send nodes require an approval node before execution")
	}

	response := WorkflowPlanResponse{
		ApprovalRequired: hasApproval || hasSend || hasDraftSMS,
	}

	switch {
	case hasValidate && !hasResearch && !hasCompare && !hasDraftSMS && !hasSend:
		url := firstCanvasValue(request.Nodes, "validate", "url", "https://example.com/?utm_source=meta&utm_medium=paid_social&utm_campaign=launch")
		response.Mode = "workflow"
		response.Summary = "Validate a campaign URL directly through the UTM validator skill."
		response.Workflow = &engine.WorkflowDefinition{
			Name:  "Canvas: Validate campaign UTM",
			Skill: "utm-validator",
			Input: map[string]any{"url": url},
		}
		return response, nil
	case hasDraftSMS && !hasResearch && !hasCompare:
		campaignName := firstCanvasValue(request.Nodes, "draft_sms", "campaign_name", "Canvas SMS draft")
		message := firstCanvasValue(request.Nodes, "draft_sms", "message", "VIP early access is live. Tap to claim your spot.")
		recipient := firstCanvasValue(request.Nodes, "draft_sms", "recipient", "+61400000000")
		response.Mode = "workflow"
		response.Summary = "Draft an approval-gated SMS directly through the Mitto skill."
		response.Workflow = &engine.WorkflowDefinition{
			Name:  "Canvas: Draft launch SMS",
			Skill: "mitto-sms-drafter",
			Input: map[string]any{
				"campaign_name": campaignName,
				"message":       message,
				"recipients":    []string{recipient},
				"test":          true,
			},
		}
		return response, nil
	default:
		goal := strings.TrimSpace(request.Goal)
		if goal == "" {
			goal = buildGoalFromNodes(nodeTypes)
		}
		response.Mode = "brain"
		response.NeedsBrain = true
		response.Summary = "Use the configured brain to translate the canvas into a marketing workflow."
		response.BrainPrompt = buildBrainPrompt(goal, nodeTypes, hasApproval, hasSend)
		if hasResearch || hasCompare {
			response.Warnings = append(response.Warnings, "Research and comparison steps are modeled as agent tasks, not direct local workflows.")
		}
		return response, nil
	}
}

func summarizeEvent(event engine.Event) AuditEntryView {
	entry := AuditEntryView{
		ID:         event.ID,
		Type:       string(event.Type),
		WorkflowID: event.WorkflowID,
		Source:     firstNonEmpty(event.Source, "runtime"),
		Timestamp:  event.Time,
		Severity:   "info",
		Title:      humanizeEventType(event.Type),
		Detail:     "Runtime activity received.",
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	switch event.Type {
	case engine.EventWorkflowSubmitted:
		entry.Title = "Workflow created"
		entry.Detail = fmt.Sprintf("%s queued with %s.", firstNonEmpty(payloadString(event.Payload, "name"), "Workflow"), firstNonEmpty(payloadString(event.Payload, "skill"), "a skill"))
	case engine.EventWorkflowUpdated:
		entry.Title = "Workflow updated"
		entry.Detail = fmt.Sprintf("Workflow moved to %s.", firstNonEmpty(payloadString(event.Payload, "status"), "a new state"))
		if reason := payloadString(event.Payload, "reason"); reason != "" {
			entry.Detail += " Reason: " + reason + "."
		}
		if errText := payloadString(event.Payload, "error"); errText != "" {
			entry.Severity = "error"
			entry.Detail += " Error: " + errText + "."
		}
	case engine.EventSkillCompleted:
		entry.Title = "Skill completed"
		entry.Detail = fmt.Sprintf("%s finished with status %s.", firstNonEmpty(payloadString(event.Payload, "skill"), "Skill"), firstNonEmpty(payloadString(event.Payload, "status"), "completed"))
	case engine.EventApprovalRequired:
		entry.Title = "Approval required"
		entry.Detail = fmt.Sprintf("Waiting for operator approval before %s via %s.", firstNonEmpty(payloadString(event.Payload, "action"), "the outbound action"), firstNonEmpty(payloadString(event.Payload, "adapter"), "the adapter"))
		entry.ApprovalID = payloadString(event.Payload, "approval_id")
		entry.Severity = "warning"
	case engine.EventAdapterExecuted:
		entry.Title = "Adapter executed"
		entry.Detail = fmt.Sprintf("%s %s completed with status %s.", firstNonEmpty(payloadString(event.Payload, "adapter"), "Adapter"), firstNonEmpty(payloadString(event.Payload, "operation"), "operation"), firstNonEmpty(payloadString(event.Payload, "status"), "ok"))
	case engine.EventAdapterFailed:
		entry.Title = "Adapter failed"
		entry.Detail = fmt.Sprintf("%s %s failed: %s.", firstNonEmpty(payloadString(event.Payload, "adapter"), "Adapter"), firstNonEmpty(payloadString(event.Payload, "operation"), "operation"), firstNonEmpty(payloadString(event.Payload, "error"), "unknown error"))
		entry.Severity = "error"
	case engine.EventBrainCommand:
		entry.Title = "Brain routed workflow"
		entry.Detail = fmt.Sprintf("Routing request to %s using %s.", firstNonEmpty(payloadString(event.Payload, "skill"), "the best matching skill"), firstNonEmpty(payloadString(event.Payload, "model"), "the configured model"))
	case engine.EventBrainCommandError:
		entry.Title = "Brain routing failed"
		entry.Detail = fmt.Sprintf("The model response could not be used: %s.", firstNonEmpty(payloadString(event.Payload, "error"), "unknown error"))
		entry.Severity = "error"
	case engine.EventSubTurnStarted:
		entry.Title = "Subtask started"
		entry.Detail = "A workflow subtask is running."
	case engine.EventSubTurnCompleted:
		entry.Title = "Subtask completed"
		entry.Detail = "A workflow subtask completed successfully."
	case engine.EventSubTurnOrphaned:
		entry.Title = "Subtask orphaned"
		entry.Detail = "A subtask finished after its parent workflow stopped waiting."
		entry.Severity = "warning"
	}

	if url := payloadString(event.Payload, "url"); url != "" {
		entry.Detail += " Target: " + url + "."
	}
	return entry
}

func humanizeEventType(eventType engine.EventType) string {
	value := strings.ReplaceAll(string(eventType), ".", " ")
	value = strings.ReplaceAll(value, "_", " ")
	if value == "" {
		return "Event"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch cast := value.(type) {
	case string:
		return strings.TrimSpace(cast)
	case fmt.Stringer:
		return strings.TrimSpace(cast.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func inferBrainModeFromURL(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case value == "":
		return "disabled"
	case strings.Contains(value, "127.0.0.1"), strings.Contains(value, "localhost"), strings.Contains(value, "[::1]"):
		return "local"
	default:
		return "hosted"
	}
}

func appendIfValue(dest map[string]string, key string, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		dest[key] = value
	}
}

func containsNodeType(nodeTypes []string, target string) bool {
	for _, nodeType := range nodeTypes {
		if nodeType == target {
			return true
		}
	}
	return false
}

func normalizeNodeType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func buildGoalFromNodes(nodeTypes []string) string {
	if len(nodeTypes) == 0 {
		return "Run the best matching marketing workflow."
	}

	phrases := make([]string, 0, len(nodeTypes))
	for _, nodeType := range nodeTypes {
		switch nodeType {
		case "goal":
			phrases = append(phrases, "understand the campaign goal")
		case "research":
			phrases = append(phrases, "research competitors online")
		case "compare":
			phrases = append(phrases, "compare findings")
		case "validate":
			phrases = append(phrases, "validate the campaign URL")
		case "draft_sms":
			phrases = append(phrases, "draft the SMS campaign")
		case "approval":
			phrases = append(phrases, "pause for human approval")
		case "send":
			phrases = append(phrases, "prepare the send action")
		}
	}
	if len(phrases) == 0 {
		return "Run the best matching marketing workflow."
	}
	return strings.Join(phrases, ", ")
}

func buildBrainPrompt(goal string, nodeTypes []string, hasApproval bool, hasSend bool) string {
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(goal))
	if len(nodeTypes) > 0 {
		builder.WriteString("\n\nCanvas steps:\n")
		for index, nodeType := range nodeTypes {
			builder.WriteString(fmt.Sprintf("%d. %s\n", index+1, strings.ReplaceAll(nodeType, "_", " ")))
		}
	}
	if hasApproval {
		builder.WriteString("\nRequire human approval before any external send or CRM action.")
	}
	if hasSend {
		builder.WriteString("\nPrepare a send-ready output but do not execute it without approval.")
	}
	return strings.TrimSpace(builder.String())
}

func firstCanvasValue(nodes []WorkflowCanvasNode, nodeType string, key string, fallback string) string {
	for _, node := range nodes {
		if normalizeNodeType(node.Type) != nodeType || node.Config == nil {
			continue
		}
		value := payloadString(node.Config, key)
		if value != "" {
			return value
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func writeJSONError(writer http.ResponseWriter, err error, status int) {
	writeJSON(writer, status, map[string]any{"error": err.Error()})
}

func defaultWorkflowTemplates() []WorkflowTemplate {
	return []WorkflowTemplate{
		{
			Name:        "Validate campaign UTM",
			Skill:       "utm-validator",
			Description: "Check campaign links for missing or malformed UTM parameters.",
			Input: map[string]any{
				"url": "https://example.com/?utm_source=meta&utm_medium=paid_social&utm_campaign=launch",
			},
		},
		{
			Name:        "Route CRM lead",
			Skill:       "salesmanago-lead-router",
			Description: "Queue a lead for the right marketing or sales route with approval before delivery.",
			Input: map[string]any{
				"email":    "lead@example.com",
				"name":     "Taylor Prospect",
				"segment":  "vip",
				"priority": "high",
			},
		},
		{
			Name:        "Draft launch SMS",
			Skill:       "mitto-sms-drafter",
			Description: "Prepare an SMS draft and send intent for operator approval.",
			Input: map[string]any{
				"campaign_name": "April VIP launch",
				"message":       "VIP early access is live. Tap to claim your spot.",
				"recipients":    []string{"+61400000000"},
				"test":          true,
			},
		},
	}
}
