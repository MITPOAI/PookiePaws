package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/assets"
	"github.com/mitpoai/pookiepaws/internal/browser"
	"github.com/mitpoai/pookiepaws/internal/feedback"
	"github.com/mitpoai/pookiepaws/internal/memory"
	"github.com/mitpoai/pookiepaws/internal/planner"
	"github.com/mitpoai/pookiepaws/internal/providers"
	"github.com/mitpoai/pookiepaws/internal/providers/fal"
	"github.com/mitpoai/pookiepaws/internal/providers/mock"
	"github.com/mitpoai/pookiepaws/internal/providers/runware"
	"github.com/mitpoai/pookiepaws/internal/renderer"
)

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stdout)
		return
	}

	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "serve":
		cmdServe(os.Args[2:])
	case "init":
		err = cmdInit(ctx, os.Args[2:])
	case "create-ad":
		err = cmdCreateAd(ctx, os.Args[2:])
	case "automate":
		err = cmdContentRun(ctx, os.Args[2:])
	case "content":
		err = cmdContent(ctx, os.Args[2:])
	case "studio":
		err = cmdStudio(ctx, os.Args[2:])
	case "generate-image":
		err = cmdGenerateImage(ctx, os.Args[2:])
	case "generate-video":
		err = cmdGenerateVideo(ctx, os.Args[2:])
	case "render":
		err = cmdRender(ctx, os.Args[2:])
	case "memory":
		err = cmdMemory(ctx, os.Args[2:])
	case "feedback":
		err = cmdFeedback(ctx, os.Args[2:])
	case "browser":
		err = cmdBrowser(ctx, os.Args[2:])
	case "providers":
		err = cmdProviders(ctx, os.Args[2:])
	case "doctor":
		err = cmdDoctor(ctx, os.Args[2:])
	case "setup":
		err = cmdSetup(ctx, os.Args[2:])
	case "help", "--help", "-h":
		printUsage(os.Stdout)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "pookiepaws: %v\n", err)
		os.Exit(1)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: pookiepaws <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Ad automation MVP commands:")
	fmt.Fprintln(w, "  init                         Initialize local ad memory")
	fmt.Fprintln(w, "  create-ad                    Plan, generate mock assets, render, and save project history")
	fmt.Fprintln(w, "  automate                     Generate a batch of content drafts for manual review")
	fmt.Fprintln(w, "  content run                  Alias for automate")
	fmt.Fprintln(w, "  studio campaign              Create a local research, ads, and content workspace")
	fmt.Fprintln(w, "  generate-image               Generate one image asset through a provider")
	fmt.Fprintln(w, "  generate-video               Generate one video asset through a provider")
	fmt.Fprintln(w, "  render                       Render an edit_plan.json to video")
	fmt.Fprintln(w, "  memory show|update|search|export|reset")
	fmt.Fprintln(w, "  feedback add                 Save user feedback and reusable lessons")
	fmt.Fprintln(w, "  browser run|dry-run|open|record")
	fmt.Fprintln(w, "  providers test               Verify provider configuration")
	fmt.Fprintln(w, "  doctor                       Check local media/tooling readiness")
	fmt.Fprintln(w, "  setup check                  Alias for doctor")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Compatibility:")
	fmt.Fprintln(w, "  serve                        Start the existing local operator daemon")
}

func cmdInit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	home := fs.String("home", "", "runtime home directory")
	brandName := fs.String("brand-name", "", "brand name to save in memory")
	niche := fs.String("niche", "", "brand niche")
	tone := fs.String("tone", "", "brand tone")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := openMemoryStore(ctx, *home)
	if err != nil {
		return err
	}
	defer store.Close()

	profile, err := store.GetBrandProfile(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*brandName) != "" {
		profile.BrandName = strings.TrimSpace(*brandName)
	}
	if strings.TrimSpace(*niche) != "" {
		profile.Niche = strings.TrimSpace(*niche)
	}
	if strings.TrimSpace(*tone) != "" {
		profile.Tone = strings.TrimSpace(*tone)
	}
	if err := store.SaveBrandProfile(ctx, profile); err != nil {
		return err
	}

	fmt.Printf("initialized pookiepaws memory at %s\n", store.Path())
	return nil
}

func cmdCreateAd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("create-ad", flag.ExitOnError)
	home := fs.String("home", "", "runtime home directory")
	platform := fs.String("platform", "tiktok", "platform: tiktok, instagram, youtube-shorts, facebook")
	duration := fs.Int("duration", 15, "video duration in seconds")
	product := fs.String("product", "", "product or offer being advertised")
	style := fs.String("style", "cute motion graphics", "creative style")
	providerName := fs.String("provider", "mock", "provider: mock, fal, runware")
	userRequest := fs.String("request", "", "full natural-language request")
	outDir := fs.String("out-dir", "outputs", "directory for generated project outputs")
	dryRun := fs.Bool("dry-run", false, "plan and save files without calling media renderer")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*product) == "" && strings.TrimSpace(*userRequest) == "" {
		return errors.New("create-ad requires --product or --request")
	}
	if *duration < 3 {
		return errors.New("--duration must be at least 3 seconds")
	}

	store, err := openMemoryStore(ctx, *home)
	if err != nil {
		return err
	}
	defer store.Close()

	profile, err := store.GetBrandProfile(ctx)
	if err != nil {
		return err
	}

	req := planner.Request{
		Platform:    *platform,
		DurationSec: *duration,
		Product:     strings.TrimSpace(*product),
		Style:       strings.TrimSpace(*style),
		UserRequest: strings.TrimSpace(*userRequest),
	}
	if req.UserRequest == "" {
		req.UserRequest = fmt.Sprintf("Create a %d-second %s commercial for %s in a %s style.", req.DurationSec, req.Platform, req.Product, req.Style)
	}
	if req.Product == "" {
		req.Product = "the product"
	}

	adPlan, err := planner.PlanAd(profile, req)
	if err != nil {
		return err
	}

	projectID := assets.NewProjectID(req.Product, time.Now())
	dirs, err := assets.EnsureProjectDirs(*outDir, projectID)
	if err != nil {
		return err
	}

	provider, err := selectProvider(*providerName)
	if err != nil {
		return err
	}

	backgrounds := make(map[string]string, len(adPlan.Storyboard.Scenes))
	promptsUsed := make([]string, 0, len(adPlan.ImagePrompts)+len(adPlan.VideoPrompts))
	for _, scene := range adPlan.Storyboard.Scenes {
		asset, err := provider.GenerateImage(ctx, scene.VisualPrompt, providers.ImageOptions{
			OutputDir: dirs.Assets,
			Width:     1080,
			Height:    1920,
			Format:    "png",
			DryRun:    *dryRun,
		})
		if err != nil {
			return fmt.Errorf("generate asset for scene %s: %w", scene.ID, err)
		}
		backgrounds[scene.ID] = asset.Path
		promptsUsed = append(promptsUsed, scene.VisualPrompt)
	}
	promptsUsed = append(promptsUsed, adPlan.VideoPrompts...)

	editPlan := planner.ToEditPlan(adPlan, backgrounds)
	editPlanPath := filepath.Join(dirs.Root, "edit_plan.json")
	if err := renderer.SavePlan(editPlanPath, editPlan); err != nil {
		return err
	}

	finalPath := filepath.Join(dirs.Outputs, "final.mp4")
	renderStatus := "skipped"
	if !*dryRun {
		if err := renderer.Render(ctx, editPlanPath, finalPath, renderer.RenderOptions{
			ScriptPath: filepath.Join(repoRoot(), "scripts", "media", "render.py"),
		}); err != nil {
			return fmt.Errorf("render draft video: %w", err)
		}
		renderStatus = "rendered"
	}

	reviewPath, err := writeReviewReport(dirs.Reports, projectID, req, adPlan, editPlanPath, finalPath, renderStatus)
	if err != nil {
		return err
	}

	history := memory.ProjectHistory{
		ID:               projectID,
		CreatedAt:        time.Now(),
		UserRequest:      req.UserRequest,
		Platform:         req.Platform,
		DurationSec:      req.DurationSec,
		Provider:         provider.Name(),
		GeneratedBrief:   adPlan.Brief,
		PromptsUsed:      promptsUsed,
		ModelUsed:        provider.Name(),
		EditPlanPath:     editPlanPath,
		FinalOutputPath:  finalPath,
		ReviewReportPath: reviewPath,
	}
	if err := store.SaveProject(ctx, history); err != nil {
		return err
	}

	fmt.Printf("project_id: %s\n", projectID)
	fmt.Printf("storyboard: %d scenes\n", len(adPlan.Storyboard.Scenes))
	fmt.Printf("edit_plan: %s\n", editPlanPath)
	fmt.Printf("video: %s\n", finalPath)
	fmt.Printf("review_report: %s\n", reviewPath)
	fmt.Printf("next: pookiepaws feedback add --project-id %s --score 5 --lessons \"what worked\"\n", projectID)
	return nil
}

func cmdGenerateImage(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("generate-image", flag.ExitOnError)
	providerName := fs.String("provider", "mock", "provider: mock, fal, runware")
	prompt := fs.String("prompt", "", "image prompt")
	outDir := fs.String("out-dir", "outputs/assets", "asset output directory")
	width := fs.Int("width", 1080, "image width")
	height := fs.Int("height", 1920, "image height")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*prompt) == "" {
		return errors.New("generate-image requires --prompt")
	}
	provider, err := selectProvider(*providerName)
	if err != nil {
		return err
	}
	asset, err := provider.GenerateImage(ctx, *prompt, providers.ImageOptions{
		OutputDir: *outDir,
		Width:     *width,
		Height:    *height,
		Format:    "png",
	})
	if err != nil {
		return err
	}
	printJSON(asset)
	return nil
}

func cmdGenerateVideo(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("generate-video", flag.ExitOnError)
	providerName := fs.String("provider", "mock", "provider: mock, fal, runware")
	prompt := fs.String("prompt", "", "video prompt")
	imageInput := fs.String("image-input", "", "optional starting image")
	outDir := fs.String("out-dir", "outputs/assets", "asset output directory")
	duration := fs.Int("duration", 4, "duration in seconds")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*prompt) == "" {
		return errors.New("generate-video requires --prompt")
	}
	provider, err := selectProvider(*providerName)
	if err != nil {
		return err
	}
	asset, err := provider.GenerateVideo(ctx, *prompt, *imageInput, providers.VideoOptions{
		OutputDir:   *outDir,
		DurationSec: *duration,
		Width:       1080,
		Height:      1920,
	})
	if err != nil {
		return err
	}
	printJSON(asset)
	return nil
}

func cmdRender(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	planPath := fs.String("plan", "", "edit plan JSON path")
	outPath := fs.String("out", "outputs/final.mp4", "output video path")
	dryRun := fs.Bool("dry-run", false, "validate plan but skip ffmpeg")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*planPath) == "" {
		return errors.New("render requires --plan")
	}
	if *dryRun {
		_, err := renderer.LoadPlan(*planPath)
		if err != nil {
			return err
		}
		fmt.Printf("valid edit plan: %s\n", *planPath)
		return nil
	}
	if err := renderer.Render(ctx, *planPath, *outPath, renderer.RenderOptions{
		ScriptPath: filepath.Join(repoRoot(), "scripts", "media", "render.py"),
	}); err != nil {
		return err
	}
	fmt.Printf("rendered: %s\n", *outPath)
	return nil
}

func cmdMemory(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("memory requires show, update, search, export, or reset")
	}
	switch args[0] {
	case "show":
		return cmdMemoryShow(ctx, args[1:])
	case "update":
		return cmdMemoryUpdate(ctx, args[1:])
	case "search":
		return cmdMemorySearch(ctx, args[1:])
	case "export":
		return cmdMemoryExport(ctx, args[1:])
	case "reset":
		return cmdMemoryReset(ctx, args[1:])
	default:
		return fmt.Errorf("unknown memory command %q", args[0])
	}
}

func cmdMemoryShow(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("memory show", flag.ExitOnError)
	home := fs.String("home", "", "runtime home directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := openMemoryStore(ctx, *home)
	if err != nil {
		return err
	}
	defer store.Close()
	profile, err := store.GetBrandProfile(ctx)
	if err != nil {
		return err
	}
	projects, err := store.ListProjects(ctx, 10)
	if err != nil {
		return err
	}
	printJSON(map[string]any{
		"brand_profile":   profile,
		"recent_projects": projects,
	})
	return nil
}

func cmdMemoryUpdate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("memory update", flag.ExitOnError)
	home := fs.String("home", "", "runtime home directory")
	brandName := fs.String("brand-name", "", "brand name")
	niche := fs.String("niche", "", "niche")
	colors := fs.String("colors", "", "comma-separated colors")
	fonts := fs.String("fonts", "", "comma-separated fonts")
	tone := fs.String("tone", "", "tone")
	targetAudience := fs.String("target-audience", "", "target audience")
	videoStyle := fs.String("preferred-video-style", "", "preferred video style")
	ctaStyle := fs.String("preferred-cta-style", "", "preferred CTA style")
	bannedWords := fs.String("banned-words", "", "comma-separated banned words")
	bannedStyles := fs.String("banned-styles", "", "comma-separated banned styles")
	platform := fs.String("platform", "", "platform preference key")
	platformStyle := fs.String("platform-style", "", "style note for --platform")
	platformCTA := fs.String("platform-cta", "", "CTA note for --platform")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := openMemoryStore(ctx, *home)
	if err != nil {
		return err
	}
	defer store.Close()
	profile, err := store.GetBrandProfile(ctx)
	if err != nil {
		return err
	}

	setString(&profile.BrandName, *brandName)
	setString(&profile.Niche, *niche)
	setString(&profile.Tone, *tone)
	setString(&profile.TargetAudience, *targetAudience)
	setString(&profile.PreferredVideoStyle, *videoStyle)
	setString(&profile.PreferredCTAStyle, *ctaStyle)
	if list := splitCSV(*colors); len(list) > 0 {
		profile.Colors = list
	}
	if list := splitCSV(*fonts); len(list) > 0 {
		profile.Fonts = list
	}
	if list := splitCSV(*bannedWords); len(list) > 0 {
		profile.BannedWords = list
	}
	if list := splitCSV(*bannedStyles); len(list) > 0 {
		profile.BannedStyles = list
	}
	if strings.TrimSpace(*platform) != "" {
		key := strings.ToLower(strings.TrimSpace(*platform))
		pref := profile.PlatformPreferences[key]
		setString(&pref.Style, *platformStyle)
		setString(&pref.CTAStyle, *platformCTA)
		profile.PlatformPreferences[key] = pref
	}

	if err := store.SaveBrandProfile(ctx, profile); err != nil {
		return err
	}
	fmt.Println("memory updated")
	return nil
}

func cmdMemorySearch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("memory search", flag.ExitOnError)
	home := fs.String("home", "", "runtime home directory")
	query := fs.String("query", "", "search query")
	limit := fs.Int("limit", 10, "maximum results")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *query == "" && fs.NArg() > 0 {
		*query = strings.Join(fs.Args(), " ")
	}
	if strings.TrimSpace(*query) == "" {
		return errors.New("memory search requires --query or query text")
	}
	store, err := openMemoryStore(ctx, *home)
	if err != nil {
		return err
	}
	defer store.Close()
	results, err := store.Search(ctx, *query, *limit)
	if err != nil {
		return err
	}
	printJSON(results)
	return nil
}

func cmdMemoryExport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("memory export", flag.ExitOnError)
	home := fs.String("home", "", "runtime home directory")
	out := fs.String("out", "", "output JSON path; stdout when omitted")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := openMemoryStore(ctx, *home)
	if err != nil {
		return err
	}
	defer store.Close()
	if strings.TrimSpace(*out) == "" {
		return store.Export(ctx, os.Stdout)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := store.Export(ctx, f); err != nil {
		return err
	}
	fmt.Printf("exported memory: %s\n", *out)
	return nil
}

func cmdMemoryReset(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("memory reset", flag.ExitOnError)
	home := fs.String("home", "", "runtime home directory")
	yes := fs.Bool("yes", false, "confirm destructive reset")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*yes {
		return errors.New("memory reset requires --yes")
	}
	store, err := openMemoryStore(ctx, *home)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Reset(ctx); err != nil {
		return err
	}
	fmt.Println("memory reset")
	return nil
}

func cmdFeedback(ctx context.Context, args []string) error {
	if len(args) < 1 || args[0] != "add" {
		return errors.New("feedback requires: add")
	}
	fs := flag.NewFlagSet("feedback add", flag.ExitOnError)
	home := fs.String("home", "", "runtime home directory")
	projectID := fs.String("project-id", "", "project ID")
	score := fs.Int("score", 0, "feedback score from 1 to 5")
	corrections := fs.String("corrections", "", "user corrections")
	lessons := fs.String("lessons", "", "lessons to remember")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*projectID) == "" {
		return errors.New("feedback add requires --project-id")
	}
	store, err := openMemoryStore(ctx, *home)
	if err != nil {
		return err
	}
	defer store.Close()
	svc := feedback.NewService(store)
	item, err := svc.Add(ctx, *projectID, *score, *corrections, *lessons)
	if err != nil {
		return err
	}
	printJSON(item)
	return nil
}

func cmdBrowser(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("browser requires run, dry-run, open, or record")
	}
	switch args[0] {
	case "run":
		return cmdBrowserRun(ctx, args[1:], false)
	case "dry-run":
		return cmdBrowserRun(ctx, args[1:], true)
	case "open":
		fs := flag.NewFlagSet("browser open", flag.ExitOnError)
		url := fs.String("url", "", "URL to open")
		dryRun := fs.Bool("dry-run", true, "preview only in MVP")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return browser.Open(ctx, *url, *dryRun)
	case "record":
		return browser.Record(ctx)
	default:
		return fmt.Errorf("unknown browser command %q", args[0])
	}
}

func cmdBrowserRun(ctx context.Context, args []string, forceDryRun bool) error {
	fs := flag.NewFlagSet("browser run", flag.ExitOnError)
	workflow := fs.String("workflow", "", "workflow YAML path")
	dryRun := fs.Bool("dry-run", forceDryRun, "preview workflow without browser control")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *workflow == "" && fs.NArg() > 0 {
		*workflow = fs.Arg(0)
	}
	if strings.TrimSpace(*workflow) == "" {
		return errors.New("browser run requires --workflow or a workflow path")
	}
	result, err := browser.RunWorkflow(ctx, *workflow, *dryRun || forceDryRun)
	if err != nil {
		return err
	}
	printJSON(result)
	return nil
}

func cmdProviders(ctx context.Context, args []string) error {
	if len(args) < 1 || args[0] != "test" {
		return errors.New("providers requires: test")
	}
	fs := flag.NewFlagSet("providers test", flag.ExitOnError)
	providerName := fs.String("provider", "mock", "provider: mock, fal, runware")
	outDir := fs.String("out-dir", filepath.Join(os.TempDir(), "pookiepaws-provider-test"), "output directory")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	provider, err := selectProvider(*providerName)
	if err != nil {
		return err
	}
	status, err := provider.GetTaskStatus(ctx, "healthcheck")
	if err == nil && provider.Name() != "mock" {
		printJSON(status)
		return nil
	}
	if provider.Name() != "mock" {
		return err
	}
	asset, err := provider.GenerateImage(ctx, "provider healthcheck test image", providers.ImageOptions{
		OutputDir: *outDir,
		Width:     320,
		Height:    568,
		Format:    "png",
	})
	if err != nil {
		return err
	}
	printJSON(map[string]any{
		"provider": provider.Name(),
		"ok":       true,
		"asset":    asset,
	})
	return nil
}

func openMemoryStore(ctx context.Context, homeOverride string) (*memory.Store, error) {
	home, err := resolveAdHome(homeOverride)
	if err != nil {
		return nil, err
	}
	store, err := memory.Open(filepath.Join(home, "memory", "pookiepaws.db"))
	if err != nil {
		return nil, err
	}
	if err := store.Initialize(ctx); err != nil {
		store.Close()
		return nil, err
	}
	return store, nil
}

func resolveAdHome(homeOverride string) (string, error) {
	if strings.TrimSpace(homeOverride) != "" {
		return filepath.Abs(homeOverride)
	}
	if env := strings.TrimSpace(os.Getenv("POOKIEPAWS_HOME")); env != "" {
		return filepath.Abs(env)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pookiepaws"), nil
}

func selectProvider(name string) (providers.Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "mock":
		return mock.New(), nil
	case "fal", "fal.ai":
		return fal.NewFromEnv(), nil
	case "runware":
		return runware.NewFromEnv(), nil
	default:
		return nil, fmt.Errorf("unknown provider %q", name)
	}
}

func writeReviewReport(reportDir, projectID string, req planner.Request, plan planner.Plan, editPlanPath, finalPath, renderStatus string) (string, error) {
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(reportDir, "review.md")
	var b strings.Builder
	b.WriteString("# PookiePaws Ad Review\n\n")
	b.WriteString("Project ID: " + projectID + "\n\n")
	b.WriteString("## Request\n\n")
	b.WriteString("- Platform: " + req.Platform + "\n")
	b.WriteString("- Duration: " + strconv.Itoa(req.DurationSec) + " seconds\n")
	b.WriteString("- Product: " + req.Product + "\n")
	b.WriteString("- Style: " + req.Style + "\n\n")
	b.WriteString("## Strategy\n\n")
	b.WriteString("- Hook: " + plan.Strategy.Hook + "\n")
	b.WriteString("- Problem: " + plan.Strategy.Problem + "\n")
	b.WriteString("- Solution: " + plan.Strategy.Solution + "\n")
	b.WriteString("- Benefit: " + plan.Strategy.Benefit + "\n")
	b.WriteString("- Proof: " + plan.Strategy.Proof + "\n")
	b.WriteString("- CTA: " + plan.Strategy.CTA + "\n\n")
	b.WriteString("## Output\n\n")
	b.WriteString("- Render status: " + renderStatus + "\n")
	b.WriteString("- Edit plan: " + editPlanPath + "\n")
	b.WriteString("- Video: " + finalPath + "\n\n")
	b.WriteString("## Feedback Prompt\n\n")
	b.WriteString("Run `pookiepaws feedback add --project-id " + projectID + " --score 1..5 --corrections \"...\" --lessons \"...\"` after review.\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func setString(target *string, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		*target = value
	}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func repoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "."
		}
		wd = parent
	}
}
