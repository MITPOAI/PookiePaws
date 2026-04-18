package gateway

import (
	"compress/gzip"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mitpoai/pookiepaws/internal/brain"
	"github.com/mitpoai/pookiepaws/internal/demo"
	"github.com/mitpoai/pookiepaws/internal/dossier"
	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/persistence"
	"github.com/mitpoai/pookiepaws/internal/scheduler"
)

//go:embed ui/*
var uiFS embed.FS

type Config struct {
	Coordinator     engine.WorkflowCoordinator
	EventBus        engine.EventBus
	Brain           PromptDispatcher
	Store           engine.StateStore
	Vault           Vault
	WhatsApp        engine.ChannelAdapter
	Dossier         *dossier.Service
	Address         string
	RequestShutdown func()
	// MaxBodyBytes caps incoming request body size. 0 uses the default
	// (1 MiB, matching nginx's client_max_body_size). A negative value
	// disables the cap entirely.
	MaxBodyBytes int64
}

// DefaultMaxBodyBytes is the default cap on inbound request bodies for the
// gateway. Workflow JSON, vault settings, and watchlist apply payloads all
// fit comfortably under 1 MiB.
const DefaultMaxBodyBytes int64 = 1 << 20

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
	CanWrite         bool                   `json:"can_write"`
	LoopbackOnly     bool                   `json:"loopback_only"`
	StorageFormat    string                 `json:"storage_format,omitempty"`
	ResearchProvider string                 `json:"research_provider,omitempty"`
	ResearchSchedule string                 `json:"research_schedule,omitempty"`
	AutonomyPolicy   string                 `json:"autonomy_policy,omitempty"`
	ActionPolicy     string                 `json:"action_policy,omitempty"`
	TrustedDomains   []string               `json:"trusted_domains,omitempty"`
	Brain            ProviderVaultStatus    `json:"brain"`
	Firecrawl        IntegrationVaultStatus `json:"firecrawl"`
	Salesmanago      IntegrationVaultStatus `json:"salesmanago"`
	Mitto            IntegrationVaultStatus `json:"mitto"`
	WhatsApp         IntegrationVaultStatus `json:"whatsapp"`
}

type SchedulerStatus struct {
	Schedule      string     `json:"schedule"`
	LastTickAt    *time.Time `json:"last_tick_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	NextDueAt     *time.Time `json:"next_due_at,omitempty"`
	LastWorkflow  string     `json:"last_workflow,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

type ConsoleSnapshot struct {
	Status            engine.StatusSnapshot          `json:"status"`
	Brain             brain.Status                   `json:"brain"`
	Vault             VaultStatus                    `json:"vault"`
	Scheduler         *SchedulerStatus               `json:"scheduler,omitempty"`
	DemoSmoke         *demo.Result                   `json:"demo_smoke,omitempty"`
	LiveResearchSmoke *demo.Result                   `json:"live_research_smoke,omitempty"`
	Watchlists        []dossier.Watchlist            `json:"watchlists,omitempty"`
	Dossiers          []dossier.Dossier              `json:"dossiers,omitempty"`
	Evidence          []dossier.EvidenceRecord       `json:"evidence,omitempty"`
	Changes           []dossier.ChangeRecord         `json:"changes,omitempty"`
	Recommendations   []dossier.Recommendation       `json:"recommendations,omitempty"`
	Channels          []engine.ChannelProviderStatus `json:"channels"`
	Workflows         []engine.Workflow              `json:"workflows"`
	Approvals         []engine.Approval              `json:"approvals"`
	FilePermissions   []engine.FilePermission        `json:"file_permissions"`
	Skills            []engine.SkillDefinition       `json:"skills"`
	Templates         []WorkflowTemplate             `json:"templates"`
}

type VaultUpdateRequest struct {
	StorageFormat              string `json:"storage_format,omitempty"`
	ResearchProvider           string `json:"research_provider,omitempty"`
	ResearchWatchlists         string `json:"research_watchlists,omitempty"`
	ResearchSchedule           string `json:"research_schedule,omitempty"`
	AutonomyPolicy             string `json:"autonomy_policy,omitempty"`
	TrustedDomains             string `json:"trusted_domains,omitempty"`
	ActionPolicy               string `json:"action_policy,omitempty"`
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
	appendIfValue(values, "storage_format", r.StorageFormat)
	appendIfValue(values, "research_provider", r.ResearchProvider)
	appendIfValue(values, "research_schedule", r.ResearchSchedule)
	appendIfValue(values, "autonomy_policy", r.AutonomyPolicy)
	appendIfValue(values, "trusted_domains", r.TrustedDomains)
	appendIfValue(values, "action_policy", r.ActionPolicy)
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
	Brain    brain.ProviderHealth           `json:"brain"`
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
	AssetVersion string
	ThemeOptions []ThemeOption
}

type Server struct {
	coordinator     engine.WorkflowCoordinator
	eventBus        engine.EventBus
	brain           PromptDispatcher
	vault           Vault
	whatsApp        engine.ChannelAdapter
	dossier         *dossier.Service
	requestShutdown func()
	chat            *chatStore
	address         string
	maxBodyBytes    int64 // 0 = no cap
	mux             *http.ServeMux
	indexTmpl       *template.Template
	indexView       IndexViewModel
	seenIncoming    sync.Map // deduplication for incoming WhatsApp messages
}

func NewServer(cfg Config) *Server {
	indexTmpl, err := template.ParseFS(uiFS, "ui/index.html")
	if err != nil {
		panic(err)
	}

	maxBody := cfg.MaxBodyBytes
	switch {
	case maxBody == 0:
		maxBody = DefaultMaxBodyBytes
	case maxBody < 0:
		maxBody = 0 // explicitly disabled
	}

	server := &Server{
		coordinator:     cfg.Coordinator,
		eventBus:        cfg.EventBus,
		brain:           cfg.Brain,
		vault:           cfg.Vault,
		whatsApp:        cfg.WhatsApp,
		dossier:         cfg.Dossier,
		requestShutdown: cfg.RequestShutdown,
		chat:            newChatStore(cfg.Store),
		address:         cfg.Address,
		maxBodyBytes:    maxBody,
		mux:             http.NewServeMux(),
		indexTmpl:       indexTmpl,
		indexView: IndexViewModel{
			Title:        "PookiePaws Operator Console",
			DefaultTheme: "dark",
			AssetVersion: fmt.Sprintf("%d", time.Now().Unix()),
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
	// Middleware order: outermost wraps innermost. Body cap runs first
	// so oversized payloads are rejected before any handler reads them.
	var handler http.Handler = s.mux
	handler = gzipMiddleware(handler)
	if s.maxBodyBytes > 0 {
		handler = maxBytesMiddleware(s.maxBodyBytes)(handler)
	}
	return handler
}

// maxBytesMiddleware caps the request body using http.MaxBytesReader.
// Handlers that read the body will see an http.MaxBytesError once the
// limit is exceeded; in practice the JSON decoder returns it as a
// decode failure that handlers convert to HTTP 400. Only request bodies
// are limited — SSE/streaming response handlers are unaffected.
func maxBytesMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip compression for SSE streams and WebSocket upgrades — they
		// need unbuffered delivery and http.Flusher access.
		if strings.Contains(r.Header.Get("Accept"), "text/event-stream") ||
			strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
			strings.HasSuffix(r.URL.Path, "/events") ||
			strings.HasSuffix(r.URL.Path, "/ws") {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gz, _ := gzip.NewWriterLevel(w, gzip.DefaultCompression)
		defer gz.Close()

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length")
		next.ServeHTTP(gzipResponseWriter{Writer: gz, ResponseWriter: w}, r)
	})
}

// cacheableFileServer wraps http.FileServer with aggressive Cache-Control headers.
// Static assets are cache-busted via ?v={{ .AssetVersion }} in HTML, so browsers
// will always fetch new versions after a restart. Between restarts, assets are
// served from the browser cache without a round-trip — eliminating re-gzip overhead.
func cacheableFileServer(root http.FileSystem) http.Handler {
	fileServer := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	uiAssets, err := fs.Sub(uiFS, "ui")
	if err != nil {
		panic(err)
	}

	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/favicon.ico", s.handleFavicon)
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/readyz", s.handleReadiness)
	s.mux.Handle("/ui/", http.StripPrefix("/ui/", cacheableFileServer(http.FS(uiAssets))))
	s.mux.HandleFunc("/api/v1/console", s.handleConsole)
	s.mux.HandleFunc("/api/v1/diagnostics", s.handleDiagnostics)
	s.mux.HandleFunc("/api/v1/status", s.handleStatus)
	s.mux.HandleFunc("/api/v1/system/stop", s.handleSystemStop)
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
	s.mux.HandleFunc("/api/v1/sessions", s.handleChatSessions)
	s.mux.HandleFunc("/api/v1/sessions/", s.handleSessionAliasRoutes)
	s.mux.HandleFunc("/api/v1/chat/sessions", s.handleChatSessions)
	s.mux.HandleFunc("/api/v1/chat/sessions/", s.handleChatSessionRoutes)
	s.mux.HandleFunc("/api/v1/chat/ws", s.handleChatWebSocket)
	s.mux.HandleFunc("/api/v1/file-permissions", s.handleFilePermissions)
	s.mux.HandleFunc("/api/v1/file-permissions/", s.handleFilePermissionAction)
	s.mux.HandleFunc("/api/v1/demo/smoke", s.handleDemoSmoke)
	s.mux.HandleFunc("/api/v1/research/watchlists", s.handleResearchWatchlists)
	s.mux.HandleFunc("/api/v1/research/watchlists/refresh", s.handleResearchWatchlistRefresh)
	s.mux.HandleFunc("/api/v1/research/dossiers", s.handleResearchDossiers)
	s.mux.HandleFunc("/api/v1/research/evidence", s.handleResearchEvidence)
	s.mux.HandleFunc("/api/v1/research/changes", s.handleResearchChanges)
	s.mux.HandleFunc("/api/v1/research/recommendations", s.handleResearchRecommendations)
	s.mux.HandleFunc("/api/v1/research/recommendations/", s.handleResearchRecommendationAction)
	s.mux.HandleFunc("/api/v1/settings/vault", s.handleSettingsVault)
	s.mux.HandleFunc("/api/v1/settings/auto-approval", s.handleAutoApproval)
}

func (s *Server) handleIndex(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	if err := s.indexTmpl.ExecuteTemplate(writer, "index.html", s.indexView); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleFavicon(writer http.ResponseWriter, request *http.Request) {
	data, err := uiFS.ReadFile("ui/favicon.ico")
	if err != nil {
		writer.WriteHeader(http.StatusNotFound)
		return
	}
	writer.Header().Set("Content-Type", "image/x-icon")
	writer.Header().Set("Cache-Control", "public, max-age=86400")
	writer.Write(data)
}

func (s *Server) handleStatus(writer http.ResponseWriter, request *http.Request) {
	status, err := s.coordinator.Status(request.Context())
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	writeJSON(writer, http.StatusOK, status)
}

func (s *Server) handleSystemStop(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !isLoopbackClient(request.RemoteAddr) {
		writeJSONError(writer, fmt.Errorf("system stop is available only from loopback clients"), http.StatusForbidden)
		return
	}
	if s.requestShutdown == nil {
		writeJSONError(writer, fmt.Errorf("shutdown is not configured"), http.StatusNotImplemented)
		return
	}

	writeJSON(writer, http.StatusAccepted, map[string]any{
		"status":  "stopping",
		"message": "Shutdown requested. The local server is stopping.",
	})

	go func() {
		time.Sleep(150 * time.Millisecond)
		s.requestShutdown()
	}()
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
	latestDemo, err := demo.LoadLatest(status.RuntimeRoot)
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	latestLive, err := demo.LoadLatestMode(status.RuntimeRoot, "live")
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	researchState, err := s.loadResearchSnapshot(request.Context())
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	schedSnapshot := loadSchedulerSnapshot(status.RuntimeRoot)

	writeJSON(writer, http.StatusOK, ConsoleSnapshot{
		Status:            status,
		Brain:             brainStatus,
		Vault:             s.currentVaultStatus(),
		Scheduler:         schedSnapshot,
		DemoSmoke:         latestDemo,
		LiveResearchSmoke: latestLive,
		Watchlists:        researchState.Watchlists,
		Dossiers:          researchState.Dossiers,
		Evidence:          researchState.Evidence,
		Changes:           researchState.Changes,
		Recommendations:   researchState.Recommendations,
		Channels:          channels,
		Workflows:         workflows,
		Approvals:         approvals,
		FilePermissions:   filePerms,
		Skills:            s.coordinator.SkillDefinitions(),
		Templates:         defaultWorkflowTemplates(),
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
		Brain:    brain.CheckProviderHealth(request.Context(), s.vault),
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
	writer.WriteHeader(http.StatusOK)
	flusher.Flush()

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
			if !s.matchesEventFilter(request.Context(), request.URL.Query().Get("session_id"), request.URL.Query().Get("workflow_id"), event) {
				continue
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

func (s *Server) handleSessionAliasRoutes(writer http.ResponseWriter, request *http.Request) {
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/sessions/")
	s.handleSessionRoutes(writer, request, path)
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

		// Process delivery status events (sent, delivered, read, failed).
		events := s.whatsApp.ParseDeliveryEvents(payload)
		updated := make([]engine.Message, 0, len(events))
		for _, event := range events {
			message, err := s.coordinator.ProcessChannelDelivery(request.Context(), event)
			if err == nil {
				updated = append(updated, message)
			}
		}

		// Process incoming messages and route text to the brain.
		incoming := s.whatsApp.ParseIncomingMessages(payload)
		for _, msg := range incoming {
			_ = s.eventBus.Publish(context.Background(), engine.Event{
				Type:   engine.EventChannelIncoming,
				Source: "whatsapp-webhook",
				Payload: map[string]any{
					"from":       msg.From,
					"from_name":  msg.FromName,
					"type":       msg.Type,
					"message_id": msg.MessageID,
					"channel":    msg.Channel,
				},
			})
			if msg.Type == "text" && msg.Text != "" && s.brain != nil && s.brain.Available() {
				go s.routeIncomingWhatsAppToBrain(msg)
			}
		}

		writeJSON(writer, http.StatusOK, map[string]any{
			"accepted_statuses": len(events),
			"updated_messages":  len(updated),
			"incoming_messages": len(incoming),
		})
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// routeIncomingWhatsAppToBrain dispatches an incoming WhatsApp text message
// through the brain service. The brain picks the best skill and submits a
// workflow, which then follows the normal approval/execution path.
// Runs in a goroutine with a detached context to survive after the webhook
// HTTP response is sent.
func (s *Server) routeIncomingWhatsAppToBrain(msg engine.ChannelIncomingMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Deduplication: skip if we've recently processed this message ID.
	if _, loaded := s.seenIncoming.LoadOrStore(msg.MessageID, time.Now()); loaded {
		return
	}

	result, err := s.brain.DispatchPrompt(ctx, msg.Text)
	if err != nil {
		_ = s.eventBus.Publish(context.Background(), engine.Event{
			Type:   engine.EventBrainCommandError,
			Source: "whatsapp-incoming",
			Payload: map[string]any{
				"from":  msg.From,
				"error": err.Error(),
			},
		})
		return
	}

	payload := map[string]any{
		"from":    msg.From,
		"channel": "whatsapp",
	}
	if result.Workflow != nil {
		payload["workflow_id"] = result.Workflow.ID
		payload["skill"] = result.Workflow.Skill
	}
	if result.Blocked != nil {
		payload["blocked"] = true
		payload["reason"] = result.Blocked.Reason
	}
	_ = s.eventBus.Publish(context.Background(), engine.Event{
		Type:    engine.EventBrainCommand,
		Source:  "whatsapp-incoming",
		Payload: payload,
	})
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

func (s *Server) handleDemoSmoke(writer http.ResponseWriter, request *http.Request) {
	status, err := s.coordinator.Status(request.Context())
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}

	switch request.Method {
	case http.MethodGet:
		mode := firstNonEmpty(strings.TrimSpace(request.URL.Query().Get("mode")), "deterministic")
		result, err := demo.LoadLatestMode(status.RuntimeRoot, mode)
		if err != nil {
			writeJSONError(writer, err, http.StatusInternalServerError)
			return
		}
		if result == nil {
			writeJSON(writer, http.StatusOK, map[string]any{"status": "idle"})
			return
		}
		writeJSON(writer, http.StatusOK, result)
	case http.MethodPost:
		mode := firstNonEmpty(strings.TrimSpace(request.URL.Query().Get("mode")), "deterministic")
		var result demo.Result
		switch mode {
		case "live":
			result, err = demo.RunScenarioLiveSmoke(request.Context(), s.coordinator, status.RuntimeRoot, status.WorkspaceRoot)
		default:
			result, err = demo.RunScenarioSmoke(request.Context(), s.coordinator, status.RuntimeRoot, status.WorkspaceRoot)
		}
		if err != nil {
			writeJSON(writer, http.StatusBadRequest, result)
			return
		}
		writeJSON(writer, http.StatusOK, result)
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleResearchWatchlists(writer http.ResponseWriter, request *http.Request) {
	service, err := s.dossierService()
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}

	switch request.Method {
	case http.MethodGet:
		watchlists, err := service.ListWatchlists(request.Context())
		if err != nil {
			writeJSONError(writer, err, http.StatusInternalServerError)
			return
		}
		writeJSON(writer, http.StatusOK, watchlists)
	case http.MethodPut:
		if !s.isLoopbackBound() {
			writeJSONError(writer, fmt.Errorf("watchlist writes are allowed only from loopback"), http.StatusForbidden)
			return
		}
		var watchlists []dossier.Watchlist
		if err := json.NewDecoder(request.Body).Decode(&watchlists); err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		saved, err := service.SaveWatchlists(request.Context(), watchlists)
		if err != nil {
			writeJSONError(writer, err, http.StatusInternalServerError)
			return
		}
		writeJSON(writer, http.StatusOK, saved)
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleResearchWatchlistRefresh(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload map[string]any
	if request.Body != nil {
		_ = json.NewDecoder(request.Body).Decode(&payload)
	}
	if payload == nil {
		payload = map[string]any{}
	}

	workflow, err := s.coordinator.SubmitWorkflow(request.Context(), engine.WorkflowDefinition{
		Name:  "Watchlist refresh",
		Skill: "mitpo-watchlist-refresh",
		Input: payload,
	})
	if err != nil {
		writeJSONError(writer, err, http.StatusBadRequest)
		return
	}
	_ = s.eventBus.Publish(request.Context(), engine.Event{
		Type:       engine.EventResearchObserved,
		WorkflowID: workflow.ID,
		Source:     "research-watchlists",
		Payload: map[string]any{
			"skill": workflow.Skill,
			"name":  workflow.Name,
		},
	})
	writeJSON(writer, http.StatusCreated, workflow)
}

func (s *Server) handleResearchDossiers(writer http.ResponseWriter, request *http.Request) {
	service, err := s.dossierService()
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}

	switch request.Method {
	case http.MethodGet:
		items, err := service.ListDossiers(request.Context(), parseQueryInt(request.URL.Query().Get("limit"), 12))
		if err != nil {
			writeJSONError(writer, err, http.StatusInternalServerError)
			return
		}
		writeJSON(writer, http.StatusOK, items)
	case http.MethodPost:
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		workflow, err := s.coordinator.SubmitWorkflow(request.Context(), engine.WorkflowDefinition{
			Name:  firstNonEmpty(strings.TrimSpace(fmt.Sprint(payload["name"])), "Generate dossier"),
			Skill: "mitpo-dossier-generate",
			Input: payload,
		})
		if err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		_ = s.eventBus.Publish(request.Context(), engine.Event{
			Type:       engine.EventDossierGenerated,
			WorkflowID: workflow.ID,
			Source:     "research-dossiers",
			Payload: map[string]any{
				"skill": workflow.Skill,
				"name":  workflow.Name,
			},
		})
		writeJSON(writer, http.StatusCreated, workflow)
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleResearchEvidence(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	service, err := s.dossierService()
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	items, err := service.ListEvidence(request.Context(), strings.TrimSpace(request.URL.Query().Get("dossier_id")), parseQueryInt(request.URL.Query().Get("limit"), 24))
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	writeJSON(writer, http.StatusOK, items)
}

func (s *Server) handleResearchChanges(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	service, err := s.dossierService()
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	items, err := service.ListChanges(request.Context(), strings.TrimSpace(request.URL.Query().Get("watchlist_id")), parseQueryInt(request.URL.Query().Get("limit"), 24))
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	writeJSON(writer, http.StatusOK, items)
}

func (s *Server) handleResearchRecommendations(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	service, err := s.dossierService()
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	items, err := service.ListRecommendations(request.Context(), dossier.RecommendationStatus(strings.TrimSpace(request.URL.Query().Get("status"))), parseQueryInt(request.URL.Query().Get("limit"), 24))
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}
	writeJSON(writer, http.StatusOK, items)
}

func (s *Server) handleResearchRecommendationAction(writer http.ResponseWriter, request *http.Request) {
	service, err := s.dossierService()
	if err != nil {
		writeJSONError(writer, err, http.StatusInternalServerError)
		return
	}

	trimmed := strings.TrimPrefix(request.URL.Path, "/api/v1/research/recommendations/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) < 2 {
		writer.WriteHeader(http.StatusNotFound)
		return
	}
	id := strings.TrimSpace(parts[0])
	action := strings.TrimSpace(parts[1])

	switch {
	case request.Method == http.MethodPut && action == "edit":
		var update dossier.RecommendationUpdate
		if err := json.NewDecoder(request.Body).Decode(&update); err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		item, err := service.UpdateRecommendation(request.Context(), id, update)
		if err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		writeJSON(writer, http.StatusOK, item)
	case request.Method == http.MethodPost && action == "queue":
		item, err := service.GetRecommendation(request.Context(), id)
		if err != nil {
			writeJSONError(writer, err, http.StatusNotFound)
			return
		}
		workflow, err := s.coordinator.SubmitWorkflow(request.Context(), item.ProposedWorkflow)
		if err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		item, err = service.MarkRecommendationQueued(request.Context(), id, workflow.ID)
		if err != nil {
			writeJSONError(writer, err, http.StatusInternalServerError)
			return
		}
		_ = s.eventBus.Publish(request.Context(), engine.Event{
			Type:       engine.EventRecommendationQueued,
			WorkflowID: workflow.ID,
			Source:     "research-recommendations",
			Payload: map[string]any{
				"title":             item.Title,
				"recommendation_id": item.ID,
				"skill":             workflow.Skill,
			},
		})
		writeJSON(writer, http.StatusCreated, map[string]any{
			"recommendation": item,
			"workflow":       workflow,
		})
	case request.Method == http.MethodPost && action == "discard":
		item, err := service.DiscardRecommendation(request.Context(), id)
		if err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		_ = s.eventBus.Publish(request.Context(), engine.Event{
			Type:   engine.EventRecommendationDiscarded,
			Source: "research-recommendations",
			Payload: map[string]any{
				"title":             item.Title,
				"recommendation_id": item.ID,
			},
		})
		writeJSON(writer, http.StatusOK, item)
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
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
		// research_watchlists is deprecated as a writable vault field. Watchlists
		// now live under state/research/watchlists/ and are edited via
		// POST /api/v1/research/watchlists or `pookie research watchlists apply`.
		// Empty values remain accepted so that stale form posts continue to succeed.
		// This check runs before the len(values) == 0 guard so payloads that only
		// carry the deprecated key surface the descriptive 400 instead of the
		// generic "at least one non-empty setting is required" error.
		if raw := strings.TrimSpace(payload.ResearchWatchlists); raw != "" {
			writeJSONError(writer, fmt.Errorf("research_watchlists is no longer writable via /api/v1/settings/vault — use POST /api/v1/research/watchlists or `pookie research watchlists apply`"), http.StatusBadRequest)
			return
		}
		values := payload.ToMap()
		if len(values) == 0 {
			writeJSONError(writer, fmt.Errorf("at least one non-empty setting is required"), http.StatusBadRequest)
			return
		}
		if raw := strings.TrimSpace(payload.StorageFormat); raw != "" {
			normalized := persistence.NormalizeFormat(raw)
			if string(normalized) != raw {
				writeJSONError(writer, fmt.Errorf("storage_format must be %q or %q", persistence.FormatJSON, persistence.FormatCompactV1), http.StatusBadRequest)
				return
			}
		}
		if raw := strings.TrimSpace(payload.ResearchProvider); raw != "" {
			if normalizeResearchProviderSetting(raw) == "" {
				writeJSONError(writer, fmt.Errorf("research_provider must be %q, %q, %q, or %q", "internal", "auto", "firecrawl", "jina"), http.StatusBadRequest)
				return
			}
		}
		if raw := strings.TrimSpace(payload.ResearchSchedule); raw != "" {
			if normalizeResearchSchedule(raw) == "" {
				writeJSONError(writer, fmt.Errorf("research_schedule must be %q, %q, or %q", "manual", "hourly", "daily"), http.StatusBadRequest)
				return
			}
		}
		if raw := strings.TrimSpace(payload.AutonomyPolicy); raw != "" {
			if normalizeAutonomyPolicy(raw) == "" {
				writeJSONError(writer, fmt.Errorf("autonomy_policy must be %q", "trusted_ops_v1"), http.StatusBadRequest)
				return
			}
		}
		if raw := strings.TrimSpace(payload.ActionPolicy); raw != "" {
			if normalizeActionPolicy(raw) == "" {
				writeJSONError(writer, fmt.Errorf("action_policy must be %q or %q", "approval_gated", "adapter_rules"), http.StatusBadRequest)
				return
			}
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

func (s *Server) handleAutoApproval(writer http.ResponseWriter, request *http.Request) {
	coord, ok := s.coordinator.(*engine.StandardWorkflowCoordinator)
	if !ok {
		writeJSONError(writer, fmt.Errorf("auto-approval not supported by this coordinator"), http.StatusNotImplemented)
		return
	}

	switch request.Method {
	case http.MethodGet:
		writeJSON(writer, http.StatusOK, coord.GetAutoApprovalPolicy())
	case http.MethodPut:
		var policy engine.AutoApprovalPolicy
		if err := json.NewDecoder(request.Body).Decode(&policy); err != nil {
			writeJSONError(writer, err, http.StatusBadRequest)
			return
		}
		if policy.MaxRisk == "" {
			policy.MaxRisk = "low"
		}
		coord.SetAutoApprovalPolicy(policy)
		writeJSON(writer, http.StatusOK, coord.GetAutoApprovalPolicy())
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) dossierService() (*dossier.Service, error) {
	if s.dossier == nil {
		return nil, fmt.Errorf("dossier service not configured")
	}
	return s.dossier, nil
}

func (s *Server) loadResearchSnapshot(ctx context.Context) (dossier.Snapshot, error) {
	service, err := s.dossierService()
	if err != nil {
		return dossier.Snapshot{}, err
	}
	return service.Snapshot(ctx)
}

func loadSchedulerSnapshot(runtimeRoot string) *SchedulerStatus {
	if runtimeRoot == "" {
		return nil
	}
	store := scheduler.NewStateStore(scheduler.DefaultStatePath(runtimeRoot))
	st, err := store.Load()
	if err != nil {
		return nil
	}
	if st.LastTickAt.IsZero() && st.Schedule == "" {
		return nil
	}
	return &SchedulerStatus{
		Schedule:      st.Schedule,
		LastTickAt:    timePtrOrNil(st.LastTickAt),
		LastSuccessAt: timePtrOrNil(st.LastSuccessAt),
		NextDueAt:     timePtrOrNil(st.NextDueAt),
		LastWorkflow:  st.LastWorkflow,
		LastError:     st.LastError,
	}
}

func timePtrOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func parseQueryInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func (s *Server) currentVaultStatus() VaultStatus {
	status := VaultStatus{
		CanWrite:         s.isLoopbackBound(),
		LoopbackOnly:     true,
		StorageFormat:    string(persistence.FormatCompactV1),
		ResearchProvider: "internal",
		ResearchSchedule: "manual",
		AutonomyPolicy:   "trusted_ops_v1",
		ActionPolicy:     "approval_gated",
		TrustedDomains:   []string{},
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
	if format := persistence.NormalizeFormat(s.vaultValue("storage_format")); format != "" {
		status.StorageFormat = string(format)
	}
	if provider := normalizeResearchProviderSetting(s.vaultValue("research_provider")); provider != "" {
		status.ResearchProvider = provider
	}
	if schedule := normalizeResearchSchedule(s.vaultValue("research_schedule")); schedule != "" {
		status.ResearchSchedule = schedule
	}
	if policy := normalizeAutonomyPolicy(s.vaultValue("autonomy_policy")); policy != "" {
		status.AutonomyPolicy = policy
	}
	if actionPolicy := normalizeActionPolicy(s.vaultValue("action_policy")); actionPolicy != "" {
		status.ActionPolicy = actionPolicy
	}
	status.TrustedDomains = dossier.ParseTrustedDomains(s.vaultValue("trusted_domains"))

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
	status.Firecrawl.Configured = s.hasVaultValue("firecrawl_api_key")
	return status
}

func normalizeResearchProviderSetting(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "internal":
		return "internal"
	case "auto":
		return "auto"
	case "firecrawl":
		return "firecrawl"
	case "jina":
		return "jina"
	default:
		return ""
	}
}

func normalizeResearchSchedule(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "manual":
		return "manual"
	case "hourly":
		return "hourly"
	case "daily":
		return "daily"
	default:
		return ""
	}
}

func normalizeAutonomyPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "trusted_ops_v1":
		return "trusted_ops_v1"
	default:
		return ""
	}
}

func normalizeActionPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "approval_gated":
		return "approval_gated"
	case "adapter_rules":
		return "adapter_rules"
	default:
		return ""
	}
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
	case engine.EventAutoApproved:
		entry.Title = "Auto-approved"
		entry.Detail = fmt.Sprintf("%s via %s was auto-approved by smart sandbox policy (risk: %s).", firstNonEmpty(payloadString(event.Payload, "operation"), "Action"), firstNonEmpty(payloadString(event.Payload, "adapter"), "adapter"), firstNonEmpty(payloadString(event.Payload, "risk"), "low"))
	case engine.EventChannelIncoming:
		entry.Title = "Incoming message"
		entry.Detail = fmt.Sprintf("Received %s message from %s via %s.", firstNonEmpty(payloadString(event.Payload, "type"), "text"), firstNonEmpty(payloadString(event.Payload, "from"), "unknown"), firstNonEmpty(payloadString(event.Payload, "channel"), "channel"))
		entry.Severity = "info"
	case engine.EventResearchObserved:
		entry.Title = "Watchlists refreshed"
		entry.Detail = fmt.Sprintf("%s started the latest watchlist observation cycle.", firstNonEmpty(payloadString(event.Payload, "name"), "Pookie"))
	case engine.EventDossierGenerated:
		entry.Title = "Dossier generated"
		entry.Detail = fmt.Sprintf("%s queued a dossier generation run.", firstNonEmpty(payloadString(event.Payload, "name"), "Research dossier"))
	case engine.EventRecommendationQueued:
		entry.Title = "Recommendation queued"
		entry.Detail = fmt.Sprintf("%s was queued as workflow %s.", firstNonEmpty(payloadString(event.Payload, "title"), "Recommendation"), firstNonEmpty(event.WorkflowID, "pending"))
	case engine.EventRecommendationDiscarded:
		entry.Title = "Recommendation discarded"
		entry.Detail = fmt.Sprintf("%s was dismissed from the operator queue.", firstNonEmpty(payloadString(event.Payload, "title"), "Recommendation"))
		entry.Severity = "warning"
	}

	if url := payloadString(event.Payload, "url"); url != "" {
		entry.Detail += " Target: " + url + "."
	}
	return entry
}

func (s *Server) matchesEventFilter(ctx context.Context, sessionID string, workflowID string, event engine.Event) bool {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID != "" {
		return event.WorkflowID == workflowID
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return true
	}
	if strings.TrimSpace(event.WorkflowID) == "" {
		return false
	}
	session, ok := s.chat.Get(ctx, sessionID)
	if !ok {
		return false
	}
	for _, run := range session.Runs {
		if run.WorkflowID != "" && run.WorkflowID == event.WorkflowID {
			return true
		}
	}
	return false
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

func isLoopbackClient(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
		{
			Name:        "Generate competitor dossier",
			Skill:       "mitpo-dossier-generate",
			Description: "Create a grounded dossier with evidence, changes, and queued recommendations.",
			Input: map[string]any{
				"name":        "OpenClaw core watchlist",
				"topic":       "OpenClaw",
				"company":     "PookiePaws",
				"competitors": []string{"OpenClaw"},
				"domains":     []string{"openclaw.example"},
				"pages":       []string{"https://openclaw.example/pricing"},
				"market":      "AU pet gifting",
				"focus_areas": []string{"pricing", "positioning", "offers"},
			},
		},
		{
			Name:        "Refresh saved watchlists",
			Skill:       "mitpo-watchlist-refresh",
			Description: "Run the autonomous research loop across the configured watchlist set.",
			Input:       map[string]any{},
		},
	}
}
