package storyboard

type Scene struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Objective    string  `json:"objective"`
	Start        float64 `json:"start"`
	End          float64 `json:"end"`
	VisualPrompt string  `json:"visual_prompt"`
	VideoPrompt  string  `json:"video_prompt"`
	Caption      string  `json:"caption"`
	Subcaption   string  `json:"subcaption,omitempty"`
	CTA          string  `json:"cta,omitempty"`
	Animation    string  `json:"animation"`
}

type Storyboard struct {
	Platform    string  `json:"platform"`
	Format      string  `json:"format"`
	DurationSec int     `json:"duration_sec"`
	Scenes      []Scene `json:"scenes"`
}
