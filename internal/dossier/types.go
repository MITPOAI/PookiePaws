package dossier

import (
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/research"
)

type Watchlist struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Topic          string     `json:"topic,omitempty"`
	Company        string     `json:"company,omitempty"`
	Competitors    []string   `json:"competitors,omitempty"`
	Domains        []string   `json:"domains,omitempty"`
	Pages          []string   `json:"pages,omitempty"`
	Market         string     `json:"market,omitempty"`
	FocusAreas     []string   `json:"focus_areas,omitempty"`
	TrustedDomains []string   `json:"trusted_domains,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	LastDossierID  string     `json:"last_dossier_id,omitempty"`
}

type Dossier struct {
	ID                string                    `json:"id"`
	WatchlistID       string                    `json:"watchlist_id,omitempty"`
	Topic             string                    `json:"topic"`
	Company           string                    `json:"company,omitempty"`
	Summary           string                    `json:"summary"`
	Provider          string                    `json:"provider,omitempty"`
	FallbackReason    string                    `json:"fallback_reason,omitempty"`
	Coverage          research.Coverage         `json:"coverage"`
	Findings          []string                  `json:"findings,omitempty"`
	CompetitorNotes   []research.CompetitorNote `json:"competitor_notes,omitempty"`
	Warnings          []string                  `json:"warnings,omitempty"`
	EvidenceIDs       []string                  `json:"evidence_ids,omitempty"`
	ChangeIDs         []string                  `json:"change_ids,omitempty"`
	RecommendationIDs []string                  `json:"recommendation_ids,omitempty"`
	CreatedAt         time.Time                 `json:"created_at"`
}

type EvidenceRecord struct {
	ID          string    `json:"id"`
	WatchlistID string    `json:"watchlist_id,omitempty"`
	DossierID   string    `json:"dossier_id"`
	Entity      string    `json:"entity,omitempty"`
	EntityType  string    `json:"entity_type,omitempty"`
	SourceURL   string    `json:"source_url"`
	Host        string    `json:"host,omitempty"`
	Title       string    `json:"title,omitempty"`
	PageType    string    `json:"page_type,omitempty"`
	Excerpt     string    `json:"excerpt,omitempty"`
	Claim       string    `json:"claim"`
	Provider    string    `json:"provider,omitempty"`
	Query       string    `json:"query,omitempty"`
	ObservedAt  time.Time `json:"observed_at"`
	Fingerprint string    `json:"fingerprint"`
}

type ChangeRecord struct {
	ID                 string    `json:"id"`
	WatchlistID        string    `json:"watchlist_id,omitempty"`
	DossierID          string    `json:"dossier_id"`
	Entity             string    `json:"entity,omitempty"`
	SourceURL          string    `json:"source_url"`
	Kind               string    `json:"kind"`
	Summary            string    `json:"summary"`
	EvidenceID         string    `json:"evidence_id,omitempty"`
	PreviousEvidenceID string    `json:"previous_evidence_id,omitempty"`
	ObservedAt         time.Time `json:"observed_at"`
}

type RecommendationStatus string

const (
	RecommendationDraft     RecommendationStatus = "draft"
	RecommendationQueued    RecommendationStatus = "queued"
	RecommendationSubmitted RecommendationStatus = "submitted"
	RecommendationDiscarded RecommendationStatus = "discarded"
)

type Recommendation struct {
	ID               string                    `json:"id"`
	WatchlistID      string                    `json:"watchlist_id,omitempty"`
	DossierID        string                    `json:"dossier_id"`
	Topic            string                    `json:"topic"`
	Title            string                    `json:"title"`
	Summary          string                    `json:"summary"`
	Confidence       float64                   `json:"confidence"`
	EvidenceIDs      []string                  `json:"evidence_ids,omitempty"`
	SourceURLs       []string                  `json:"source_urls,omitempty"`
	ApprovalStatus   string                    `json:"approval_status,omitempty"`
	Status           RecommendationStatus      `json:"status"`
	Provider         string                    `json:"provider,omitempty"`
	QueuedWorkflowID string                    `json:"queued_workflow_id,omitempty"`
	ProposedWorkflow engine.WorkflowDefinition `json:"proposed_workflow"`
	CreatedAt        time.Time                 `json:"created_at"`
	UpdatedAt        time.Time                 `json:"updated_at"`
}

type Snapshot struct {
	Watchlists      []Watchlist      `json:"watchlists,omitempty"`
	Dossiers        []Dossier        `json:"dossiers,omitempty"`
	Evidence        []EvidenceRecord `json:"evidence,omitempty"`
	Changes         []ChangeRecord   `json:"changes,omitempty"`
	Recommendations []Recommendation `json:"recommendations,omitempty"`
}

type GenerateRequest struct {
	WatchlistID    string
	Name           string
	Topic          string
	Company        string
	Competitors    []string
	Domains        []string
	Pages          []string
	Market         string
	FocusAreas     []string
	TrustedDomains []string
	Provider       string
	Debug          bool
}

type GeneratedDossier struct {
	Watchlist       Watchlist        `json:"watchlist"`
	Dossier         Dossier          `json:"dossier"`
	Evidence        []EvidenceRecord `json:"evidence"`
	Changes         []ChangeRecord   `json:"changes"`
	Recommendations []Recommendation `json:"recommendations"`
}

type RefreshResult struct {
	Watchlists      []Watchlist      `json:"watchlists"`
	Dossiers        []Dossier        `json:"dossiers"`
	Changes         []ChangeRecord   `json:"changes"`
	Recommendations []Recommendation `json:"recommendations"`
	Warnings        []string         `json:"warnings,omitempty"`
}

type RecommendationUpdate struct {
	Title            string                     `json:"title,omitempty"`
	Summary          string                     `json:"summary,omitempty"`
	Confidence       *float64                   `json:"confidence,omitempty"`
	ApprovalStatus   string                     `json:"approval_status,omitempty"`
	ProposedWorkflow *engine.WorkflowDefinition `json:"proposed_workflow,omitempty"`
}

type DiffView struct {
	WatchlistID string         `json:"watchlist_id,omitempty"`
	DossierID   string         `json:"dossier_id,omitempty"`
	Summary     string         `json:"summary"`
	Changes     []ChangeRecord `json:"changes,omitempty"`
}
