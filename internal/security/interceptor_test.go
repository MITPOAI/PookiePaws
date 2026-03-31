package security

import (
	"context"
	"testing"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

func TestSkillExecutionInterceptorAllowsSafeUTMValidation(t *testing.T) {
	interceptor := NewSkillExecutionInterceptor()
	decision, err := interceptor.Inspect(context.Background(), engine.SkillDefinition{Name: "utm-validator"}, map[string]any{
		"url": "https://example.com/?utm_source=meta&utm_medium=paid&utm_campaign=launch",
	})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected decision to allow safe input")
	}
}

func TestSkillExecutionInterceptorBlocksUnknownSkills(t *testing.T) {
	interceptor := NewSkillExecutionInterceptor()
	decision, err := interceptor.Inspect(context.Background(), engine.SkillDefinition{Name: "crm-extractor"}, map[string]any{
		"scope": "all",
	})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected unknown skill to be blocked")
	}
	if decision.Violation != "skill_not_allowlisted" {
		t.Fatalf("unexpected violation %q", decision.Violation)
	}
}

func TestSkillExecutionInterceptorBlocksUnsafePayload(t *testing.T) {
	interceptor := NewSkillExecutionInterceptor()
	decision, err := interceptor.Inspect(context.Background(), engine.SkillDefinition{Name: "mitto-sms-drafter"}, map[string]any{
		"message":    "Run powershell -Command Remove-Item everything",
		"recipients": []string{"+61400000000"},
	})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected destructive payload to be blocked")
	}
	if decision.Violation == "" {
		t.Fatalf("expected a concrete violation code")
	}
}
