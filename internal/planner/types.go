package planner

import "github.com/mitpoai/pookiepaws/internal/storyboard"

type Request struct {
	Platform    string `json:"platform"`
	DurationSec int    `json:"duration_sec"`
	Product     string `json:"product"`
	Style       string `json:"style"`
	UserRequest string `json:"user_request"`
}

type Strategy struct {
	Hook     string `json:"hook"`
	Problem  string `json:"problem"`
	Solution string `json:"solution"`
	Benefit  string `json:"benefit"`
	Proof    string `json:"proof"`
	CTA      string `json:"cta"`
}

type Plan struct {
	Request      Request                `json:"request"`
	Brief        string                 `json:"brief"`
	Strategy     Strategy               `json:"strategy"`
	Storyboard   storyboard.Storyboard  `json:"storyboard"`
	ImagePrompts []string               `json:"image_prompts"`
	VideoPrompts []string               `json:"video_prompts"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}
