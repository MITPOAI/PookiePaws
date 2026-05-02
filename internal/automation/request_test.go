package automation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRequestExpandsDefaultItems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "batch.yaml")
	raw := []byte(`
name: launch-week
product: Paw Balm
goal: Drive trials
platforms: [tiktok, instagram]
duration_sec: 12
variants: 3
`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	req, err := LoadRequest(path)
	if err != nil {
		t.Fatalf("load request: %v", err)
	}
	if len(req.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(req.Items))
	}
	if req.Items[1].Platform != "instagram" {
		t.Fatalf("second platform = %q", req.Items[1].Platform)
	}
}

func TestNormalizeRejectsMissingProduct(t *testing.T) {
	req := BatchRequest{Variants: 1}
	if err := req.Normalize(); err == nil {
		t.Fatal("expected missing product error")
	}
}
