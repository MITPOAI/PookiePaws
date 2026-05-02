package automation

import "time"

type BatchRequest struct {
	Name        string        `json:"name" yaml:"name"`
	Product     string        `json:"product" yaml:"product"`
	Goal        string        `json:"goal" yaml:"goal"`
	Style       string        `json:"style" yaml:"style"`
	Platforms   []string      `json:"platforms" yaml:"platforms"`
	DurationSec int           `json:"duration_sec" yaml:"duration_sec"`
	Variants    int           `json:"variants" yaml:"variants"`
	Provider    string        `json:"provider" yaml:"provider"`
	Items       []ContentItem `json:"items" yaml:"items"`
}

type ContentItem struct {
	ID          string   `json:"id" yaml:"id"`
	Platform    string   `json:"platform" yaml:"platform"`
	Product     string   `json:"product" yaml:"product"`
	Angle       string   `json:"angle" yaml:"angle"`
	Style       string   `json:"style" yaml:"style"`
	DurationSec int      `json:"duration_sec" yaml:"duration_sec"`
	Notes       []string `json:"notes" yaml:"notes"`
}

type RunConfig struct {
	Request          BatchRequest
	Home             string
	OutputDir        string
	ProviderOverride string
	DryRun           bool
	Render           bool
	RenderScriptPath string
	Now              func() time.Time
}

type BatchResult struct {
	BatchID          string       `json:"batch_id"`
	Name             string       `json:"name"`
	OutputDir        string       `json:"output_dir"`
	ReviewQueueJSON  string       `json:"review_queue_json"`
	ReviewQueueMD    string       `json:"review_queue_md"`
	RequiresApproval bool         `json:"requires_manual_approval"`
	Items            []ItemResult `json:"items"`
	CreatedAt        time.Time    `json:"created_at"`
}

type ItemResult struct {
	ID               string   `json:"id"`
	ProjectID        string   `json:"project_id"`
	Platform         string   `json:"platform"`
	Product          string   `json:"product"`
	Angle            string   `json:"angle"`
	Status           string   `json:"status"`
	RenderStatus     string   `json:"render_status"`
	EditPlanPath     string   `json:"edit_plan_path,omitempty"`
	FinalOutputPath  string   `json:"final_output_path,omitempty"`
	ReviewReportPath string   `json:"review_report_path,omitempty"`
	AssetPaths       []string `json:"asset_paths,omitempty"`
	Error            string   `json:"error,omitempty"`
}
