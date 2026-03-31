package skills

import "time"

const (
	MethodMarketTrendsRefresh        = "marketing.trends.refresh"
	MethodSEOAuditRun                = "marketing.seo.audit"
	MethodCompetitorPricingExtract   = "marketing.pricing.extract"
	MethodCrossChannelCampaignDraft  = "marketing.campaign.draft"
	NotificationMarketingSkillStatus = "marketing.skill.progress"
)

type JSONRPCRequest[T any] struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  T      `json:"params"`
}

type JSONRPCNotification[T any] struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  T      `json:"params"`
}

type JSONRPCResponse[T any] struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Result  T      `json:"result"`
}

type EvidenceSource struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Publisher   string    `json:"publisher,omitempty"`
	Snippet     string    `json:"snippet,omitempty"`
	CollectedAt time.Time `json:"collected_at"`
}

type ProgressParams struct {
	WorkflowID string `json:"workflow_id,omitempty"`
	Skill      string `json:"skill"`
	Stage      string `json:"stage"`
	Percent    int    `json:"percent"`
	Message    string `json:"message"`
}

type MarketTrendsParams struct {
	WorkspaceID      string   `json:"workspace_id,omitempty"`
	Brand            string   `json:"brand"`
	Regions          []string `json:"regions,omitempty"`
	Channels         []string `json:"channels,omitempty"`
	Topics           []string `json:"topics,omitempty"`
	Competitors      []string `json:"competitors,omitempty"`
	LookbackDays     int      `json:"lookback_days"`
	MaxSources       int      `json:"max_sources"`
	IncludeSentiment bool     `json:"include_sentiment"`
}

type TrendSignal struct {
	Topic             string   `json:"topic"`
	Direction         string   `json:"direction"`
	Confidence        string   `json:"confidence"`
	Summary           string   `json:"summary"`
	Keywords          []string `json:"keywords,omitempty"`
	RecommendedAction string   `json:"recommended_action,omitempty"`
}

type MarketTrendsResult struct {
	Summary            string           `json:"summary"`
	Signals            []TrendSignal    `json:"signals"`
	RecommendedActions []string         `json:"recommended_actions,omitempty"`
	Sources            []EvidenceSource `json:"sources,omitempty"`
	RefreshedAt        time.Time        `json:"refreshed_at"`
}

type SEOAuditParams struct {
	URL              string   `json:"url"`
	SitemapURL       string   `json:"sitemap_url,omitempty"`
	FocusKeywords    []string `json:"focus_keywords,omitempty"`
	Locale           string   `json:"locale,omitempty"`
	Device           string   `json:"device,omitempty"`
	CrawlLimit       int      `json:"crawl_limit"`
	CheckCompetitors []string `json:"check_competitors,omitempty"`
}

type SEOAuditFinding struct {
	Severity       string   `json:"severity"`
	Category       string   `json:"category"`
	Page           string   `json:"page,omitempty"`
	Issue          string   `json:"issue"`
	Recommendation string   `json:"recommendation"`
	Evidence       []string `json:"evidence,omitempty"`
}

type SEOAuditResult struct {
	CanonicalURL    string            `json:"canonical_url,omitempty"`
	Score           int               `json:"score"`
	IndexedPages    int               `json:"indexed_pages"`
	Findings        []SEOAuditFinding `json:"findings"`
	Recommendations []string          `json:"recommendations,omitempty"`
	Sources         []EvidenceSource  `json:"sources,omitempty"`
}

type CompetitorPricingParams struct {
	Competitor        string   `json:"competitor"`
	Domains           []string `json:"domains"`
	Products          []string `json:"products,omitempty"`
	Regions           []string `json:"regions,omitempty"`
	Currency          string   `json:"currency,omitempty"`
	MaxPages          int      `json:"max_pages"`
	CapturePromotions bool     `json:"capture_promotions"`
}

type PricingObservation struct {
	Product    string    `json:"product"`
	Currency   string    `json:"currency"`
	Price      float64   `json:"price"`
	Region     string    `json:"region,omitempty"`
	Promotion  string    `json:"promotion,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
	SourceURL  string    `json:"source_url"`
}

type CompetitorPricingResult struct {
	Competitor   string               `json:"competitor"`
	Observations []PricingObservation `json:"observations"`
	Summary      string               `json:"summary"`
	Sources      []EvidenceSource     `json:"sources,omitempty"`
}

type AudienceSegment struct {
	Name      string `json:"name"`
	NeedState string `json:"need_state,omitempty"`
	Offer     string `json:"offer,omitempty"`
}

type CampaignConstraints struct {
	CharacterLimit int      `json:"character_limit,omitempty"`
	BlockedClaims  []string `json:"blocked_claims,omitempty"`
	RequiredCTA    string   `json:"required_cta,omitempty"`
}

type CrossChannelCampaignDraftParams struct {
	Brief               string              `json:"brief"`
	Objective           string              `json:"objective"`
	AudienceSegments    []AudienceSegment   `json:"audience_segments"`
	Channels            []string            `json:"channels"`
	BrandVoice          string              `json:"brand_voice,omitempty"`
	Tone                string              `json:"tone,omitempty"`
	Offer               string              `json:"offer,omitempty"`
	Locales             []string            `json:"locales,omitempty"`
	Constraints         CampaignConstraints `json:"constraints,omitempty"`
	IncludeApprovalStep bool                `json:"include_approval_step"`
}

type CampaignAsset struct {
	Channel string `json:"channel"`
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body"`
	CTA     string `json:"cta,omitempty"`
}

type CampaignJourneyStep struct {
	Stage     string   `json:"stage"`
	Channel   string   `json:"channel"`
	Goal      string   `json:"goal"`
	DependsOn []string `json:"depends_on,omitempty"`
}

type CrossChannelCampaignDraftResult struct {
	Summary           string                `json:"summary"`
	Assets            []CampaignAsset       `json:"assets"`
	Journey           []CampaignJourneyStep `json:"journey,omitempty"`
	ApprovalChecklist []string              `json:"approval_checklist,omitempty"`
}

func NewMarketTrendsRequest(id string, params MarketTrendsParams) JSONRPCRequest[MarketTrendsParams] {
	return JSONRPCRequest[MarketTrendsParams]{
		JSONRPC: "2.0",
		ID:      id,
		Method:  MethodMarketTrendsRefresh,
		Params:  params,
	}
}

func NewSEOAuditRequest(id string, params SEOAuditParams) JSONRPCRequest[SEOAuditParams] {
	return JSONRPCRequest[SEOAuditParams]{
		JSONRPC: "2.0",
		ID:      id,
		Method:  MethodSEOAuditRun,
		Params:  params,
	}
}

func NewCompetitorPricingRequest(id string, params CompetitorPricingParams) JSONRPCRequest[CompetitorPricingParams] {
	return JSONRPCRequest[CompetitorPricingParams]{
		JSONRPC: "2.0",
		ID:      id,
		Method:  MethodCompetitorPricingExtract,
		Params:  params,
	}
}

func NewCrossChannelCampaignDraftRequest(id string, params CrossChannelCampaignDraftParams) JSONRPCRequest[CrossChannelCampaignDraftParams] {
	return JSONRPCRequest[CrossChannelCampaignDraftParams]{
		JSONRPC: "2.0",
		ID:      id,
		Method:  MethodCrossChannelCampaignDraft,
		Params:  params,
	}
}

func NewMarketingProgressNotification(params ProgressParams) JSONRPCNotification[ProgressParams] {
	return JSONRPCNotification[ProgressParams]{
		JSONRPC: "2.0",
		Method:  NotificationMarketingSkillStatus,
		Params:  params,
	}
}
