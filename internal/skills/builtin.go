package skills

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

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
	identifier := strings.TrimSpace(asString(input["email"]))
	if identifier == "" {
		identifier = strings.TrimSpace(asString(input["contact_id"]))
	}
	if identifier == "" {
		identifier = strings.TrimSpace(asString(input["lead_id"]))
	}
	if identifier == "" {
		return fmt.Errorf("salesmanago-lead-router requires email, contact_id, or lead_id")
	}
	if strings.TrimSpace(asString(input["segment"])) == "" {
		return fmt.Errorf("salesmanago-lead-router requires segment")
	}
	return nil
}

func (s *SalesmanagoLeadRouterSkill) Execute(_ context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	email := strings.TrimSpace(asString(req.Input["email"]))
	contactID := strings.TrimSpace(asString(req.Input["contact_id"]))
	leadID := strings.TrimSpace(asString(req.Input["lead_id"]))
	segment := strings.ToLower(asString(req.Input["segment"]))
	priority := strings.ToLower(asString(req.Input["priority"]))
	name := strings.TrimSpace(asString(req.Input["name"]))
	phone := strings.TrimSpace(asString(req.Input["phone"]))

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
	if strings.TrimSpace(asString(input["message"])) == "" {
		return fmt.Errorf("mitto-sms-drafter requires message")
	}
	recipients := asStringSlice(input["recipients"])
	if len(recipients) == 0 {
		return fmt.Errorf("mitto-sms-drafter requires at least one recipient")
	}
	return nil
}

func (s *MittoSMSDrafterSkill) Execute(_ context.Context, req engine.SkillRequest) (engine.SkillResult, error) {
	message := strings.TrimSpace(asString(req.Input["message"]))
	recipients := asStringSlice(req.Input["recipients"])
	campaign := asString(req.Input["campaign_name"])
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
		"from":          asString(req.Input["from"]),
		"test":          asBool(req.Input["test"]),
	}

	return engine.SkillResult{
		Output: map[string]any{
			"campaign_name":  campaign,
			"message":        message,
			"recipients":     recipients,
			"issues":         issues,
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

func asString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func asStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			items = append(items, asString(item))
		}
		return items
	default:
		return nil
	}
}

func asBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}
