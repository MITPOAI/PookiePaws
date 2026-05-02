package planner

import (
	"testing"

	"github.com/mitpoai/pookiepaws/internal/memory"
)

func TestPlanAdBuildsThreeScenePortraitPlan(t *testing.T) {
	plan, err := PlanAd(memory.DefaultBrandProfile(), Request{
		Platform:    "tiktok",
		DurationSec: 15,
		Product:     "Paw Balm",
		Style:       "cute motion graphics",
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Storyboard.Format != "9:16" {
		t.Fatalf("format = %q, want 9:16", plan.Storyboard.Format)
	}
	if len(plan.Storyboard.Scenes) != 3 {
		t.Fatalf("scenes = %d, want 3", len(plan.Storyboard.Scenes))
	}
	if plan.Storyboard.Scenes[0].Start != 0 {
		t.Fatalf("first scene starts at %v", plan.Storyboard.Scenes[0].Start)
	}
	last := plan.Storyboard.Scenes[len(plan.Storyboard.Scenes)-1]
	if last.End != 15 {
		t.Fatalf("last scene ends at %v, want 15", last.End)
	}
	if len(plan.ImagePrompts) != 3 || len(plan.VideoPrompts) != 3 {
		t.Fatalf("prompts image=%d video=%d", len(plan.ImagePrompts), len(plan.VideoPrompts))
	}
}

func TestToEditPlanDefaultsExportSettings(t *testing.T) {
	plan, err := PlanAd(memory.DefaultBrandProfile(), Request{
		Platform:    "facebook",
		DurationSec: 9,
		Product:     "Treat Box",
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	edit := ToEditPlan(plan, map[string]string{})
	if edit.Format != "1:1" || edit.Width != 1080 || edit.Height != 1080 {
		t.Fatalf("edit dimensions = %s %dx%d", edit.Format, edit.Width, edit.Height)
	}
	if edit.Export.Codec != "libx264" {
		t.Fatalf("codec = %q", edit.Export.Codec)
	}
}
