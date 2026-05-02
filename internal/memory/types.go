package memory

import "time"

type PlatformPreference struct {
	Style    string   `json:"style,omitempty"`
	CTAStyle string   `json:"cta_style,omitempty"`
	Notes    string   `json:"notes,omitempty"`
	Lessons  []string `json:"lessons,omitempty"`
}

type BrandProfile struct {
	BrandName             string                        `json:"brand_name"`
	Niche                 string                        `json:"niche"`
	Colors                []string                      `json:"colors"`
	Fonts                 []string                      `json:"fonts"`
	Tone                  string                        `json:"tone"`
	TargetAudience        string                        `json:"target_audience"`
	PreferredVideoStyle   string                        `json:"preferred_video_style"`
	PreferredCTAStyle     string                        `json:"preferred_cta_style"`
	BannedWords           []string                      `json:"banned_words"`
	BannedStyles          []string                      `json:"banned_styles"`
	SuccessfulPastPrompts []string                      `json:"successful_past_prompts"`
	FailedPastPrompts     []string                      `json:"failed_past_prompts"`
	PlatformPreferences   map[string]PlatformPreference `json:"platform_preferences"`
	CreatedAt             time.Time                     `json:"created_at"`
	UpdatedAt             time.Time                     `json:"updated_at"`
}

type ProjectHistory struct {
	ID               string    `json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	UserRequest      string    `json:"user_request"`
	Platform         string    `json:"platform"`
	DurationSec      int       `json:"duration_sec"`
	Provider         string    `json:"provider"`
	GeneratedBrief   string    `json:"generated_brief"`
	PromptsUsed      []string  `json:"prompts_used"`
	ModelUsed        string    `json:"model_used"`
	EditPlanPath     string    `json:"edit_plan_path"`
	FinalOutputPath  string    `json:"final_output_path"`
	ReviewReportPath string    `json:"review_report_path"`
	FeedbackScore    *int      `json:"feedback_score,omitempty"`
	UserCorrections  string    `json:"user_corrections,omitempty"`
	LessonsLearned   string    `json:"lessons_learned,omitempty"`
}

type Feedback struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	CreatedAt       time.Time `json:"created_at"`
	Score           int       `json:"score"`
	UserCorrections string    `json:"user_corrections,omitempty"`
	LessonsLearned  string    `json:"lessons_learned,omitempty"`
}

type SearchResult struct {
	Kind      string    `json:"kind"`
	ID        string    `json:"id"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}

func DefaultBrandProfile() BrandProfile {
	now := time.Now().UTC()
	return BrandProfile{
		BrandName:             "PookiePaws",
		Niche:                 "direct-response social media ads",
		Colors:                []string{"#ff4fa3", "#202124", "#ffffff", "#2ad4ff"},
		Fonts:                 []string{"Inter", "Arial", "system-ui"},
		Tone:                  "cute, direct, upbeat, practical",
		TargetAudience:        "mobile-first shoppers and social media viewers",
		PreferredVideoStyle:   "high-energy motion graphics with bold captions and clean product shots",
		PreferredCTAStyle:     "short, explicit, approval-safe call to action",
		BannedWords:           []string{"guaranteed", "miracle", "risk-free"},
		BannedStyles:          []string{"deceptive urgency", "impersonation", "spammy posting"},
		SuccessfulPastPrompts: []string{},
		FailedPastPrompts:     []string{},
		PlatformPreferences: map[string]PlatformPreference{
			"tiktok": {
				Style:    "9:16, fast hook, large captions, visible CTA",
				CTAStyle: "Tap to shop, learn more, or save for later",
			},
			"instagram": {
				Style:    "9:16 or 1:1, polished motion graphics, brand-color accents",
				CTAStyle: "Shop now or send a DM",
			},
			"youtube-shorts": {
				Style:    "9:16, clear first-frame hook, readable captions",
				CTAStyle: "Subscribe, learn more, or visit the link",
			},
			"facebook": {
				Style:    "1:1 or 9:16, benefit-forward, conservative claims",
				CTAStyle: "Learn more or shop now",
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}
