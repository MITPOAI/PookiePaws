package memory

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreInitializesAndPersistsProfile(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	profile, err := store.GetBrandProfile(ctx)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	profile.BrandName = "Test Brand"
	profile.Colors = []string{"#111111", "#ffffff"}
	if err := store.SaveBrandProfile(ctx, profile); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	got, err := store.GetBrandProfile(ctx)
	if err != nil {
		t.Fatalf("get saved profile: %v", err)
	}
	if got.BrandName != "Test Brand" {
		t.Fatalf("brand name = %q, want Test Brand", got.BrandName)
	}
	if len(got.Colors) != 2 {
		t.Fatalf("colors = %#v", got.Colors)
	}
}

func TestProjectSearchAndFeedbackLearning(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	project := ProjectHistory{
		ID:              "project-1",
		CreatedAt:       time.Now().UTC(),
		UserRequest:     "Create a cute TikTok ad for Paw Balm",
		Platform:        "tiktok",
		DurationSec:     15,
		Provider:        "mock",
		GeneratedBrief:  "cute motion graphics",
		PromptsUsed:     []string{"pink product hero shot"},
		ModelUsed:       "mock",
		EditPlanPath:    "edit_plan.json",
		FinalOutputPath: "final.mp4",
	}
	if err := store.SaveProject(ctx, project); err != nil {
		t.Fatalf("save project: %v", err)
	}
	results, err := store.Search(ctx, "Paw Balm", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if _, err := store.AddFeedback(ctx, "project-1", 5, "keep captions large", "large captions worked"); err != nil {
		t.Fatalf("feedback: %v", err)
	}
	profile, err := store.GetBrandProfile(ctx)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if len(profile.SuccessfulPastPrompts) != 1 {
		t.Fatalf("successful prompts = %#v", profile.SuccessfulPastPrompts)
	}
	if got := profile.PlatformPreferences["tiktok"].Lessons; len(got) != 1 || got[0] != "large captions worked" {
		t.Fatalf("lessons = %#v", got)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return store
}
