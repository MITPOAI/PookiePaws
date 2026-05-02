package studio

import (
	"time"

	"github.com/mitpoai/pookiepaws/internal/automation"
)

type CampaignRequest struct {
	Name            string   `json:"name" yaml:"name"`
	BrandName       string   `json:"brand_name" yaml:"brand_name"`
	Product         string   `json:"product" yaml:"product"`
	Niche           string   `json:"niche" yaml:"niche"`
	Goal            string   `json:"goal" yaml:"goal"`
	Offer           string   `json:"offer" yaml:"offer"`
	TargetAudience  string   `json:"target_audience" yaml:"target_audience"`
	Competitors     []string `json:"competitors" yaml:"competitors"`
	Platforms       []string `json:"platforms" yaml:"platforms"`
	ContentVariants int      `json:"content_variants" yaml:"content_variants"`
	DurationSec     int      `json:"duration_sec" yaml:"duration_sec"`
	Style           string   `json:"style" yaml:"style"`
	Provider        string   `json:"provider" yaml:"provider"`
}

type RunConfig struct {
	Request          CampaignRequest
	OutputDir        string
	ProviderOverride string
	DryRun           bool
	Render           bool
	RenderScriptPath string
	Now              func() time.Time
}

type CampaignResult struct {
	CampaignID          string                 `json:"campaign_id"`
	Name                string                 `json:"name"`
	OutputDir           string                 `json:"output_dir"`
	ResearchBriefPath   string                 `json:"research_brief_path"`
	StrategyPath        string                 `json:"strategy_path"`
	AdExportPath        string                 `json:"ad_export_path"`
	LaunchChecklistPath string                 `json:"launch_checklist_path"`
	BrowserWorkflowPath string                 `json:"browser_workflow_path"`
	Content             automation.BatchResult `json:"content"`
	PostingStatus       string                 `json:"posting_status"`
	CreatedAt           time.Time              `json:"created_at"`
}

type AdExport struct {
	CampaignID     string         `json:"campaign_id"`
	Name           string         `json:"name"`
	PostingStatus  string         `json:"posting_status"`
	ManualApproval bool           `json:"manual_approval_required"`
	Ads            []AdExportItem `json:"ads"`
}

type AdExportItem struct {
	ProjectID       string `json:"project_id"`
	Platform        string `json:"platform"`
	AdName          string `json:"ad_name"`
	PrimaryText     string `json:"primary_text"`
	Headline        string `json:"headline"`
	Description     string `json:"description"`
	CTA             string `json:"cta"`
	DestinationURL  string `json:"destination_url_placeholder"`
	EditPlanPath    string `json:"edit_plan_path"`
	FinalOutputPath string `json:"final_output_path"`
}
