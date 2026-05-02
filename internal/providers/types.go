package providers

import (
	"context"
	"time"
)

type AssetType string

const (
	AssetImage AssetType = "image"
	AssetVideo AssetType = "video"
	AssetOther AssetType = "other"
)

type Asset struct {
	ID        string            `json:"id"`
	Type      AssetType         `json:"type"`
	Path      string            `json:"path"`
	URL       string            `json:"url,omitempty"`
	Prompt    string            `json:"prompt,omitempty"`
	Provider  string            `json:"provider"`
	Width     int               `json:"width,omitempty"`
	Height    int               `json:"height,omitempty"`
	Duration  int               `json:"duration_sec,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

type ImageOptions struct {
	OutputDir string `json:"output_dir"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Format    string `json:"format"`
	Seed      int64  `json:"seed,omitempty"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

type VideoOptions struct {
	OutputDir   string `json:"output_dir"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	DurationSec int    `json:"duration_sec"`
	FPS         int    `json:"fps,omitempty"`
	DryRun      bool   `json:"dry_run,omitempty"`
}

type TaskStatus struct {
	TaskID    string    `json:"task_id"`
	Provider  string    `json:"provider"`
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type Provider interface {
	Name() string
	GenerateImage(ctx context.Context, prompt string, options ImageOptions) (Asset, error)
	GenerateVideo(ctx context.Context, prompt string, imageInput string, options VideoOptions) (Asset, error)
	UpscaleImage(ctx context.Context, asset Asset) (Asset, error)
	UpscaleVideo(ctx context.Context, asset Asset) (Asset, error)
	RemoveBackground(ctx context.Context, asset Asset) (Asset, error)
	CaptionAsset(ctx context.Context, asset Asset) (Asset, error)
	GetTaskStatus(ctx context.Context, taskID string) (TaskStatus, error)
	DownloadResult(ctx context.Context, taskID string, outputDir string) (Asset, error)
}
