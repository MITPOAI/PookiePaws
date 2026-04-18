package skills

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/security"
)

type noopSecrets struct{}

func (noopSecrets) Get(string) (string, error)                      { return "", nil }
func (noopSecrets) RedactMap(payload map[string]any) map[string]any { return payload }

type mapSecrets map[string]string

func (s mapSecrets) Get(name string) (string, error) {
	return s[name], nil
}

func (s mapSecrets) RedactMap(payload map[string]any) map[string]any { return payload }

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

func TestBAResearcherSkillLiveOutput(t *testing.T) {
	firecrawl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"web": []map[string]any{
					{
						"title":       "OpenClaw Pricing",
						"description": "Operator pricing page",
						"url":         "https://openclaw.example/pricing",
						"markdown":    "# Pricing\nPremium operator plan.",
					},
				},
			},
		})
	}))
	defer firecrawl.Close()

	t.Setenv("POOKIEPAWS_FIRECRAWL_BASE_URL", firecrawl.URL)
	t.Setenv("POOKIEPAWS_JINA_BASE_URL", firecrawl.URL)

	skill := NewBAResearcherSkill(Manifest{Name: "mitpo-ba-researcher"})
	result, err := skill.Execute(context.Background(), engine.SkillRequest{
		Input: map[string]any{
			"company":     "PookiePaws Reserve",
			"competitors": []string{"OpenClaw"},
			"market":      "AU pet gifting",
			"focus_areas": []string{"pricing"},
		},
		Secrets: mapSecrets{"firecrawl_api_key": "fc-test"},
		Now:     time.Now(),
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if result.Output["summary"] == "" {
		t.Fatalf("expected summary output, got %+v", result.Output)
	}
	if _, ok := result.Output["competitor_notes"]; !ok {
		t.Fatalf("expected competitor_notes in output")
	}
	if _, ok := result.Output["coverage"]; !ok {
		t.Fatalf("expected coverage in output")
	}
}

func TestDossierGenerateAndWatchlistRefreshSkills(t *testing.T) {
	root := t.TempDir()
	runtimeRoot := filepath.Join(root, ".pookiepaws")
	workspaceRoot := filepath.Join(runtimeRoot, "workspace")

	sandbox, err := security.NewWorkspaceSandbox(runtimeRoot, workspaceRoot)
	if err != nil {
		t.Fatalf("sandbox: %v", err)
	}

	generate := NewDossierGenerateSkill(Manifest{Name: "mitpo-dossier-generate"})
	result, err := generate.Execute(context.Background(), engine.SkillRequest{
		Input: map[string]any{
			"name":        "OpenClaw core watchlist",
			"topic":       "OpenClaw",
			"company":     "PookiePaws",
			"competitors": []string{"OpenClaw"},
			"domains":     []string{"openclaw.example"},
			"market":      "AU pet gifting",
			"focus_areas": []string{"pricing", "positioning"},
		},
		Sandbox: sandbox,
		Secrets: noopSecrets{},
		Now:     time.Now(),
	})
	if err != nil {
		t.Fatalf("generate execute failed: %v", err)
	}
	if _, ok := result.Output["dossier"]; !ok {
		t.Fatalf("expected dossier in output")
	}
	if _, ok := result.Output["recommendations"]; !ok {
		t.Fatalf("expected recommendations in output")
	}

	refresh := NewWatchlistRefreshSkill(Manifest{Name: "mitpo-watchlist-refresh"})
	refreshResult, err := refresh.Execute(context.Background(), engine.SkillRequest{
		Input:   map[string]any{},
		Sandbox: sandbox,
		Secrets: noopSecrets{},
		Now:     time.Now(),
	})
	if err != nil {
		t.Fatalf("refresh execute failed: %v", err)
	}
	if count, _ := refreshResult.Output["watchlist_count"].(int); count == 0 {
		t.Fatalf("expected refreshed watchlists, got %+v", refreshResult.Output)
	}
}
