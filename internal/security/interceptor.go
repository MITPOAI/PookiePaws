package security

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/conv"
	"github.com/mitpoai/pookiepaws/internal/engine"
)

type SkillExecutionInterceptor struct {
	guard    engine.ExecGuard
	policies map[string]skillPolicy
}

type skillPolicy struct {
	risk         string
	allowedKeys  map[string]struct{}
	altPrompt    string
	inspectInput func(input map[string]any) *payloadFinding
}

type payloadFinding struct {
	path      string
	reason    string
	violation string
	risk      string
}

var _ engine.ExecutionInterceptor = (*SkillExecutionInterceptor)(nil)

func NewSkillExecutionInterceptor() *SkillExecutionInterceptor {
	return &SkillExecutionInterceptor{
		guard: NewCommandExecGuard(),
		policies: map[string]skillPolicy{
			"utm-validator": {
				risk:        "low",
				allowedKeys: setOf("url"),
				altPrompt:   "Suggest a read-only alternative that validates or normalizes the marketing URL without touching local files, credentials, or external systems.",
				inspectInput: func(input map[string]any) *payloadFinding {
					rawURL := strings.TrimSpace(conv.AsString(input["url"]))
					if rawURL == "" {
						return nil
					}
					parsed, err := url.Parse(rawURL)
					if err != nil {
						return nil
					}
					scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
					host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
					if scheme != "http" && scheme != "https" {
						return &payloadFinding{
							path:      "url",
							reason:    "only http and https URLs are allowed in the validator",
							violation: "unsupported_url_scheme",
							risk:      "medium",
						}
					}
					if host == "localhost" || host == "127.0.0.1" || host == "::1" {
						return &payloadFinding{
							path:      "url",
							reason:    "local addresses are blocked from marketing URL validation",
							violation: "local_target_blocked",
							risk:      "medium",
						}
					}
					return nil
				},
			},
			"salesmanago-lead-router": {
				risk:        "high",
				allowedKeys: setOf("email", "contact_id", "lead_id", "segment", "priority", "name", "phone"),
				altPrompt:   "Suggest a safer CRM workflow that keeps the action approval-gated and limited to a single identified lead instead of any bulk export, deletion, or unrestricted extraction.",
				inspectInput: func(input map[string]any) *payloadFinding {
					for _, key := range []string{"email", "contact_id", "lead_id"} {
						value := strings.ToLower(strings.TrimSpace(conv.AsString(input[key])))
						switch value {
						case "*", "all", "everyone", "all_contacts", "entire_database":
							return &payloadFinding{
								path:      key,
								reason:    "bulk CRM targets are not allowed",
								violation: "bulk_target_blocked",
								risk:      "high",
							}
						}
					}
					return nil
				},
			},
			"mitto-sms-drafter": {
				risk:        "high",
				allowedKeys: setOf("message", "recipients", "campaign_name", "from", "test"),
				altPrompt:   "Suggest a safer outreach workflow that stays approval-gated, limits scope, and avoids any destructive, unrestricted, or credential-related action.",
				inspectInput: func(input map[string]any) *payloadFinding {
					recipients := conv.AsStringSlice(input["recipients"])
					if len(recipients) > 100 {
						return &payloadFinding{
							path:      "recipients",
							reason:    "broadcasts over 100 recipients are blocked until a narrower audience is defined",
							violation: "bulk_send_blocked",
							risk:      "high",
						}
					}
					return nil
				},
			},
		},
	}
}

func (i *SkillExecutionInterceptor) Inspect(_ context.Context, skill engine.SkillDefinition, input map[string]any) (engine.InterceptionDecision, error) {
	policy, ok := i.policies[skill.Name]
	if !ok {
		return blockDecision("medium", "that skill is not on the security allowlist yet", "skill_not_allowlisted", skill.Name, "Suggest a read-only, approval-aware alternative using only already allowlisted marketing skills."), nil
	}

	if input == nil {
		input = map[string]any{}
	}

	if finding := enforceAllowedKeys(input, policy.allowedKeys); finding != nil {
		return i.toDecision(skill.Name, policy.altPrompt, *finding), nil
	}
	if finding := inspectPayload(i.guard, input); finding != nil {
		return i.toDecision(skill.Name, policy.altPrompt, *finding), nil
	}
	if policy.inspectInput != nil {
		if finding := policy.inspectInput(input); finding != nil {
			return i.toDecision(skill.Name, policy.altPrompt, *finding), nil
		}
	}

	return engine.InterceptionDecision{
		Allowed: true,
		Risk:    policy.risk,
	}, nil
}

func (i *SkillExecutionInterceptor) toDecision(skillName string, altPrompt string, finding payloadFinding) engine.InterceptionDecision {
	decision := blockDecision(finding.risk, finding.reason, finding.violation, skillName, altPrompt)
	decision.SafeAlternativeContext["blocked_path"] = finding.path
	return decision
}

func blockDecision(risk string, reason string, violation string, skillName string, altPrompt string) engine.InterceptionDecision {
	if risk == "" {
		risk = "medium"
	}
	context := map[string]any{
		"blocked_skill": skillName,
		"constraints": []string{
			"No destructive commands",
			"No shell or script execution",
			"No credential extraction or secret handling",
			"No bulk export, delete, wipe, or unrestricted scraping",
			"External sends and CRM mutations must remain approval-gated",
		},
	}
	return engine.InterceptionDecision{
		Allowed:                false,
		Risk:                   risk,
		Reason:                 reason,
		Violation:              violation,
		SafeAlternativePrompt:  strings.TrimSpace(altPrompt),
		SafeAlternativeContext: context,
	}
}

func inspectPayload(guard engine.ExecGuard, input map[string]any) *payloadFinding {
	for key, value := range input {
		if finding := inspectValue(guard, key, value); finding != nil {
			return finding
		}
	}
	return nil
}

func inspectValue(guard engine.ExecGuard, path string, value any) *payloadFinding {
	lowerPath := strings.ToLower(strings.TrimSpace(path))
	for _, token := range []string{"command", "script", "shell", "exec", "credential", "secret", "password", "token"} {
		if strings.Contains(lowerPath, token) {
			return &payloadFinding{
				path:      path,
				reason:    "command-like or secret-bearing payload fields are blocked",
				violation: "unsafe_field_blocked",
				risk:      "high",
			}
		}
	}

	switch cast := value.(type) {
	case nil:
		return nil
	case string:
		return inspectString(guard, path, cast)
	case []string:
		for index, item := range cast {
			if finding := inspectString(guard, fmt.Sprintf("%s[%d]", path, index), item); finding != nil {
				return finding
			}
		}
	case []any:
		for index, item := range cast {
			if finding := inspectValue(guard, fmt.Sprintf("%s[%d]", path, index), item); finding != nil {
				return finding
			}
		}
	case map[string]string:
		for key, item := range cast {
			nextPath := key
			if path != "" {
				nextPath = path + "." + key
			}
			if finding := inspectString(guard, nextPath, item); finding != nil {
				return finding
			}
		}
	case map[string]any:
		for key, item := range cast {
			nextPath := key
			if path != "" {
				nextPath = path + "." + key
			}
			if finding := inspectValue(guard, nextPath, item); finding != nil {
				return finding
			}
		}
	}

	return nil
}

func inspectString(guard engine.ExecGuard, path string, value string) *payloadFinding {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	lower := strings.ToLower(value)
	blockedPhrases := map[string]string{
		"rm -rf":        "destructive shell fragments are blocked",
		"del /f":        "destructive shell fragments are blocked",
		"drop table":    "database-destructive instructions are blocked",
		"truncate":      "database-destructive instructions are blocked",
		"wipe":          "destructive actions are blocked",
		"destroy":       "destructive actions are blocked",
		"powershell":    "shell execution is blocked",
		"cmd /c":        "shell execution is blocked",
		"bash -c":       "shell execution is blocked",
		"curl | sh":     "piped shell execution is blocked",
		"wget | sh":     "piped shell execution is blocked",
		"delete all":    "bulk destructive actions are blocked",
		"export all":    "bulk extraction actions are blocked",
		"dump database": "bulk extraction actions are blocked",
	}
	for phrase, reason := range blockedPhrases {
		if strings.Contains(lower, phrase) {
			return &payloadFinding{
				path:      path,
				reason:    reason,
				violation: "unsafe_payload_phrase",
				risk:      "high",
			}
		}
	}

	if looksLikeCommand(value) && guard != nil {
		parts := strings.Fields(value)
		if len(parts) > 0 {
			if err := guard.Validate(parts); err != nil {
				return &payloadFinding{
					path:      path,
					reason:    "command execution payloads are blocked unless explicitly allowlisted",
					violation: "command_payload_blocked",
					risk:      "high",
				}
			}
		}
	}

	return nil
}

func looksLikeCommand(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, prefix := range []string{"git ", "go ", "powershell ", "cmd ", "bash ", "sh ", "python ", "rm ", "del ", "curl ", "wget "} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func enforceAllowedKeys(input map[string]any, allowed map[string]struct{}) *payloadFinding {
	for key := range input {
		if _, ok := allowed[key]; ok {
			continue
		}
		return &payloadFinding{
			path:      key,
			reason:    "that field is not approved for this skill",
			violation: "field_not_allowlisted",
			risk:      "medium",
		}
	}
	return nil
}

func setOf(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
