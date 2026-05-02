package studio

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/memory"
	"github.com/mitpoai/pookiepaws/internal/providers/mock"
)

func TestStudioRunCreatesCampaignWorkspace(t *testing.T) {
	ctx := context.Background()
	store := openStudioStore(t)
	svc := NewService(store, mock.New())
	result, err := svc.Run(ctx, RunConfig{
		Request: CampaignRequest{
			Name:            "launch",
			BrandName:       "PookiePaws",
			Product:         "Paw Balm",
			Niche:           "pet care",
			Goal:            "Launch social campaign",
			Offer:           "Shop now",
			TargetAudience:  "pet owners",
			Platforms:       []string{"tiktok", "instagram"},
			ContentVariants: 3,
			DurationSec:     9,
		},
		OutputDir: t.TempDir(),
		Now: func() time.Time {
			return time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("run studio: %v", err)
	}
	for _, path := range []string{
		result.ResearchBriefPath,
		result.StrategyPath,
		result.AdExportPath,
		result.LaunchChecklistPath,
		result.BrowserWorkflowPath,
		result.Content.ReviewQueueMD,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
	}
	if len(result.Content.Items) != 3 {
		t.Fatalf("content items = %d, want 3", len(result.Content.Items))
	}
	if result.PostingStatus != "not_posted_manual_approval_required" {
		t.Fatalf("posting status = %q", result.PostingStatus)
	}
}

func TestCampaignRequestRequiresProduct(t *testing.T) {
	req := CampaignRequest{Name: "missing-product"}
	if err := req.Normalize(); err == nil {
		t.Fatal("expected missing product error")
	}
}

func openStudioStore(t *testing.T) *memory.Store {
	t.Helper()
	store, err := memory.Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("open memory: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize memory: %v", err)
	}
	return store
}
