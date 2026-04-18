package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/conv"
	"github.com/mitpoai/pookiepaws/internal/dossier"
	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/research"
)

type UTMValidatorSkill struct {
	def engine.SkillDefinition
}

func NewUTMValidatorSkill(manifest Manifest) *UTMValidatorSkill {
	return &UTMValidatorSkill{def: manifest.toDefinition()}
}

func (s *UTMValidatorSkill) Definition() engine.SkillDefinition { return s.def }

func (s *UTMValidatorSkill) Validate(input map[string]any) error {
	rawURL, ok := input["url"].(string)
	if !ok || strings.TrimSpace(rawURL) == "" {
		return fmt.Errorf("utm-validator requires a non-empty url")
	}
	_, err := url.Parse(rawURL)
	return err
}

func (s *UTMValidatorSkill) Execute(_ context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	rawURL := req.Input["url"].(string)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return engine.SkillResult{}, err
	}

	query := parsed.Query()
	required := []string{"utm_source", "utm_medium", "utm_campaign"}
	issues := make([]string, 0)

	normalized := url.Values{}
	for key, values := range query {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		for _, value := range values {
			normalized.Add(lowerKey, strings.TrimSpace(value))
		}
	}

	for _, key := range required {
		if strings.TrimSpace(normalized.Get(key)) == "" {
			issues = append(issues, "missing "+key)
		}
	}

	sortedKeys := make([]string, 0, len(normalized))
	for key := range normalized {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)
	sorted := url.Values{}
	for _, key := range sortedKeys {
		sorted[key] = normalized[key]
	}
	parsed.RawQuery = sorted.Encode()

	return engine.SkillResult{
		Output: map[string]any{
			"url":            rawURL,
			"normalized_url": parsed.String(),
			"issues":         issues,
			"valid":          len(issues) == 0,
			"params":         sorted,
		},
	}, nil
}

type SalesmanagoLeadRouterSkill struct {
	def engine.SkillDefinition
}

func NewSalesmanagoLeadRouterSkill(manifest Manifest) *SalesmanagoLeadRouterSkill {
	return &SalesmanagoLeadRouterSkill{def: manifest.toDefinition()}
}

func (s *SalesmanagoLeadRouterSkill) Definition() engine.SkillDefinition { return s.def }

func (s *SalesmanagoLeadRouterSkill) Validate(input map[string]any) error {
	identifier := strings.TrimSpace(conv.AsString(input["email"]))
	if identifier == "" {
		identifier = strings.TrimSpace(conv.AsString(input["contact_id"]))
	}
	if identifier == "" {
		identifier = strings.TrimSpace(conv.AsString(input["lead_id"]))
	}
	if identifier == "" {
		return fmt.Errorf("salesmanago-lead-router requires email, contact_id, or lead_id")
	}
	if strings.TrimSpace(conv.AsString(input["segment"])) == "" {
		return fmt.Errorf("salesmanago-lead-router requires segment")
	}
	return nil
}

func (s *SalesmanagoLeadRouterSkill) Execute(_ context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	email := strings.TrimSpace(conv.AsString(req.Input["email"]))
	contactID := strings.TrimSpace(conv.AsString(req.Input["contact_id"]))
	leadID := strings.TrimSpace(conv.AsString(req.Input["lead_id"]))
	segment := strings.ToLower(conv.AsString(req.Input["segment"]))
	priority := strings.ToLower(conv.AsString(req.Input["priority"]))
	name := strings.TrimSpace(conv.AsString(req.Input["name"]))
	phone := strings.TrimSpace(conv.AsString(req.Input["phone"]))

	queue := "nurture-default"
	switch {
	case priority == "high":
		queue = "priority-sales"
	case strings.Contains(segment, "vip"):
		queue = "vip-success"
	case strings.Contains(segment, "trial"):
		queue = "trial-conversion"
	}

	payload := map[string]any{
		"email":       email,
		"contact_id":  contactID,
		"lead_id":     leadID,
		"name":        name,
		"phone":       phone,
		"segment":     segment,
		"priority":    priority,
		"route_queue": queue,
	}

	return engine.SkillResult{
		Output: map[string]any{
			"email":       email,
			"contact_id":  contactID,
			"lead_id":     leadID,
			"segment":     segment,
			"priority":    priority,
			"route_queue": queue,
		},
		Actions: []engine.AdapterAction{{
			Adapter:          "salesmanago",
			Operation:        "route_lead",
			Payload:          payload,
			RequiresApproval: true,
		}},
	}, nil
}

type MittoSMSDrafterSkill struct {
	def engine.SkillDefinition
}

func NewMittoSMSDrafterSkill(manifest Manifest) *MittoSMSDrafterSkill {
	return &MittoSMSDrafterSkill{def: manifest.toDefinition()}
}

func (s *MittoSMSDrafterSkill) Definition() engine.SkillDefinition { return s.def }

func (s *MittoSMSDrafterSkill) Validate(input map[string]any) error {
	if strings.TrimSpace(conv.AsString(input["message"])) == "" {
		return fmt.Errorf("mitto-sms-drafter requires message")
	}
	recipients := conv.AsStringSlice(input["recipients"])
	if len(recipients) == 0 {
		return fmt.Errorf("mitto-sms-drafter requires at least one recipient")
	}
	return nil
}

func (s *MittoSMSDrafterSkill) Execute(_ context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	message := strings.TrimSpace(conv.AsString(req.Input["message"]))
	recipients := conv.AsStringSlice(req.Input["recipients"])
	campaign := conv.AsString(req.Input["campaign_name"])
	if campaign == "" {
		campaign = "untitled-campaign"
	}

	issues := make([]string, 0)
	if len(message) > 160 {
		issues = append(issues, "message exceeds 160 characters")
	}

	payload := map[string]any{
		"campaign_name": campaign,
		"message":       message,
		"recipients":    recipients,
		"from":          conv.AsString(req.Input["from"]),
		"test":          conv.AsBool(req.Input["test"]),
	}

	return engine.SkillResult{
		Output: map[string]any{
			"campaign_name":   campaign,
			"message":         message,
			"recipients":      recipients,
			"issues":          issues,
			"recipient_count": len(recipients),
		},
		Actions: []engine.AdapterAction{{
			Adapter:          "mitto",
			Operation:        "send_sms",
			Payload:          payload,
			RequiresApproval: true,
		}},
	}, nil
}

func (m Manifest) toDefinition() engine.SkillDefinition {
	return engine.SkillDefinition{
		Name:        m.Name,
		Description: m.Description,
		Tools:       m.Tools,
		Events:      m.Events,
		Prompt:      m.Prompt,
	}
}

type WhatsAppMessageDrafterSkill struct {
	def engine.SkillDefinition
}

func NewWhatsAppMessageDrafterSkill(manifest Manifest) *WhatsAppMessageDrafterSkill {
	return &WhatsAppMessageDrafterSkill{def: manifest.toDefinition()}
}

func (s *WhatsAppMessageDrafterSkill) Definition() engine.SkillDefinition { return s.def }

func (s *WhatsAppMessageDrafterSkill) Validate(input map[string]any) error {
	recipient := strings.TrimSpace(conv.AsString(input["to"]))
	if recipient == "" {
		recipient = strings.TrimSpace(conv.AsString(input["recipient"]))
	}
	if recipient == "" {
		return fmt.Errorf("whatsapp-message-drafter requires a recipient")
	}

	messageType := strings.ToLower(strings.TrimSpace(conv.AsString(input["type"])))
	if messageType == "" {
		messageType = "text"
	}
	switch messageType {
	case "text":
		if strings.TrimSpace(conv.AsString(input["text"])) == "" {
			return fmt.Errorf("whatsapp-message-drafter requires text for text messages")
		}
	case "template":
		if strings.TrimSpace(conv.AsString(input["template_name"])) == "" {
			return fmt.Errorf("whatsapp-message-drafter requires template_name for template sends")
		}
	default:
		return fmt.Errorf("whatsapp-message-drafter type must be text or template")
	}
	return nil
}

func (s *WhatsAppMessageDrafterSkill) Execute(_ context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	recipient := strings.TrimSpace(conv.AsString(req.Input["to"]))
	if recipient == "" {
		recipient = strings.TrimSpace(conv.AsString(req.Input["recipient"]))
	}
	messageType := strings.ToLower(strings.TrimSpace(conv.AsString(req.Input["type"])))
	if messageType == "" {
		messageType = "text"
	}

	provider := strings.TrimSpace(conv.AsString(req.Input["provider"]))
	if provider == "" {
		provider = "meta_cloud"
	}

	payload := map[string]any{
		"provider":          provider,
		"channel":           "whatsapp",
		"to":                recipient,
		"type":              messageType,
		"text":              strings.TrimSpace(conv.AsString(req.Input["text"])),
		"template_name":     strings.TrimSpace(conv.AsString(req.Input["template_name"])),
		"template_language": firstNonEmptyString(strings.TrimSpace(conv.AsString(req.Input["template_language"])), "en"),
		"test":              conv.AsBool(req.Input["test"]),
	}
	if variables := normalizeTemplateVariables(req.Input["template_variables"]); len(variables) > 0 {
		payload["template_variables"] = variables
	}

	output := map[string]any{
		"provider": provider,
		"channel":  "whatsapp",
		"to":       recipient,
		"type":     messageType,
	}
	if messageType == "text" {
		output["text_preview"] = strings.TrimSpace(conv.AsString(req.Input["text"]))
	}
	if templateName := strings.TrimSpace(conv.AsString(req.Input["template_name"])); templateName != "" {
		output["template_name"] = templateName
	}

	return engine.SkillResult{
		Output: output,
		Actions: []engine.AdapterAction{{
			Adapter:          "whatsapp",
			Operation:        "send_message",
			Payload:          payload,
			RequiresApproval: true,
		}},
	}, nil
}

func normalizeTemplateVariables(value any) map[string]string {
	vars := map[string]string{}
	switch cast := value.(type) {
	case map[string]string:
		for key, item := range cast {
			key = strings.TrimSpace(key)
			item = strings.TrimSpace(item)
			if key != "" && item != "" {
				vars[key] = item
			}
		}
	case map[string]any:
		for key, item := range cast {
			key = strings.TrimSpace(key)
			itemValue := strings.TrimSpace(conv.AsString(item))
			if key != "" && itemValue != "" {
				vars[key] = itemValue
			}
		}
	}
	return vars
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func newDossierService(req engine.SkillRequest) (*dossier.Service, error) {
	return dossier.NewService(req.Sandbox.RuntimeRoot())
}

func parseWatchlistsInput(input map[string]any, secrets engine.SecretProvider) ([]dossier.Watchlist, error) {
	if raw := strings.TrimSpace(conv.AsString(input["watchlists_json"])); raw != "" {
		return dossier.ParseWatchlists(raw, dossier.ParseTrustedDomains(conv.AsString(input["trusted_domains"])))
	}
	if value, ok := input["watchlists"].([]any); ok {
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return dossier.ParseWatchlists(string(data), dossier.ParseTrustedDomains(conv.AsString(input["trusted_domains"])))
	}
	if secrets != nil {
		raw, _ := secrets.Get("research_watchlists")
		trusted, _ := secrets.Get("trusted_domains")
		return dossier.ParseWatchlists(raw, dossier.ParseTrustedDomains(trusted))
	}
	return nil, nil
}

// ── mitpo-ba-researcher ─────────────────────────────────────────────────────

type BAResearcherSkill struct {
	manifest Manifest
}

func NewBAResearcherSkill(manifest Manifest) *BAResearcherSkill {
	return &BAResearcherSkill{manifest: manifest}
}

func (s *BAResearcherSkill) Definition() engine.SkillDefinition {
	return engine.SkillDefinition{
		Name:        s.manifest.Name,
		Description: s.manifest.Description,
		Tools:       s.manifest.Tools,
		Events:      s.manifest.Events,
		Prompt:      s.manifest.Prompt,
	}
}

func (s *BAResearcherSkill) Validate(input map[string]any) error {
	company := strings.TrimSpace(conv.AsString(input["company"]))
	competitors := conv.AsStringSlice(input["competitors"])
	if company == "" && len(competitors) == 0 {
		return fmt.Errorf("company or competitors is required")
	}
	return nil
}

func (s *BAResearcherSkill) Execute(ctx context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	service := research.NewService()
	result, err := service.Analyze(ctx, research.AnalyzeRequest{
		Company:     strings.TrimSpace(conv.AsString(req.Input["company"])),
		Competitors: conv.AsStringSlice(req.Input["competitors"]),
		Domains:     conv.AsStringSlice(req.Input["domains"]),
		Pages:       conv.AsStringSlice(req.Input["pages"]),
		FocusAreas:  conv.AsStringSlice(req.Input["focus_areas"]),
		Market:      strings.TrimSpace(conv.AsString(req.Input["market"])),
		Country:     strings.TrimSpace(conv.AsString(req.Input["country"])),
		Location:    strings.TrimSpace(conv.AsString(req.Input["location"])),
		MaxSources:  parseOptionalInt(req.Input["max_sources"]),
		Provider:    strings.TrimSpace(conv.AsString(req.Input["provider"])),
		Debug:       conv.AsBool(req.Input["debug"]),
	}, req.Secrets)
	if err != nil {
		return engine.SkillResult{}, err
	}

	output := map[string]any{
		"company":          strings.TrimSpace(conv.AsString(req.Input["company"])),
		"competitors":      conv.AsStringSlice(req.Input["competitors"]),
		"domains":          conv.AsStringSlice(req.Input["domains"]),
		"pages":            conv.AsStringSlice(req.Input["pages"]),
		"market":           strings.TrimSpace(conv.AsString(req.Input["market"])),
		"focus_areas":      conv.AsStringSlice(req.Input["focus_areas"]),
		"provider":         result.Provider,
		"fallback_reason":  result.FallbackReason,
		"summary":          result.Summary,
		"findings":         result.Findings,
		"competitor_notes": result.CompetitorNotes,
		"sources":          result.Sources,
		"warnings":         result.Warnings,
		"coverage":         result.Coverage,
	}
	if output["company"] == "" {
		output["company"] = firstNonEmptyString(conv.AsStringSlice(req.Input["competitors"])...)
	}
	return engine.SkillResult{Output: output}, nil
}

// ── mitpo-creative-director ─────────────────────────────────────────────────

type DossierGenerateSkill struct {
	manifest Manifest
}

func NewDossierGenerateSkill(manifest Manifest) *DossierGenerateSkill {
	return &DossierGenerateSkill{manifest: manifest}
}

func (s *DossierGenerateSkill) Definition() engine.SkillDefinition {
	return engine.SkillDefinition{
		Name:        s.manifest.Name,
		Description: s.manifest.Description,
		Tools:       s.manifest.Tools,
		Events:      s.manifest.Events,
		Prompt:      s.manifest.Prompt,
	}
}

func (s *DossierGenerateSkill) Validate(input map[string]any) error {
	if strings.TrimSpace(conv.AsString(input["company"])) == "" &&
		len(conv.AsStringSlice(input["competitors"])) == 0 &&
		strings.TrimSpace(conv.AsString(input["watchlist_id"])) == "" &&
		strings.TrimSpace(conv.AsString(input["name"])) == "" {
		return fmt.Errorf("company, competitors, or watchlist context is required")
	}
	return nil
}

func (s *DossierGenerateSkill) Execute(ctx context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	service, err := newDossierService(req)
	if err != nil {
		return engine.SkillResult{}, err
	}
	generated, err := service.GenerateDossier(ctx, dossier.GenerateRequest{
		WatchlistID:    strings.TrimSpace(conv.AsString(req.Input["watchlist_id"])),
		Name:           strings.TrimSpace(conv.AsString(req.Input["name"])),
		Topic:          strings.TrimSpace(conv.AsString(req.Input["topic"])),
		Company:        strings.TrimSpace(conv.AsString(req.Input["company"])),
		Competitors:    conv.AsStringSlice(req.Input["competitors"]),
		Domains:        conv.AsStringSlice(req.Input["domains"]),
		Pages:          conv.AsStringSlice(req.Input["pages"]),
		Market:         strings.TrimSpace(conv.AsString(req.Input["market"])),
		FocusAreas:     conv.AsStringSlice(req.Input["focus_areas"]),
		TrustedDomains: dossier.ParseTrustedDomains(conv.AsString(req.Input["trusted_domains"])),
		Provider:       strings.TrimSpace(conv.AsString(req.Input["provider"])),
		Debug:          conv.AsBool(req.Input["debug"]),
	}, req.Secrets)
	if err != nil {
		return engine.SkillResult{}, err
	}
	return engine.SkillResult{
		Output: map[string]any{
			"watchlist":       generated.Watchlist,
			"dossier":         generated.Dossier,
			"evidence":        generated.Evidence,
			"changes":         generated.Changes,
			"recommendations": generated.Recommendations,
		},
	}, nil
}

type WatchlistRefreshSkill struct {
	manifest Manifest
}

func NewWatchlistRefreshSkill(manifest Manifest) *WatchlistRefreshSkill {
	return &WatchlistRefreshSkill{manifest: manifest}
}

func (s *WatchlistRefreshSkill) Definition() engine.SkillDefinition {
	return engine.SkillDefinition{
		Name:        s.manifest.Name,
		Description: s.manifest.Description,
		Tools:       s.manifest.Tools,
		Events:      s.manifest.Events,
		Prompt:      s.manifest.Prompt,
	}
}

func (s *WatchlistRefreshSkill) Validate(_ map[string]any) error {
	return nil
}

func (s *WatchlistRefreshSkill) Execute(ctx context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	service, err := newDossierService(req)
	if err != nil {
		return engine.SkillResult{}, err
	}
	watchlists, err := parseWatchlistsInput(req.Input, req.Secrets)
	if err != nil {
		return engine.SkillResult{}, err
	}
	if len(watchlists) == 0 {
		watchlists, err = service.ListWatchlists(ctx)
		if err != nil {
			return engine.SkillResult{}, err
		}
	}
	if len(watchlists) == 0 {
		return engine.SkillResult{}, fmt.Errorf("no watchlists provided or configured")
	}
	result, err := service.RefreshWatchlists(ctx, watchlists, req.Secrets)
	if err != nil {
		return engine.SkillResult{}, err
	}
	return engine.SkillResult{
		Output: map[string]any{
			"watchlists":           result.Watchlists,
			"dossiers":             result.Dossiers,
			"changes":              result.Changes,
			"recommendations":      result.Recommendations,
			"warnings":             result.Warnings,
			"watchlist_count":      len(result.Watchlists),
			"dossier_count":        len(result.Dossiers),
			"change_count":         len(result.Changes),
			"recommendation_count": len(result.Recommendations),
		},
	}, nil
}

type DossierDiffSkill struct {
	manifest Manifest
}

func NewDossierDiffSkill(manifest Manifest) *DossierDiffSkill {
	return &DossierDiffSkill{manifest: manifest}
}

func (s *DossierDiffSkill) Definition() engine.SkillDefinition {
	return engine.SkillDefinition{
		Name:        s.manifest.Name,
		Description: s.manifest.Description,
		Tools:       s.manifest.Tools,
		Events:      s.manifest.Events,
		Prompt:      s.manifest.Prompt,
	}
}

func (s *DossierDiffSkill) Validate(_ map[string]any) error {
	return nil
}

func (s *DossierDiffSkill) Execute(ctx context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	service, err := newDossierService(req)
	if err != nil {
		return engine.SkillResult{}, err
	}
	diff, err := service.DiffLatest(ctx, strings.TrimSpace(conv.AsString(req.Input["watchlist_id"])))
	if err != nil {
		return engine.SkillResult{}, err
	}
	return engine.SkillResult{
		Output: map[string]any{
			"watchlist_id": diff.WatchlistID,
			"dossier_id":   diff.DossierID,
			"summary":      diff.Summary,
			"changes":      diff.Changes,
		},
	}, nil
}

type RecommendActionsSkill struct {
	manifest Manifest
}

func NewRecommendActionsSkill(manifest Manifest) *RecommendActionsSkill {
	return &RecommendActionsSkill{manifest: manifest}
}

func (s *RecommendActionsSkill) Definition() engine.SkillDefinition {
	return engine.SkillDefinition{
		Name:        s.manifest.Name,
		Description: s.manifest.Description,
		Tools:       s.manifest.Tools,
		Events:      s.manifest.Events,
		Prompt:      s.manifest.Prompt,
	}
}

func (s *RecommendActionsSkill) Validate(_ map[string]any) error {
	return nil
}

func (s *RecommendActionsSkill) Execute(ctx context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	service, err := newDossierService(req)
	if err != nil {
		return engine.SkillResult{}, err
	}
	recommendations, err := service.ListRecommendations(ctx, "", 32)
	if err != nil {
		return engine.SkillResult{}, err
	}
	dossierID := strings.TrimSpace(conv.AsString(req.Input["dossier_id"]))
	watchlistID := strings.TrimSpace(conv.AsString(req.Input["watchlist_id"]))
	filtered := make([]dossier.Recommendation, 0, len(recommendations))
	for _, item := range recommendations {
		if dossierID != "" && item.DossierID != dossierID {
			continue
		}
		if watchlistID != "" && item.WatchlistID != watchlistID {
			continue
		}
		filtered = append(filtered, item)
	}
	if len(filtered) == 0 {
		return engine.SkillResult{}, fmt.Errorf("no recommendations available for the requested dossier scope")
	}
	return engine.SkillResult{
		Output: map[string]any{
			"dossier_id":           dossierID,
			"watchlist_id":         watchlistID,
			"recommendations":      filtered,
			"recommendation_count": len(filtered),
		},
	}, nil
}

type CreativeDirectorSkill struct {
	manifest Manifest
}

func NewCreativeDirectorSkill(manifest Manifest) *CreativeDirectorSkill {
	return &CreativeDirectorSkill{manifest: manifest}
}

func (s *CreativeDirectorSkill) Definition() engine.SkillDefinition {
	return engine.SkillDefinition{
		Name:        s.manifest.Name,
		Description: s.manifest.Description,
		Tools:       s.manifest.Tools,
		Events:      s.manifest.Events,
		Prompt:      s.manifest.Prompt,
	}
}

func (s *CreativeDirectorSkill) Validate(input map[string]any) error {
	if strings.TrimSpace(conv.AsString(input["brand_name"])) == "" {
		return fmt.Errorf("brand_name is required")
	}
	if strings.TrimSpace(conv.AsString(input["tone"])) == "" {
		return fmt.Errorf("tone is required")
	}
	if strings.TrimSpace(conv.AsString(input["audience"])) == "" {
		return fmt.Errorf("audience is required")
	}
	return nil
}

func (s *CreativeDirectorSkill) Execute(_ context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	brandName := strings.TrimSpace(conv.AsString(req.Input["brand_name"]))
	tone := strings.TrimSpace(conv.AsString(req.Input["tone"]))
	audience := strings.TrimSpace(conv.AsString(req.Input["audience"]))
	contentType := strings.TrimSpace(conv.AsString(req.Input["content_type"]))
	guidelines := strings.TrimSpace(conv.AsString(req.Input["guidelines"]))

	output := map[string]any{
		"brand_name":    brandName,
		"copy_variants": []string{},
		"tone_analysis": fmt.Sprintf("Tone direction: %s for %s audience.", tone, audience),
		"recommendations": []string{
			fmt.Sprintf("Consider A/B testing %s variations with the %s audience segment.", tone, audience),
		},
	}
	if contentType != "" {
		output["content_type"] = contentType
	}
	if guidelines != "" {
		output["guidelines"] = guidelines
	}

	return engine.SkillResult{Output: output}, nil
}

// ── mitpo-seo-auditor ───────────────────────────────────────────────────────

type SEOAuditorSkill struct {
	manifest Manifest
}

func NewSEOAuditorSkill(manifest Manifest) *SEOAuditorSkill {
	return &SEOAuditorSkill{manifest: manifest}
}

func (s *SEOAuditorSkill) Definition() engine.SkillDefinition {
	return engine.SkillDefinition{
		Name:        s.manifest.Name,
		Description: s.manifest.Description,
		Tools:       s.manifest.Tools,
		Events:      s.manifest.Events,
		Prompt:      s.manifest.Prompt,
	}
}

func (s *SEOAuditorSkill) Validate(input map[string]any) error {
	rawURL := strings.TrimSpace(conv.AsString(input["url"]))
	if rawURL == "" {
		return fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("only http and https URLs are supported")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return fmt.Errorf("local addresses are not allowed")
	}
	return nil
}

func (s *SEOAuditorSkill) Execute(_ context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	targetURL := strings.TrimSpace(conv.AsString(req.Input["url"]))
	keywords := conv.AsStringSlice(req.Input["keywords"])

	output := map[string]any{
		"url":   targetURL,
		"score": 0,
		"findings": []string{
			"SEO audit request queued for analysis.",
		},
		"recommendations": []string{
			"Full crawl and metadata evaluation pending.",
		},
	}
	if len(keywords) > 0 {
		output["keywords"] = keywords
	}

	return engine.SkillResult{Output: output}, nil
}

// ── mitpo-researcher ───────────────────────────────────────────────────────

type ResearcherSkill struct {
	manifest Manifest
}

func NewResearcherSkill(manifest Manifest) *ResearcherSkill {
	return &ResearcherSkill{manifest: manifest}
}

func (s *ResearcherSkill) Definition() engine.SkillDefinition {
	return engine.SkillDefinition{
		Name:        s.manifest.Name,
		Description: s.manifest.Description,
		Tools:       s.manifest.Tools,
		Events:      s.manifest.Events,
		Prompt:      s.manifest.Prompt,
	}
}

func (s *ResearcherSkill) Validate(input map[string]any) error {
	rawURL := strings.TrimSpace(conv.AsString(input["url"]))
	if rawURL == "" {
		return fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("only http and https URLs are supported")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return fmt.Errorf("local addresses are not allowed")
	}
	return nil
}

func (s *ResearcherSkill) Execute(ctx context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	targetURL := strings.TrimSpace(conv.AsString(req.Input["url"]))

	client := &http.Client{Timeout: 30 * time.Second}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return engine.SkillResult{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("User-Agent", "PookiePaws/1.0 (marketing-research-bot)")

	resp, err := client.Do(httpReq)
	if err != nil {
		return engine.SkillResult{}, fmt.Errorf("fetch url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return engine.SkillResult{}, fmt.Errorf("fetch returned HTTP %d", resp.StatusCode)
	}

	// Limit body to 1 MB to prevent abuse.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return engine.SkillResult{}, fmt.Errorf("read body: %w", err)
	}

	html := string(body)
	title := extractHTMLTitle(html)
	rawText := stripHTML(html)

	// Truncate to a reasonable length for downstream processing.
	const maxChars = 10000
	if len(rawText) > maxChars {
		rawText = rawText[:maxChars]
	}

	// Build a summary from the first portion of the text.
	summary := rawText
	const summaryLen = 500
	if len(summary) > summaryLen {
		summary = summary[:summaryLen] + "…"
	}

	return engine.SkillResult{
		Output: map[string]any{
			"url":      targetURL,
			"title":    title,
			"summary":  summary,
			"raw_text": rawText,
		},
	}, nil
}

// extractHTMLTitle extracts the content between <title> and </title> tags.
func extractHTMLTitle(html string) string {
	lower := strings.ToLower(html)
	start := strings.Index(lower, "<title")
	if start < 0 {
		return ""
	}
	// Skip past the opening tag (handle attributes like <title lang="en">).
	close := strings.IndexByte(lower[start:], '>')
	if close < 0 {
		return ""
	}
	start += close + 1
	end := strings.Index(lower[start:], "</title>")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(html[start : start+end])
}

// stripHTML removes HTML tags and script/style blocks, returning plain text.
func stripHTML(html string) string {
	// Remove <script> and <style> blocks entirely.
	result := removeTagBlocks(html, "script")
	result = removeTagBlocks(result, "style")

	// Strip remaining tags.
	var buf strings.Builder
	buf.Grow(len(result))
	inTag := false
	for i := 0; i < len(result); i++ {
		switch {
		case result[i] == '<':
			inTag = true
		case result[i] == '>':
			inTag = false
		case !inTag:
			buf.WriteByte(result[i])
		}
	}
	text := buf.String()

	// Decode common HTML entities.
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", `"`)
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")

	// Collapse whitespace.
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

// removeTagBlocks strips everything between <tag...> and </tag> (case-insensitive).
func removeTagBlocks(html string, tag string) string {
	lower := strings.ToLower(html)
	openTag := "<" + tag
	closeTag := "</" + tag + ">"
	var buf strings.Builder
	buf.Grow(len(html))
	cursor := 0
	for {
		start := strings.Index(lower[cursor:], openTag)
		if start < 0 {
			buf.WriteString(html[cursor:])
			break
		}
		buf.WriteString(html[cursor : cursor+start])
		end := strings.Index(lower[cursor+start:], closeTag)
		if end < 0 {
			// No closing tag — skip to end.
			break
		}
		cursor = cursor + start + end + len(closeTag)
	}
	return buf.String()
}

func parseOptionalInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return 0
	}
}

// ── mitpo-markdown-export ──────────────────────────────────────────────────

type MarkdownExportSkill struct {
	manifest Manifest
}

func NewMarkdownExportSkill(manifest Manifest) *MarkdownExportSkill {
	return &MarkdownExportSkill{manifest: manifest}
}

func (s *MarkdownExportSkill) Definition() engine.SkillDefinition {
	return engine.SkillDefinition{
		Name:        s.manifest.Name,
		Description: s.manifest.Description,
		Tools:       s.manifest.Tools,
		Events:      s.manifest.Events,
		Prompt:      s.manifest.Prompt,
	}
}

func (s *MarkdownExportSkill) Validate(input map[string]any) error {
	content := strings.TrimSpace(conv.AsString(input["content"]))
	if content == "" {
		return fmt.Errorf("content is required")
	}
	return nil
}

func (s *MarkdownExportSkill) Execute(_ context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	content := strings.TrimSpace(conv.AsString(req.Input["content"]))
	title := strings.TrimSpace(conv.AsString(req.Input["title"]))
	prefix := strings.TrimSpace(conv.AsString(req.Input["filename"]))
	if prefix == "" {
		prefix = "export"
	}

	// Build the markdown document.
	var doc strings.Builder
	if title != "" {
		doc.WriteString("# ")
		doc.WriteString(title)
		doc.WriteString("\n\n")
	}
	doc.WriteString(content)
	doc.WriteString("\n")
	data := []byte(doc.String())

	// Generate timestamped filename.
	stamp := time.Now().UTC().Format("2006-01-05T15-04-05")
	filename := fmt.Sprintf("%s-%s.md", prefix, stamp)
	relativePath := filepath.Join("exports", filename)

	// Resolve the final export path within the workspace sandbox.
	fullPath, err := req.Sandbox.ResolveWithinWorkspace(relativePath)
	if err != nil {
		return engine.SkillResult{}, fmt.Errorf("resolve export path: %w", err)
	}

	// Write using the sandbox (which ensures the directory exists and stays
	// within the workspace boundary).
	if err := req.Sandbox.WriteFile(context.Background(), relativePath, data); err != nil {
		return engine.SkillResult{}, fmt.Errorf("write export: %w", err)
	}

	return engine.SkillResult{
		Output: map[string]any{
			"path": fullPath,
			"size": len(data),
		},
	}, nil
}
