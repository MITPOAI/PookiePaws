package fal

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/providers"
)

type Provider struct {
	apiKey string
}

func NewFromEnv() Provider {
	return Provider{apiKey: strings.TrimSpace(os.Getenv("FAL_KEY"))}
}

func (p Provider) Name() string {
	return "fal"
}

func (p Provider) GenerateImage(ctx context.Context, prompt string, options providers.ImageOptions) (providers.Asset, error) {
	if err := ctx.Err(); err != nil {
		return providers.Asset{}, err
	}
	if p.apiKey == "" {
		return providers.Asset{}, errors.New("FAL_KEY is not set")
	}
	return providers.Asset{}, errors.New("fal.ai generation is scaffolded for the MVP; use --provider mock until endpoint mapping is implemented")
}

func (p Provider) GenerateVideo(ctx context.Context, prompt string, imageInput string, options providers.VideoOptions) (providers.Asset, error) {
	if err := ctx.Err(); err != nil {
		return providers.Asset{}, err
	}
	if p.apiKey == "" {
		return providers.Asset{}, errors.New("FAL_KEY is not set")
	}
	return providers.Asset{}, errors.New("fal.ai video generation is scaffolded for the MVP; use --provider mock until endpoint mapping is implemented")
}

func (p Provider) UpscaleImage(ctx context.Context, asset providers.Asset) (providers.Asset, error) {
	return asset, p.scaffoldError(ctx)
}

func (p Provider) UpscaleVideo(ctx context.Context, asset providers.Asset) (providers.Asset, error) {
	return asset, p.scaffoldError(ctx)
}

func (p Provider) RemoveBackground(ctx context.Context, asset providers.Asset) (providers.Asset, error) {
	return asset, p.scaffoldError(ctx)
}

func (p Provider) CaptionAsset(ctx context.Context, asset providers.Asset) (providers.Asset, error) {
	return asset, p.scaffoldError(ctx)
}

func (p Provider) GetTaskStatus(ctx context.Context, taskID string) (providers.TaskStatus, error) {
	if err := ctx.Err(); err != nil {
		return providers.TaskStatus{}, err
	}
	if p.apiKey == "" {
		return providers.TaskStatus{}, errors.New("FAL_KEY is not set")
	}
	return providers.TaskStatus{}, errors.New("fal.ai task status is scaffolded for the MVP")
}

func (p Provider) DownloadResult(ctx context.Context, taskID string, outputDir string) (providers.Asset, error) {
	if err := ctx.Err(); err != nil {
		return providers.Asset{}, err
	}
	if p.apiKey == "" {
		return providers.Asset{}, errors.New("FAL_KEY is not set")
	}
	return providers.Asset{}, errors.New("fal.ai download is scaffolded for the MVP")
}

func (p Provider) scaffoldError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.apiKey == "" {
		return errors.New("FAL_KEY is not set")
	}
	return errors.New("fal.ai provider operation is scaffolded for the MVP")
}
