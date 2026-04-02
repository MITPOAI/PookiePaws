package skills

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/conv"
	"github.com/mitpoai/pookiepaws/internal/engine"
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
	if company == "" {
		return fmt.Errorf("company is required")
	}
	return nil
}

func (s *BAResearcherSkill) Execute(_ context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	company := strings.TrimSpace(conv.AsString(req.Input["company"]))
	domains := conv.AsStringSlice(req.Input["domains"])
	focusAreas := conv.AsStringSlice(req.Input["focus_areas"])
	market := strings.TrimSpace(conv.AsString(req.Input["market"]))

	output := map[string]any{
		"company":  company,
		"findings": []string{},
		"summary":  fmt.Sprintf("Business analysis request for %s queued. Domains: %d, focus areas: %d.", company, len(domains), len(focusAreas)),
		"sources":  []string{},
	}
	if market != "" {
		output["market"] = market
	}
	if len(domains) > 0 {
		output["domains"] = domains
	}
	if len(focusAreas) > 0 {
		output["focus_areas"] = focusAreas
	}

	return engine.SkillResult{Output: output}, nil
}

// ── mitpo-creative-director ─────────────────────────────────────────────────

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
