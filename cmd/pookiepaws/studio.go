package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/studio"
)

func cmdStudio(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("studio requires: campaign")
	}
	switch args[0] {
	case "campaign":
		return cmdStudioCampaign(ctx, args[1:])
	default:
		return fmt.Errorf("unknown studio command %q", args[0])
	}
}

func cmdStudioCampaign(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("studio campaign", flag.ExitOnError)
	file := fs.String("file", "", "YAML or JSON studio campaign request")
	home := fs.String("home", "", "runtime home directory")
	outDir := fs.String("out-dir", "outputs/studio", "studio output directory")
	providerName := fs.String("provider", "", "provider override: mock, fal, runware")
	brandName := fs.String("brand-name", "", "brand name")
	product := fs.String("product", "", "product")
	niche := fs.String("niche", "", "niche")
	goal := fs.String("goal", "", "campaign goal")
	offer := fs.String("offer", "", "offer or promotion")
	audience := fs.String("target-audience", "", "target audience")
	competitors := fs.String("competitors", "", "comma-separated competitors")
	platforms := fs.String("platforms", "", "comma-separated platforms")
	variants := fs.Int("variants", 0, "number of content variants")
	duration := fs.Int("duration", 0, "content duration in seconds")
	style := fs.String("style", "", "creative style")
	dryRun := fs.Bool("dry-run", false, "write plans/reports but skip provider assets and rendering")
	render := fs.Bool("render", false, "render MP4 drafts with FFmpeg")
	if err := fs.Parse(args); err != nil {
		return err
	}

	req, err := studioRequestFromFlags(*file, *brandName, *product, *niche, *goal, *offer, *audience, *competitors, *platforms, *variants, *duration, *style)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*providerName) == "" {
		*providerName = firstNonEmptyLocal(req.Provider, "mock")
	}
	provider, err := selectProvider(*providerName)
	if err != nil {
		return err
	}
	store, err := openMemoryStore(ctx, *home)
	if err != nil {
		return err
	}
	defer store.Close()

	svc := studio.NewService(store, provider)
	result, runErr := svc.Run(ctx, studio.RunConfig{
		Request:          req,
		OutputDir:        *outDir,
		ProviderOverride: *providerName,
		DryRun:           *dryRun,
		Render:           *render,
		RenderScriptPath: filepath.Join(repoRoot(), "scripts", "media", "render.py"),
	})
	printJSON(result)
	if runErr != nil {
		return runErr
	}
	fmt.Printf("studio_workspace: %s\n", result.OutputDir)
	fmt.Printf("research_brief: %s\n", result.ResearchBriefPath)
	fmt.Printf("ad_export: %s\n", result.AdExportPath)
	fmt.Printf("review_queue: %s\n", result.Content.ReviewQueueMD)
	fmt.Println("posting_status: not_posted; manual approval required")
	return nil
}

func studioRequestFromFlags(file, brandName, product, niche, goal, offer, audience, competitors, platforms string, variants, duration int, style string) (studio.CampaignRequest, error) {
	var req studio.CampaignRequest
	var err error
	if strings.TrimSpace(file) != "" {
		req, err = studio.LoadRequest(file)
		if err != nil {
			return studio.CampaignRequest{}, err
		}
	} else {
		req = studio.CampaignRequest{Name: "studio-campaign"}
	}
	if strings.TrimSpace(brandName) != "" {
		req.BrandName = strings.TrimSpace(brandName)
	}
	if strings.TrimSpace(product) != "" {
		req.Product = strings.TrimSpace(product)
	}
	if strings.TrimSpace(niche) != "" {
		req.Niche = strings.TrimSpace(niche)
	}
	if strings.TrimSpace(goal) != "" {
		req.Goal = strings.TrimSpace(goal)
	}
	if strings.TrimSpace(offer) != "" {
		req.Offer = strings.TrimSpace(offer)
	}
	if strings.TrimSpace(audience) != "" {
		req.TargetAudience = strings.TrimSpace(audience)
	}
	if list := splitCSV(competitors); len(list) > 0 {
		req.Competitors = list
	}
	if list := splitCSV(platforms); len(list) > 0 {
		req.Platforms = list
	}
	if variants > 0 {
		req.ContentVariants = variants
	}
	if duration > 0 {
		req.DurationSec = duration
	}
	if strings.TrimSpace(style) != "" {
		req.Style = strings.TrimSpace(style)
	}
	if err := req.Normalize(); err != nil {
		return studio.CampaignRequest{}, err
	}
	return req, nil
}
