package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/automation"
)

func cmdContent(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("content requires: run")
	}
	switch args[0] {
	case "run":
		return cmdContentRun(ctx, args[1:])
	default:
		return fmt.Errorf("unknown content command %q", args[0])
	}
}

func cmdContentRun(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("automate", flag.ExitOnError)
	file := fs.String("file", "", "YAML or JSON content batch request")
	home := fs.String("home", "", "runtime home directory")
	outDir := fs.String("out-dir", "outputs/content", "content batch output directory")
	providerName := fs.String("provider", "", "provider override: mock, fal, runware")
	product := fs.String("product", "", "product for an inline content batch")
	goal := fs.String("goal", "", "campaign goal for an inline content batch")
	style := fs.String("style", "", "creative style for an inline content batch")
	platforms := fs.String("platforms", "", "comma-separated platforms for an inline content batch")
	duration := fs.Int("duration", 0, "duration in seconds for an inline content batch")
	variants := fs.Int("variants", 0, "number of variants for an inline content batch")
	dryRun := fs.Bool("dry-run", false, "write plans/reports but skip provider assets and rendering")
	render := fs.Bool("render", false, "render MP4 drafts with FFmpeg")
	if err := fs.Parse(args); err != nil {
		return err
	}

	req, err := contentRequestFromFlags(*file, *product, *goal, *style, *platforms, *duration, *variants)
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

	svc := automation.NewService(store, provider)
	result, runErr := svc.Run(ctx, automation.RunConfig{
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
	fmt.Printf("review_queue: %s\n", result.ReviewQueueMD)
	fmt.Println("posting_status: not_posted; manual approval required")
	return nil
}

func contentRequestFromFlags(file, product, goal, style, platforms string, duration, variants int) (automation.BatchRequest, error) {
	var req automation.BatchRequest
	var err error
	if strings.TrimSpace(file) != "" {
		req, err = automation.LoadRequest(file)
		if err != nil {
			return automation.BatchRequest{}, err
		}
	} else {
		req = automation.BatchRequest{Name: "content-batch"}
	}
	if strings.TrimSpace(product) != "" {
		req.Product = strings.TrimSpace(product)
	}
	if strings.TrimSpace(goal) != "" {
		req.Goal = strings.TrimSpace(goal)
	}
	if strings.TrimSpace(style) != "" {
		req.Style = strings.TrimSpace(style)
	}
	if list := splitCSV(platforms); len(list) > 0 {
		req.Platforms = list
	}
	if duration > 0 {
		req.DurationSec = duration
	}
	if variants > 0 {
		req.Variants = variants
	}
	if err := req.Normalize(); err != nil {
		return automation.BatchRequest{}, err
	}
	return req, nil
}

func firstNonEmptyLocal(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
