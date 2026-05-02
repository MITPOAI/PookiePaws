package mock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/providers"
)

type Provider struct{}

func New() Provider {
	return Provider{}
}

func (Provider) Name() string {
	return "mock"
}

func (p Provider) GenerateImage(ctx context.Context, prompt string, options providers.ImageOptions) (providers.Asset, error) {
	if err := ctx.Err(); err != nil {
		return providers.Asset{}, err
	}
	options = normalizeImageOptions(options)
	if err := os.MkdirAll(options.OutputDir, 0o755); err != nil {
		return providers.Asset{}, err
	}

	id := stableID("mock-image", prompt)
	path := filepath.Join(options.OutputDir, id+"."+options.Format)
	if options.DryRun {
		return providers.Asset{
			ID:        id,
			Type:      providers.AssetImage,
			Path:      path,
			Prompt:    prompt,
			Provider:  p.Name(),
			Width:     options.Width,
			Height:    options.Height,
			CreatedAt: time.Now().UTC(),
			Metadata:  map[string]string{"dry_run": "true"},
		}, nil
	}

	img := image.NewRGBA(image.Rect(0, 0, options.Width, options.Height))
	baseA, baseB := promptColors(prompt)
	for y := 0; y < options.Height; y++ {
		ratio := float64(y) / float64(max(1, options.Height-1))
		c := color.RGBA{
			R: uint8(float64(baseA.R)*(1-ratio) + float64(baseB.R)*ratio),
			G: uint8(float64(baseA.G)*(1-ratio) + float64(baseB.G)*ratio),
			B: uint8(float64(baseA.B)*(1-ratio) + float64(baseB.B)*ratio),
			A: 255,
		}
		draw.Draw(img, image.Rect(0, y, options.Width, y+1), &image.Uniform{C: c}, image.Point{}, draw.Src)
	}
	drawAccentShapes(img, baseA, baseB)

	f, err := os.Create(path)
	if err != nil {
		return providers.Asset{}, err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return providers.Asset{}, err
	}
	if err := f.Close(); err != nil {
		return providers.Asset{}, err
	}

	metaPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".json"
	metadata := map[string]any{
		"provider": p.Name(),
		"prompt":   prompt,
		"width":    options.Width,
		"height":   options.Height,
		"created":  time.Now().UTC().Format(time.RFC3339),
	}
	if b, err := json.MarshalIndent(metadata, "", "  "); err == nil {
		_ = os.WriteFile(metaPath, b, 0o644)
	}

	return providers.Asset{
		ID:        id,
		Type:      providers.AssetImage,
		Path:      path,
		Prompt:    prompt,
		Provider:  p.Name(),
		Width:     options.Width,
		Height:    options.Height,
		CreatedAt: time.Now().UTC(),
		Metadata: map[string]string{
			"metadata_path": metaPath,
		},
	}, nil
}

func (p Provider) GenerateVideo(ctx context.Context, prompt string, imageInput string, options providers.VideoOptions) (providers.Asset, error) {
	if err := ctx.Err(); err != nil {
		return providers.Asset{}, err
	}
	if options.OutputDir == "" {
		options.OutputDir = "."
	}
	if options.Width <= 0 {
		options.Width = 1080
	}
	if options.Height <= 0 {
		options.Height = 1920
	}
	if options.DurationSec <= 0 {
		options.DurationSec = 4
	}
	if err := os.MkdirAll(options.OutputDir, 0o755); err != nil {
		return providers.Asset{}, err
	}
	id := stableID("mock-video", prompt+imageInput)
	path := filepath.Join(options.OutputDir, id+".json")
	payload := map[string]any{
		"provider":     p.Name(),
		"type":         "mock-video-placeholder",
		"prompt":       prompt,
		"image_input":  imageInput,
		"duration_sec": options.DurationSec,
		"width":        options.Width,
		"height":       options.Height,
	}
	if !options.DryRun {
		b, _ := json.MarshalIndent(payload, "", "  ")
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return providers.Asset{}, err
		}
	}
	return providers.Asset{
		ID:        id,
		Type:      providers.AssetOther,
		Path:      path,
		Prompt:    prompt,
		Provider:  p.Name(),
		Width:     options.Width,
		Height:    options.Height,
		Duration:  options.DurationSec,
		CreatedAt: time.Now().UTC(),
		Metadata: map[string]string{
			"note": "mock video generation writes a JSON placeholder; render uses still assets in the MVP",
		},
	}, nil
}

func (p Provider) UpscaleImage(ctx context.Context, asset providers.Asset) (providers.Asset, error) {
	return p.noop(ctx, asset, "upscale_image")
}

func (p Provider) UpscaleVideo(ctx context.Context, asset providers.Asset) (providers.Asset, error) {
	return p.noop(ctx, asset, "upscale_video")
}

func (p Provider) RemoveBackground(ctx context.Context, asset providers.Asset) (providers.Asset, error) {
	return p.noop(ctx, asset, "remove_background")
}

func (p Provider) CaptionAsset(ctx context.Context, asset providers.Asset) (providers.Asset, error) {
	return p.noop(ctx, asset, "caption_asset")
}

func (p Provider) GetTaskStatus(ctx context.Context, taskID string) (providers.TaskStatus, error) {
	if err := ctx.Err(); err != nil {
		return providers.TaskStatus{}, err
	}
	return providers.TaskStatus{
		TaskID:    taskID,
		Provider:  p.Name(),
		Status:    "complete",
		Message:   "mock provider tasks complete immediately",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func (p Provider) DownloadResult(ctx context.Context, taskID string, outputDir string) (providers.Asset, error) {
	return p.GenerateImage(ctx, "download result for "+taskID, providers.ImageOptions{OutputDir: outputDir})
}

func (p Provider) noop(ctx context.Context, asset providers.Asset, operation string) (providers.Asset, error) {
	if err := ctx.Err(); err != nil {
		return providers.Asset{}, err
	}
	if asset.Metadata == nil {
		asset.Metadata = map[string]string{}
	}
	asset.Metadata[operation] = "noop"
	asset.Provider = p.Name()
	return asset, nil
}

func normalizeImageOptions(options providers.ImageOptions) providers.ImageOptions {
	if options.OutputDir == "" {
		options.OutputDir = "."
	}
	if options.Width <= 0 {
		options.Width = 1080
	}
	if options.Height <= 0 {
		options.Height = 1920
	}
	if options.Format == "" {
		options.Format = "png"
	}
	return options
}

func stableID(prefix, value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(sum[:])[:12])
}

func promptColors(prompt string) (color.RGBA, color.RGBA) {
	sum := sha256.Sum256([]byte(prompt))
	a := color.RGBA{R: 120 + sum[0]%100, G: 55 + sum[1]%120, B: 120 + sum[2]%100, A: 255}
	b := color.RGBA{R: 20 + sum[3]%90, G: 130 + sum[4]%100, B: 120 + sum[5]%120, A: 255}
	return a, b
}

func drawAccentShapes(img *image.RGBA, a, b color.RGBA) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	overlay := color.RGBA{R: 255, G: 255, B: 255, A: 48}
	draw.Draw(img, image.Rect(w/12, h/10, w*11/12, h/10+h/18), &image.Uniform{C: overlay}, image.Point{}, draw.Over)
	draw.Draw(img, image.Rect(w/12, h*8/10, w*11/12, h*8/10+h/18), &image.Uniform{C: overlay}, image.Point{}, draw.Over)
	draw.Draw(img, image.Rect(w/3, h/3, w*2/3, h*2/3), &image.Uniform{C: color.RGBA{R: a.R, G: b.G, B: a.B, A: 72}}, image.Point{}, draw.Over)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
