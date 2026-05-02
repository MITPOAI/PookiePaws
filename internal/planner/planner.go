package planner

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/memory"
	"github.com/mitpoai/pookiepaws/internal/renderer"
	"github.com/mitpoai/pookiepaws/internal/storyboard"
)

func PlanAd(profile memory.BrandProfile, req Request) (Plan, error) {
	req.Platform = normalizePlatform(req.Platform)
	req.Style = strings.TrimSpace(req.Style)
	req.Product = strings.TrimSpace(req.Product)
	req.UserRequest = strings.TrimSpace(req.UserRequest)
	if req.DurationSec <= 0 {
		req.DurationSec = 15
	}
	if req.Product == "" {
		return Plan{}, errors.New("product is required")
	}
	if req.Style == "" {
		req.Style = firstNonEmpty(profile.PreferredVideoStyle, "motion graphics")
	}

	pref := profile.PlatformPreferences[req.Platform]
	format := defaultFormat(req.Platform)
	strategy := Strategy{
		Hook:     fmt.Sprintf("Stop scrolling for %s.", req.Product),
		Problem:  fmt.Sprintf("Show the everyday friction %s solves.", req.Product),
		Solution: fmt.Sprintf("Position %s as the simple, cute fix.", req.Product),
		Benefit:  fmt.Sprintf("Make the core benefit obvious in one glance for %s.", firstNonEmpty(profile.TargetAudience, "mobile viewers")),
		Proof:    "Use visual proof points, product shots, and plain-language claims only.",
		CTA:      firstNonEmpty(pref.CTAStyle, profile.PreferredCTAStyle, "Shop now"),
	}
	brief := fmt.Sprintf(
		"%s ad for %s: %d seconds, %s format, %s style, tone %q, audience %q.",
		strings.Title(req.Platform),
		req.Product,
		req.DurationSec,
		format,
		req.Style,
		firstNonEmpty(profile.Tone, "upbeat"),
		firstNonEmpty(profile.TargetAudience, "mobile-first viewers"),
	)

	sceneDur := float64(req.DurationSec) / 3
	colors := strings.Join(profile.Colors, ", ")
	fonts := strings.Join(profile.Fonts, ", ")
	basePrompt := fmt.Sprintf(
		"commercial social ad still for %s, %s, brand colors %s, fonts %s, tone %s, clean product shot, no deceptive claims",
		req.Product,
		req.Style,
		firstNonEmpty(colors, "bright contrasting colors"),
		firstNonEmpty(fonts, "bold sans serif"),
		firstNonEmpty(profile.Tone, "cute and direct"),
	)

	scenes := []storyboard.Scene{
		{
			ID:           "scene-1-hook",
			Title:        "Hook",
			Objective:    "Earn attention immediately.",
			Start:        0,
			End:          round(sceneDur),
			VisualPrompt: basePrompt + ", first frame hook, playful background, product silhouette, bold negative space for caption",
			VideoPrompt:  fmt.Sprintf("Fast pop-in motion for %s with a thumb-stopping hook and kinetic captions.", req.Product),
			Caption:      "Stop scrolling!",
			Subcaption:   fmt.Sprintf("Meet %s", req.Product),
			Animation:    "pop",
		},
		{
			ID:           "scene-2-benefit",
			Title:        "Benefit",
			Objective:    "Show the problem-to-solution transformation.",
			Start:        round(sceneDur),
			End:          round(sceneDur * 2),
			VisualPrompt: basePrompt + ", product hero shot, before-after graphic shapes, bright motion graphic panels",
			VideoPrompt:  fmt.Sprintf("Slide transition into %s benefit reveal with clean product close-up.", req.Product),
			Caption:      "Cute fix. Clear benefit.",
			Subcaption:   "Designed for everyday use",
			Animation:    "slide",
		},
		{
			ID:           "scene-3-cta",
			Title:        "CTA End Card",
			Objective:    "Make the next action explicit.",
			Start:        round(sceneDur * 2),
			End:          float64(req.DurationSec),
			VisualPrompt: basePrompt + ", CTA end card, logo-safe empty top area, clean button-like call to action",
			VideoPrompt:  fmt.Sprintf("CTA end card for %s with bounce text, logo lockup area, and final product beat.", req.Product),
			Caption:      strings.TrimSuffix(strategy.CTA, "."),
			Subcaption:   "Ready when you are",
			CTA:          strategy.CTA,
			Animation:    "bounce",
		},
	}

	imagePrompts := make([]string, 0, len(scenes))
	videoPrompts := make([]string, 0, len(scenes))
	for _, scene := range scenes {
		imagePrompts = append(imagePrompts, scene.VisualPrompt)
		videoPrompts = append(videoPrompts, scene.VideoPrompt)
	}

	return Plan{
		Request:      req,
		Brief:        brief,
		Strategy:     strategy,
		Storyboard:   storyboard.Storyboard{Platform: req.Platform, Format: format, DurationSec: req.DurationSec, Scenes: scenes},
		ImagePrompts: imagePrompts,
		VideoPrompts: videoPrompts,
		Metadata: map[string]interface{}{
			"brand_name":          profile.BrandName,
			"platform_preference": pref,
			"deterministic":       true,
		},
	}, nil
}

func ToEditPlan(plan Plan, backgroundByScene map[string]string) renderer.EditPlan {
	width, height := renderer.DimensionsForFormat(plan.Storyboard.Format)
	scenes := make([]renderer.Scene, 0, len(plan.Storyboard.Scenes))
	for i, scene := range plan.Storyboard.Scenes {
		scenes = append(scenes, renderer.Scene{
			ID:              scene.ID,
			Start:           scene.Start,
			End:             scene.End,
			Background:      backgroundByScene[scene.ID],
			BackgroundColor: sceneColor(i),
			Text:            scene.Caption,
			Subtext:         scene.Subcaption,
			Animation:       scene.Animation,
			CTA:             scene.CTA,
		})
	}
	return renderer.EditPlan{
		Format:   plan.Storyboard.Format,
		Width:    width,
		Height:   height,
		FPS:      30,
		Duration: float64(plan.Storyboard.DurationSec),
		Scenes:   scenes,
		Audio: renderer.AudioPlan{
			MusicPlaceholder:       "Add licensed upbeat background music before publishing.",
			SoundEffectPlaceholder: "Add soft pop/whoosh effects on scene changes.",
		},
		Export: renderer.ExportSettings{
			Codec:       "libx264",
			PixelFormat: "yuv420p",
			CRF:         20,
		},
	}
}

func normalizePlatform(platform string) string {
	platform = strings.ToLower(strings.TrimSpace(platform))
	switch platform {
	case "", "tiktok", "tik tok":
		return "tiktok"
	case "ig", "reels", "instagram-reels":
		return "instagram"
	case "shorts", "youtube", "youtube_short", "youtube-shorts":
		return "youtube-shorts"
	case "fb", "facebook-ads", "facebook_ad":
		return "facebook"
	default:
		return platform
	}
}

func defaultFormat(platform string) string {
	switch normalizePlatform(platform) {
	case "facebook":
		return "1:1"
	default:
		return "9:16"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func round(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

func sceneColor(i int) string {
	colors := []string{"#202124", "#ff4fa3", "#0f766e"}
	return colors[i%len(colors)]
}
