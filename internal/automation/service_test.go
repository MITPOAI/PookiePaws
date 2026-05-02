package automation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/memory"
	"github.com/mitpoai/pookiepaws/internal/providers/mock"
)

func TestServiceRunCreatesReviewQueueAndProjects(t *testing.T) {
	ctx := context.Background()
	store := openAutomationStore(t)
	svc := NewService(store, mock.New())
	result, err := svc.Run(ctx, RunConfig{
		Request: BatchRequest{
			Name:        "launch",
			Product:     "Paw Balm",
			Goal:        "Drive trial purchases",
			Platforms:   []string{"tiktok"},
			DurationSec: 9,
			Variants:    2,
		},
		OutputDir: t.TempDir(),
		Now: func() time.Time {
			return time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(result.Items))
	}
	if _, err := os.Stat(result.ReviewQueueJSON); err != nil {
		t.Fatalf("review queue json missing: %v", err)
	}
	if _, err := os.Stat(result.ReviewQueueMD); err != nil {
		t.Fatalf("review queue markdown missing: %v", err)
	}
	projects, err := store.ListProjects(ctx, 10)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("projects = %d, want 2", len(projects))
	}
	for _, item := range result.Items {
		if item.Status != "ready_for_review" {
			t.Fatalf("item status = %q", item.Status)
		}
		if len(item.AssetPaths) != 3 {
			t.Fatalf("asset paths = %d, want 3", len(item.AssetPaths))
		}
	}
}

func openAutomationStore(t *testing.T) *memory.Store {
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
