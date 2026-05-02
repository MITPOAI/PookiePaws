package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/assets"
	"github.com/mitpoai/pookiepaws/internal/automation"
	"github.com/mitpoai/pookiepaws/internal/memory"
	"github.com/mitpoai/pookiepaws/internal/providers"
)

type Service struct {
	store    *memory.Store
	provider providers.Provider
}

func NewService(store *memory.Store, provider providers.Provider) Service {
	return Service{store: store, provider: provider}
}

func (s Service) Run(ctx context.Context, cfg RunConfig) (CampaignResult, error) {
	if s.store == nil {
		return CampaignResult{}, errors.New("studio requires a memory store")
	}
	if s.provider == nil {
		return CampaignResult{}, errors.New("studio requires a provider")
	}
	if err := cfg.Request.Normalize(); err != nil {
		return CampaignResult{}, err
	}
	now := time.Now
	if cfg.Now != nil {
		now = cfg.Now
	}
	createdAt := now().UTC()
	campaignID := assets.NewProjectID(cfg.Request.Name, createdAt)
	outputDir := cfg.OutputDir
	if strings.TrimSpace(outputDir) == "" {
		outputDir = "outputs/studio"
	}
	root := filepath.Join(outputDir, campaignID)
	for _, dir := range []string{root, filepath.Join(root, "research"), filepath.Join(root, "strategy"), filepath.Join(root, "ads"), filepath.Join(root, "browser")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return CampaignResult{}, err
		}
	}

	profile, err := s.store.GetBrandProfile(ctx)
	if err != nil {
		return CampaignResult{}, err
	}
	profile = applyCampaignToProfile(profile, cfg.Request)
	if err := s.store.SaveBrandProfile(ctx, profile); err != nil {
		return CampaignResult{}, err
	}

	researchPath := filepath.Join(root, "research", "research_brief.md")
	strategyPath := filepath.Join(root, "strategy", "campaign_strategy.md")
	checklistPath := filepath.Join(root, "ads", "launch_checklist.md")
	workflowPath := filepath.Join(root, "browser", "manual_upload_workflow.yaml")
	if err := writeResearchBrief(researchPath, cfg.Request, profile); err != nil {
		return CampaignResult{}, err
	}
	if err := writeStrategy(strategyPath, cfg.Request, profile); err != nil {
		return CampaignResult{}, err
	}
	if err := writeLaunchChecklist(checklistPath, cfg.Request); err != nil {
		return CampaignResult{}, err
	}
	if err := writeBrowserWorkflow(workflowPath); err != nil {
		return CampaignResult{}, err
	}

	contentSvc := automation.NewService(s.store, s.provider)
	content, contentErr := contentSvc.Run(ctx, automation.RunConfig{
		Request:          toContentBatch(cfg.Request),
		OutputDir:        filepath.Join(root, "content"),
		ProviderOverride: cfg.ProviderOverride,
		DryRun:           cfg.DryRun,
		Render:           cfg.Render,
		RenderScriptPath: cfg.RenderScriptPath,
		Now:              now,
	})

	adExportPath := filepath.Join(root, "ads", "ad_platform_export.json")
	if err := writeAdExport(adExportPath, cfg.Request, campaignID, content); err != nil && contentErr == nil {
		contentErr = err
	}

	result := CampaignResult{
		CampaignID:          campaignID,
		Name:                cfg.Request.Name,
		OutputDir:           root,
		ResearchBriefPath:   researchPath,
		StrategyPath:        strategyPath,
		AdExportPath:        adExportPath,
		LaunchChecklistPath: checklistPath,
		BrowserWorkflowPath: workflowPath,
		Content:             content,
		PostingStatus:       "not_posted_manual_approval_required",
		CreatedAt:           createdAt,
	}
	if err := writeJSON(filepath.Join(root, "studio_result.json"), result); err != nil && contentErr == nil {
		contentErr = err
	}
	return result, contentErr
}

func applyCampaignToProfile(profile memory.BrandProfile, req CampaignRequest) memory.BrandProfile {
	if req.BrandName != "" {
		profile.BrandName = req.BrandName
	}
	if req.Niche != "" {
		profile.Niche = req.Niche
	}
	if req.TargetAudience != "" {
		profile.TargetAudience = req.TargetAudience
	}
	if req.Style != "" {
		profile.PreferredVideoStyle = req.Style
	}
	return profile
}

func toContentBatch(req CampaignRequest) automation.BatchRequest {
	angles := []struct {
		id    string
		angle string
	}{
		{"hook-demo", "thumb-stopping product introduction"},
		{"problem-solution", "problem to solution transformation"},
		{"proof-cta", "benefit proof and CTA"},
		{"offer-reminder", "friendly offer reminder without fake urgency"},
		{"objection-answer", "answer the most common buyer objection"},
	}
	items := make([]automation.ContentItem, 0, req.ContentVariants)
	for i := 0; i < req.ContentVariants; i++ {
		angle := angles[i%len(angles)]
		items = append(items, automation.ContentItem{
			ID:       fmt.Sprintf("%s-%02d", angle.id, i+1),
			Platform: req.Platforms[i%len(req.Platforms)],
			Product:  req.Product,
			Angle:    angle.angle,
		})
	}
	return automation.BatchRequest{
		Name:        req.Name + "-content",
		Product:     req.Product,
		Goal:        req.Goal,
		Style:       req.Style,
		Platforms:   req.Platforms,
		DurationSec: req.DurationSec,
		Variants:    req.ContentVariants,
		Provider:    req.Provider,
		Items:       items,
	}
}

func writeResearchBrief(path string, req CampaignRequest, profile memory.BrandProfile) error {
	var b strings.Builder
	b.WriteString("# Research Brief\n\n")
	b.WriteString("- Brand: " + firstNonEmpty(req.BrandName, profile.BrandName) + "\n")
	b.WriteString("- Product: " + req.Product + "\n")
	b.WriteString("- Niche: " + firstNonEmpty(req.Niche, profile.Niche) + "\n")
	b.WriteString("- Audience: " + firstNonEmpty(req.TargetAudience, profile.TargetAudience) + "\n")
	b.WriteString("- Goal: " + req.Goal + "\n")
	b.WriteString("- Offer: " + firstNonEmpty(req.Offer, "TBD") + "\n\n")
	b.WriteString("## Competitors To Review\n\n")
	if len(req.Competitors) == 0 {
		b.WriteString("- Add 3 direct competitors before launch.\n")
	} else {
		for _, competitor := range req.Competitors {
			b.WriteString("- " + competitor + "\n")
		}
	}
	b.WriteString("\n## Research Questions\n\n")
	b.WriteString("- What hook patterns are common in this niche?\n")
	b.WriteString("- Which claims are safe, provable, and useful?\n")
	b.WriteString("- Which objections need content answers?\n")
	b.WriteString("- Which visual proof points can be shown without misleading viewers?\n\n")
	b.WriteString("## Guardrails\n\n")
	b.WriteString("- No fake urgency, impersonation, hidden automation, credential capture, captcha bypass, or auto-posting.\n")
	b.WriteString("- Paid ad launch requires manual approval, platform policy review, and budget confirmation.\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeStrategy(path string, req CampaignRequest, profile memory.BrandProfile) error {
	var b strings.Builder
	b.WriteString("# Campaign Strategy\n\n")
	b.WriteString("- Primary goal: " + req.Goal + "\n")
	b.WriteString("- Product: " + req.Product + "\n")
	b.WriteString("- Audience: " + firstNonEmpty(req.TargetAudience, profile.TargetAudience) + "\n")
	b.WriteString("- Platforms: " + strings.Join(req.Platforms, ", ") + "\n")
	b.WriteString("- Creative style: " + req.Style + "\n\n")
	b.WriteString("## Funnel\n\n")
	b.WriteString("- Hook: fast first-frame interruption with plain product context.\n")
	b.WriteString("- Problem: show the relatable use case.\n")
	b.WriteString("- Solution: show product as a clear helper, not a miracle claim.\n")
	b.WriteString("- Proof: visual demonstration, testimonial placeholder, or benefit callout.\n")
	b.WriteString("- CTA: short and platform-appropriate.\n\n")
	b.WriteString("## Content Mix\n\n")
	b.WriteString("- 25% hooks and product introductions.\n")
	b.WriteString("- 25% problem/solution demonstrations.\n")
	b.WriteString("- 25% proof, reviews, or benefit callouts.\n")
	b.WriteString("- 25% offer reminder and CTA variations.\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeLaunchChecklist(path string, req CampaignRequest) error {
	var b strings.Builder
	b.WriteString("# Manual Ad Launch Checklist\n\n")
	b.WriteString("- [ ] Confirm product claims are accurate and supportable.\n")
	b.WriteString("- [ ] Confirm landing page URL, UTM parameters, and pixel/tracking setup.\n")
	b.WriteString("- [ ] Confirm ad account access and budget outside pookiepaws.\n")
	b.WriteString("- [ ] Review every generated draft and asset manually.\n")
	b.WriteString("- [ ] Upload only approved assets to accounts you own or have permission to use.\n")
	b.WriteString("- [ ] Confirm platform policies before submission.\n")
	b.WriteString("- [ ] Press publish manually in the platform UI.\n\n")
	b.WriteString("pookiepaws does not spend budget, bypass platform controls, or post ads automatically.\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeBrowserWorkflow(path string) error {
	raw := `browser_workflow:
  name: manual_upload_reviewed_ad
  steps:
    - action: open
      url: "https://example.com"
    - action: require_user_confirmation
      message: "Open your ad platform manually and confirm you own or have permission to use the account."
    - action: upload
      selector: "input[type=file]"
      file: "outputs/final.mp4"
    - action: require_user_confirmation
      message: "Review the draft, targeting, budget, and policy warnings manually before publishing."
`
	return os.WriteFile(path, []byte(raw), 0o644)
}

func writeAdExport(path string, req CampaignRequest, campaignID string, content automation.BatchResult) error {
	export := AdExport{
		CampaignID:     campaignID,
		Name:           req.Name,
		PostingStatus:  "not_posted",
		ManualApproval: true,
	}
	for _, item := range content.Items {
		export.Ads = append(export.Ads, AdExportItem{
			ProjectID:       item.ProjectID,
			Platform:        item.Platform,
			AdName:          req.Name + " - " + item.ID,
			PrimaryText:     primaryText(req, item),
			Headline:        headline(req, item),
			Description:     "Generated by pookiepaws for manual review before upload.",
			CTA:             "Shop now",
			DestinationURL:  "https://example.com/?utm_source=" + item.Platform + "&utm_campaign=" + campaignID,
			EditPlanPath:    item.EditPlanPath,
			FinalOutputPath: item.FinalOutputPath,
		})
	}
	return writeJSON(path, export)
}

func primaryText(req CampaignRequest, item automation.ItemResult) string {
	return fmt.Sprintf("%s: %s. See how %s fits your routine.", req.Product, item.Angle, req.Product)
}

func headline(req CampaignRequest, item automation.ItemResult) string {
	if req.Offer != "" {
		return req.Offer
	}
	return "Meet " + req.Product
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
