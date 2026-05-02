package mock

import (
	"context"
	"os"
	"testing"

	"github.com/mitpoai/pookiepaws/internal/providers"
)

func TestGenerateImageWritesPNG(t *testing.T) {
	outDir := t.TempDir()
	asset, err := New().GenerateImage(context.Background(), "cute product shot", providers.ImageOptions{
		OutputDir: outDir,
		Width:     320,
		Height:    568,
	})
	if err != nil {
		t.Fatalf("generate image: %v", err)
	}
	if asset.Type != providers.AssetImage {
		t.Fatalf("asset type = %s", asset.Type)
	}
	if _, err := os.Stat(asset.Path); err != nil {
		t.Fatalf("asset path was not written: %v", err)
	}
}

func TestGenerateImageDryRunDoesNotWritePNG(t *testing.T) {
	outDir := t.TempDir()
	asset, err := New().GenerateImage(context.Background(), "dry run", providers.ImageOptions{
		OutputDir: outDir,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("generate image dry-run: %v", err)
	}
	if _, err := os.Stat(asset.Path); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote asset or returned unexpected error: %v", err)
	}
}
