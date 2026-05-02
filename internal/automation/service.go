package automation

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
	"github.com/mitpoai/pookiepaws/internal/memory"
	"github.com/mitpoai/pookiepaws/internal/planner"
	"github.com/mitpoai/pookiepaws/internal/providers"
	"github.com/mitpoai/pookiepaws/internal/renderer"
)

type Service struct {
	store    *memory.Store
	provider providers.Provider
}

func NewService(store *memory.Store, provider providers.Provider) Service {
	return Service{store: store, provider: provider}
}

func (s Service) Run(ctx context.Context, cfg RunConfig) (BatchResult, error) {
	if s.store == nil {
		return BatchResult{}, errors.New("automation requires a memory store")
	}
	if s.provider == nil {
		return BatchResult{}, errors.New("automation requires a provider")
	}
	if err := cfg.Request.Normalize(); err != nil {
		return BatchResult{}, err
	}
	now := time.Now
	if cfg.Now != nil {
		now = cfg.Now
	}
	createdAt := now().UTC()
	batchID := assets.NewProjectID(cfg.Request.Name, createdAt)
	outputDir := cfg.OutputDir
	if strings.TrimSpace(outputDir) == "" {
		outputDir = "outputs/content"
	}
	root := filepath.Join(outputDir, batchID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return BatchResult{}, err
	}

	profile, err := s.store.GetBrandProfile(ctx)
	if err != nil {
		return BatchResult{}, err
	}

	result := BatchResult{
		BatchID:          batchID,
		Name:             cfg.Request.Name,
		OutputDir:        root,
		ReviewQueueJSON:  filepath.Join(root, "review_queue.json"),
		ReviewQueueMD:    filepath.Join(root, "review_queue.md"),
		RequiresApproval: true,
		CreatedAt:        createdAt,
	}

	requestPath := filepath.Join(root, "batch_request.json")
	if err := writeJSON(requestPath, cfg.Request); err != nil {
		return BatchResult{}, err
	}

	var firstErr error
	for _, item := range cfg.Request.Items {
		itemResult := s.runItem(ctx, profile, cfg, batchID, root, item, now)
		if itemResult.Error != "" && firstErr == nil {
			firstErr = errors.New(itemResult.Error)
		}
		result.Items = append(result.Items, itemResult)
	}

	if err := writeJSON(result.ReviewQueueJSON, result); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := writeReviewQueue(result.ReviewQueueMD, cfg.Request, result); err != nil && firstErr == nil {
		firstErr = err
	}
	return result, firstErr
}

func (s Service) runItem(ctx context.Context, profile memory.BrandProfile, cfg RunConfig, batchID, root string, item ContentItem, now func() time.Time) ItemResult {
	projectID := assets.NewProjectID(batchID+"-"+item.ID, now())
	result := ItemResult{
		ID:           item.ID,
		ProjectID:    projectID,
		Platform:     item.Platform,
		Product:      item.Product,
		Angle:        item.Angle,
		Status:       "planned",
		RenderStatus: "skipped",
	}

	dirs, err := assets.EnsureProjectDirs(root, projectID)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result
	}

	req := planner.Request{
		Platform:    item.Platform,
		DurationSec: item.DurationSec,
		Product:     item.Product,
		Style:       item.Style,
		UserRequest: userRequest(cfg.Request, item),
	}
	adPlan, err := planner.PlanAd(profile, req)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result
	}

	backgrounds := make(map[string]string, len(adPlan.Storyboard.Scenes))
	promptsUsed := make([]string, 0, len(adPlan.ImagePrompts)+len(adPlan.VideoPrompts))
	for _, scene := range adPlan.Storyboard.Scenes {
		asset, err := s.provider.GenerateImage(ctx, scene.VisualPrompt+", content angle: "+item.Angle, providers.ImageOptions{
			OutputDir: dirs.Assets,
			Width:     1080,
			Height:    1920,
			Format:    "png",
			DryRun:    cfg.DryRun,
		})
		if err != nil {
			result.Status = "failed"
			result.Error = fmt.Sprintf("generate asset for %s: %v", scene.ID, err)
			return result
		}
		backgrounds[scene.ID] = asset.Path
		result.AssetPaths = append(result.AssetPaths, asset.Path)
		promptsUsed = append(promptsUsed, scene.VisualPrompt)
	}
	promptsUsed = append(promptsUsed, adPlan.VideoPrompts...)

	editPlan := planner.ToEditPlan(adPlan, backgrounds)
	result.EditPlanPath = filepath.Join(dirs.Root, "edit_plan.json")
	if err := renderer.SavePlan(result.EditPlanPath, editPlan); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result
	}

	result.FinalOutputPath = filepath.Join(dirs.Outputs, "final.mp4")
	if cfg.Render && !cfg.DryRun {
		if err := renderer.Render(ctx, result.EditPlanPath, result.FinalOutputPath, renderer.RenderOptions{ScriptPath: cfg.RenderScriptPath}); err != nil {
			result.Status = "failed"
			result.RenderStatus = "failed"
			result.Error = fmt.Sprintf("render: %v", err)
			return result
		}
		result.RenderStatus = "rendered"
	}

	reviewPath, err := writeItemReview(dirs.Reports, item, req, adPlan, result)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result
	}
	result.ReviewReportPath = reviewPath
	result.Status = "ready_for_review"

	history := memory.ProjectHistory{
		ID:               result.ProjectID,
		CreatedAt:        now().UTC(),
		UserRequest:      req.UserRequest,
		Platform:         req.Platform,
		DurationSec:      req.DurationSec,
		Provider:         s.provider.Name(),
		GeneratedBrief:   adPlan.Brief,
		PromptsUsed:      promptsUsed,
		ModelUsed:        s.provider.Name(),
		EditPlanPath:     result.EditPlanPath,
		FinalOutputPath:  result.FinalOutputPath,
		ReviewReportPath: result.ReviewReportPath,
	}
	if err := s.store.SaveProject(ctx, history); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
	}
	return result
}

func userRequest(batch BatchRequest, item ContentItem) string {
	parts := []string{
		fmt.Sprintf("Create a %d-second %s commercial for %s.", item.DurationSec, item.Platform, item.Product),
		"Style: " + item.Style + ".",
	}
	if batch.Goal != "" {
		parts = append(parts, "Campaign goal: "+batch.Goal+".")
	}
	if item.Angle != "" {
		parts = append(parts, "Creative angle: "+item.Angle+".")
	}
	if len(item.Notes) > 0 {
		parts = append(parts, "Notes: "+strings.Join(item.Notes, "; ")+".")
	}
	return strings.Join(parts, " ")
}

func writeItemReview(reportDir string, item ContentItem, req planner.Request, plan planner.Plan, result ItemResult) (string, error) {
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(reportDir, "review.md")
	var b strings.Builder
	b.WriteString("# Content Draft Review\n\n")
	b.WriteString("- Project ID: " + result.ProjectID + "\n")
	b.WriteString("- Item ID: " + item.ID + "\n")
	b.WriteString("- Platform: " + req.Platform + "\n")
	b.WriteString("- Product: " + req.Product + "\n")
	b.WriteString("- Angle: " + item.Angle + "\n")
	b.WriteString("- Render status: " + result.RenderStatus + "\n\n")
	b.WriteString("## Brief\n\n")
	b.WriteString(plan.Brief + "\n\n")
	b.WriteString("## Strategy\n\n")
	b.WriteString("- Hook: " + plan.Strategy.Hook + "\n")
	b.WriteString("- Problem: " + plan.Strategy.Problem + "\n")
	b.WriteString("- Solution: " + plan.Strategy.Solution + "\n")
	b.WriteString("- Benefit: " + plan.Strategy.Benefit + "\n")
	b.WriteString("- Proof: " + plan.Strategy.Proof + "\n")
	b.WriteString("- CTA: " + plan.Strategy.CTA + "\n\n")
	b.WriteString("## Review Action\n\n")
	b.WriteString("Approve manually, request changes, or save feedback with `pookiepaws feedback add --project-id " + result.ProjectID + " --score 1..5 --lessons \"...\"`.\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func writeReviewQueue(path string, req BatchRequest, result BatchResult) error {
	var b strings.Builder
	b.WriteString("# Content Review Queue\n\n")
	b.WriteString("- Batch: " + result.BatchID + "\n")
	b.WriteString("- Name: " + req.Name + "\n")
	b.WriteString("- Product: " + req.Product + "\n")
	b.WriteString("- Goal: " + req.Goal + "\n")
	b.WriteString("- Manual approval required: yes\n")
	b.WriteString("- Posting status: not posted\n\n")
	b.WriteString("| Item | Platform | Status | Render | Project | Review |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, item := range result.Items {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
			item.ID,
			item.Platform,
			item.Status,
			item.RenderStatus,
			item.ProjectID,
			item.ReviewReportPath,
		))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
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
