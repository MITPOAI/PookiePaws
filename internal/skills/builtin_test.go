package skills

import (
	"context"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type noopSecrets struct{}

func (noopSecrets) Get(string) (string, error)                      { return "", nil }
func (noopSecrets) RedactMap(payload map[string]any) map[string]any { return payload }

func TestParseSkillMarkdown(t *testing.T) {
	content := `---
name: demo
description: Demo skill
tools:
  - one
events:
  - workflow.submitted
---
Prompt body`

	manifest, err := ParseSkillMarkdown(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if manifest.Name != "demo" {
		t.Fatalf("unexpected manifest name %q", manifest.Name)
	}
	if len(manifest.Tools) != 1 {
		t.Fatalf("expected one tool")
	}
}

func TestUTMValidatorSkill(t *testing.T) {
	skill := NewUTMValidatorSkill(Manifest{Name: "utm-validator"})
	result, err := skill.Execute(context.Background(), engine.SkillRequest{
		Input: map[string]any{
			"url": "https://example.com?utm_source=X&utm_medium=email&utm_campaign=Launch",
		},
		Secrets: noopSecrets{},
		Now:     time.Now(),
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if valid, _ := result.Output["valid"].(bool); !valid {
		t.Fatalf("expected validation to pass")
	}
}

func TestMittoSMSDrafterSkillRequiresApproval(t *testing.T) {
	skill := NewMittoSMSDrafterSkill(Manifest{Name: "mitto-sms-drafter"})
	result, err := skill.Execute(context.Background(), engine.SkillRequest{
		Input: map[string]any{
			"message":    "hello",
			"recipients": []any{"+10000000000"},
		},
		Secrets: noopSecrets{},
		Now:     time.Now(),
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(result.Actions) != 1 || !result.Actions[0].RequiresApproval {
		t.Fatalf("expected approval-gated action")
	}
}
