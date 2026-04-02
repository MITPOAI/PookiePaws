package gateway

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
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
	WhatsApp    engine.ChannelAdapter
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
	WhatsApp     IntegrationVaultStatus `json:"whatsapp"`
}

type ConsoleSnapshot struct {
	Status          engine.StatusSnapshot          `json:"status"`
	Brain           brain.Status                   `json:"brain"`
	Vault           VaultStatus                    `json:"vault"`
	Channels        []engine.ChannelProviderStatus `json:"channels"`
	Workflows       []engine.Workflow              `json:"workflows"`
	Approvals       []engine.Approval              `json:"approvals"`
	FilePermissions []engine.FilePermission        `json:"file_permissions"`
	Skills          []engine.SkillDefinition       `json:"skills"`
	Templates       []WorkflowTemplate             `json:"templates"`
}

type VaultUpdateRequest struct {
	LLMBaseURL                 string `json:"llm_base_url,omitempty"`
	LLMModel                   string `json:"llm_model,omitempty"`
	LLMAPIKey                  string `json:"llm_api_key,omitempty"`
	SalesmanagoAPIKey          string `json:"salesmanago_api_key,omitempty"`
	SalesmanagoBaseURL         string `json:"salesmanago_base_url,omitempty"`
	SalesmanagoOwner           string `json:"salesmanago_owner,omitempty"`
	MittoAPIKey                string `json:"mitto_api_key,omitempty"`
	MittoBaseURL               string `json:"mitto_base_url,omitempty"`
	MittoFrom                  string `json:"mitto_from,omitempty"`
	WhatsAppProvider           string `json:"whatsapp_provider,omitempty"`
	WhatsAppAccessToken        string `json:"whatsapp_access_token,omitempty"`
	WhatsAppPhoneNumberID      string `json:"whatsapp_phone_number_id,omitempty"`
	WhatsAppBusinessAccountID  string `json:"whatsapp_business_account_id,omitempty"`
	WhatsAppWebhookVerifyToken string `json:"whatsapp_webhook_verify_token,omitempty"`
	WhatsAppBaseURL            string `json:"whatsapp_base_url,omitempty"`
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
	appendIfValue(values, "whatsapp_provider", r.WhatsAppProvider)
	appendIfValue(values, "whatsapp_access_token", r.WhatsAppAccessToken)
	appendIfValue(values, "whatsapp_phone_number_id", r.WhatsAppPhoneNumberID)
	appendIfValue(values, "whatsapp_business_account_id", r.WhatsAppBusinessAccountID)
	appendIfValue(values, "whatsapp_webhook_verify_token", r.WhatsAppWebhookVerifyToken)
	appendIfValue(values, "whatsapp_base_url", r.WhatsAppBaseURL)
	return values
}

type HealthCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type HealthResponse struct {
	Status    string        `json:"status"`
	Timestamp time.Time     `json:"timestamp"`
	Checks    []HealthCheck `json:"checks"`
}

type DiagnosticsResponse struct {
	Health   HealthResponse                 `json:"health"`
	Status   engine.StatusSnapshot          `json:"status"`
	Vault    VaultStatus                    `json:"vault"`
	Channels []engine.ChannelProviderStatus `json:"channels"`
	Install  map[string]string              `json:"install"`
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

type ThemeOption struct {
	ID    string
	Label string
	Hint  string
}

type IndexViewModel struct {
	Title        string
	DefaultTheme string
	ThemeOptions []ThemeOption
}

type Server struct {
	coordinator engine.WorkflowCoordinator
	eventBus    engine.EventBus
	brain       PromptDispatcher
	vault       Vault
	whatsApp    engine.ChannelAdapter
	chat        *chatStore
	address     string
	mux         *http.ServeMux
	indexTmpl   *template.Template
	indexView   IndexViewModel
}

func NewServer(cfg Config) *Server {
	indexTmpl, err := template.ParseFS(uiFS, "ui/index.html")
	if err != nil {
		panic(err)
	}

	server := &Server{
		coordinator: cfg.Coordinator,
		eventBus:    cfg.EventBus,
		brain:       cfg.Brain,
		vault:       cfg.Vault,
		whatsApp:    cfg.WhatsApp,
		chat:        newChatStore(),
		address:     cfg.Address,
		mux:         http.NewServeMux(),
		indexTmpl:   indexTmpl,
		indexView: IndexViewModel{
			Title:        "PookiePaws Operator Console",
			DefaultTheme: "dark",
			ThemeOptions: []ThemeOption{
				{ID: "light", Label: "Light", Hint: "Bright, crisp, and focused."},
				{ID: "dark", Label: "Dark", Hint: "Low-glare for longer sessions."},
				{ID: "soft", Label: "Pookie Soft", Hint: "Warm, calm, and inviting."},
			},
		},
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
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/readyz", s.handleReadiness)
	s.mux.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.FS(uiAssets))))
	s.mux.HandleFunc("/api/v1/console", s.handleConsole)
	s.mux.HandleFunc("/api/v1/diagnostics", s.handleDiagnostics)
	s.mux.HandleFunc("/api/v1/status", s.handleStatus)
	s.mux.HandleFunc("/api/v1/channels", s.handleChannels)
	s.mux.HandleFunc("/api/v1/channels/status", s.handleChannels)
	s.mux.HandleFunc("/api/v1/channels/whatsapp/test", s.handleWhatsAppTest)
	s.mux.HandleFunc("/api/v1/channels/whatsapp/webhook", s.handleWhatsAppWebhook)
	s.mux.HandleFunc("/api/v1/events", s.handleEvents)
	s.mux.HandleFunc("/api/v1/messages", s.handleMessages)
	s.mux.HandleFunc("/api/v1/messages/", s.handleMessageRoutes)
	s.mux.HandleFunc("/api/v1/workflows", s.handleWorkflows)
	s.mux.HandleFunc("/api/v1/workflows/plan", s.handleWorkflowPlan)
	s.mux.HandleFunc("/api/v1/approvals", s.handleApprovals)
	s.mux.HandleFunc("/api/v1/approvals/", s.handleApprovalAction)
	s.mux.HandleFunc("/api/v1/skills", s.handleSkills)
	s.mux.HandleFunc("/api/v1/skills/validate", s.handleValidateSkill)
	s.mux.HandleFunc("/api/v1/brain/dispatch", s.handleBrainDispatch)
	s.mux.HandleFunc("/api/v1/chat/sessions", s.handleChatSessions)
	s.mux.HandleFunc("/api/v1/chat/sessions/", s.handleChatSessionRoutes)
	s.mux.HandleFunc("/api/v1/chat/ws", s.handleChatWebSocket)
	s.mux.HandleFunc("/api/v1/file-permissions", s.handleFilePermissions)
	s.mux.HandleFunc("/api/v1/file-permissions/", s.handleFilePermissionAction)
	s.mux.HandleFunc("/api/v1/settings/vault", s.handleSettingsVault)
}

func (s *Server) handleIndex(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.indexTmpl.ExecuteTemplate(writer, "index.html", s.indexView); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleStatus(writer http.ResponseWriter, request *http.Request) {
	status, err := s.coordinator.Status(request.Context())
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	writeJSON(writer, http.StatusOK, status)
}

func (s *Server) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(writer, http.StatusOK, s.healthResponse(request.Context(), false))
}

func (s *Server) handleReadiness(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	response := s.healthResponse(request.Context(), true)
	statusCode := http.StatusOK
	if response.Status != "ok" {
		statusCode = http.StatusServiceUnavailable
	}
	writeJSON(writer, statusCode, response)
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

	filePerms, err := s.coordinator.ListFilePermissions(request.Context())
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	channels, err := s.coordinator.Channels(request.Context())
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
		Status:          status,
		Brain:           brainStatus,
		Vault:           s.currentVaultStatus(),
		Channels:        channels,
		Workflows:       workflows,
		Approvals:       approvals,
		FilePermissions: filePerms,
		Skills:          s.coordinator.SkillDefinitions(),
		Templates:       defaultWorkflowTemplates(),
	})
}

func (s *Server) handleDiagnostics(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	status, err := s.coordinator.Status(request.Context())
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	channels, err := s.coordinator.Channels(request.Context())
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}

	writeJSON(writer, http.StatusOK, DiagnosticsResponse{
		Health:   s.healthResponse(request.Context(), false),
		Status:   status,
		Vault:    s.currentVaultStatus(),
		Channels: channels,
		Install: map[string]string{
			"windows": ".\\pookie.exe init && .\\pookie.exe start",
			"macos":   "./pookie init && ./pookie start",
			"linux":   "./pookie init && ./pookie start --addr 0.0.0.0:18800",
		},
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

func (s *Server) handleChannels(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	channels, err := s.coordinator.Channels(request.Context())
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	writeJSON(writer, http.StatusOK, channels)
}

func (s *Server) handleWhatsAppTest(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	status, err := s.coordinator.TestChannel(request.Context(), "whatsapp")
	if err != nil {
		writeJSONError(writer, err, http.StatusBadRequest)
		return
	}
	writeJSON(writer, http.StatusOK, status)
}

func (s *Server) handleWhatsAppWebhook(writer http.ResponseWriter, request *http.Request) {
	if s.whatsApp == nil {
		writeJSONError(writer, fmt.Errorf("whatsapp adapter is not configured"), http.StatusServiceUnavailable)
		return
	}

	switch request.Method {
	case http.MethodGet:
		mode := strings.TrimSpace(request.URL.Query().Get("hub.mode"))
		token := strings.TrimSpace(request.URL.Query().Get("hub.verify_token"))
		challenge := request.URL.Query().Get("hub.challenge")
		expected := s.vaultValue("whatsapp_webhook_verify_token")
		if mode != "subscribe" || expected == "" || token != expected {
			writeJSONError(writer, fmt.Errorf("webhook verification failed"), http.StatusForbidden)
			return
		}
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = writer.Write([]byte(challenge))
	case http.MethodPost:
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		events := s.whatsApp.ParseDeliveryEvents(payload)
		updated := make([]engine.Message, 0, len(events))
		for _, event := range events {
			message, err := s.coordinator.ProcessChannelDelivery(request.Context(), event)
			if err == nil {
				updated = append(updated, message)
			}
		}
		writeJSON(writer, http.StatusOK, map[string]any{
			"accepted": len(events),
			"updated":  len(updated),
		})
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMessages(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodPost:
		var payload engine.MessageRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(payload.Channel) == "" {
			payload.Channel = "whatsapp"
		}
		result, err := s.coordinator.SubmitMessage(request.Context(), payload)
		if err != nil {
			var blocked engine.WorkflowBlockedError
			if errors.As(err, &blocked) {
				writeJSON(writer, http.StatusForbidden, map[string]any{
					"error":    err.Error(),
					"decision": blocked.Decision,
				})
				return
			}
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		writeJSON(writer, http.StatusCreated, result)
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMessageRoutes(writer http.ResponseWriter, request *http.Request) {
	id := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/v1/messages/"), "/")
	if id == "" {
		writeJSONError(writer, fmt.Errorf("message route not found"), http.StatusNotFound)
		return
	}
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	message, err := s.coordinator.GetMessage(request.Context(), id)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, engine.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeJSONError(writer, err, status)
		return
	}
	writeJSON(writer, http.StatusOK, message)
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
			var blocked engine.WorkflowBlockedError
			if errors.As(err, &blocked) {
				writeJSON(writer, http.StatusForbidden, map[string]any{
					"error":    err.Error(),
					"decision": blocked.Decision,
				})
				return
			}
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

func (s *Server) handleFilePermissions(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	perms, err := s.coordinator.ListFilePermissions(request.Context())
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	writeJSON(writer, http.StatusOK, perms)
}

func (s *Server) handleFilePermissionAction(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(request.URL.Path, "/api/v1/file-permissions/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		writeJSONError(writer, fmt.Errorf("file permission route not found"), http.StatusNotFound)
		return
	}

	id := parts[0]
	action := parts[1]

	switch action {
	case "approve":
		perm, err := s.coordinator.ApproveFileAccess(request.Context(), id)
		if err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		writeJSON(writer, http.StatusOK, perm)
	case "reject":
		perm, err := s.coordinator.RejectFileAccess(request.Context(), id)
		if err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		writeJSON(writer, http.StatusOK, perm)
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
	status.WhatsApp.Configured = s.hasVaultValue("whatsapp_access_token") && s.hasVaultValue("whatsapp_phone_number_id")
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
	case engine.EventExecutionBlocked:
		entry.Title = "Police layer blocked workflow"
		entry.Detail = fmt.Sprintf("%s was blocked because %s.", firstNonEmpty(payloadString(event.Payload, "skill"), "A workflow"), firstNonEmpty(payloadString(event.Payload, "reason"), "it violated a security rule"))
		entry.Severity = "warning"
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
	case engine.EventFileAccessRequested:
		entry.Title = "File access requested"
		entry.Detail = fmt.Sprintf("Requesting %s access to %s.", firstNonEmpty(payloadString(event.Payload, "mode"), "file"), firstNonEmpty(payloadString(event.Payload, "path"), "a file"))
		entry.Severity = "warning"
	case engine.EventFileAccessApproved:
		entry.Title = "File access approved"
		entry.Detail = fmt.Sprintf("Approved %s access to %s.", firstNonEmpty(payloadString(event.Payload, "mode"), "file"), firstNonEmpty(payloadString(event.Payload, "path"), "a file"))
	case engine.EventFileAccessRejected:
		entry.Title = "File access rejected"
		entry.Detail = fmt.Sprintf("Rejected %s access to %s.", firstNonEmpty(payloadString(event.Payload, "mode"), "file"), firstNonEmpty(payloadString(event.Payload, "path"), "a file"))
		entry.Severity = "warning"
	case engine.EventFileAccessDenied:
		entry.Title = "File access denied"
		entry.Detail = fmt.Sprintf("Denied %s access to %s: %s.", firstNonEmpty(payloadString(event.Payload, "mode"), "file"), firstNonEmpty(payloadString(event.Payload, "path"), "a file"), firstNonEmpty(payloadString(event.Payload, "reason"), "no reason"))
		entry.Severity = "error"
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

func (s *Server) healthResponse(ctx context.Context, strict bool) HealthResponse {
	checks := []HealthCheck{
		{
			Name:   "http",
			Status: "ok",
			Detail: fmt.Sprintf("Serving on %s.", s.address),
		},
		{
			Name:   "vault",
			Status: map[bool]string{true: "ok", false: "warn"}[s.currentVaultStatus().CanWrite],
			Detail: map[bool]string{true: "Vault writes enabled on loopback.", false: "Vault writes disabled because the server is not loopback-bound."}[s.currentVaultStatus().CanWrite],
		},
	}

	statusCode := "ok"
	if _, err := s.coordinator.Status(ctx); err != nil {
		checks = append(checks, HealthCheck{
			Name:   "runtime",
			Status: "fail",
			Detail: err.Error(),
		})
		statusCode = "degraded"
	} else {
		checks = append(checks, HealthCheck{
			Name:   "runtime",
			Status: "ok",
			Detail: "Coordinator status is readable.",
		})
	}

	for _, channel := range s.channelStatuses(ctx) {
		level := "warn"
		if channel.Configured && channel.Healthy {
			level = "ok"
		}
		checks = append(checks, HealthCheck{
			Name:   channel.Channel,
			Status: level,
			Detail: firstNonEmpty(channel.Message, "No status detail available."),
		})
		if strict && channel.Configured && !channel.Healthy {
			statusCode = "degraded"
		}
	}

	return HealthResponse{
		Status:    statusCode,
		Timestamp: time.Now().UTC(),
		Checks:    checks,
	}
}

func (s *Server) channelStatuses(ctx context.Context) []engine.ChannelProviderStatus {
	channels, err := s.coordinator.Channels(ctx)
	if err != nil {
		return nil
	}
	return channels
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
		{
			Name:        "Send WhatsApp template",
			Skill:       "whatsapp-message-drafter",
			Description: "Prepare a WhatsApp template send for approval before outbound delivery.",
			Input: map[string]any{
				"provider":          "meta_cloud",
				"to":                "+61400000000",
				"type":              "template",
				"template_name":     "launch_update",
				"template_language": "en",
				"template_variables": map[string]string{
					"1": "VIP early access",
					"2": "https://example.com/vip",
				},
				"test": true,
			},
		},
	}
}
