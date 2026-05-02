package renderer

type EditPlan struct {
	Format   string         `json:"format"`
	Width    int            `json:"width"`
	Height   int            `json:"height"`
	FPS      int            `json:"fps"`
	Duration float64        `json:"duration"`
	Scenes   []Scene        `json:"scenes"`
	Audio    AudioPlan      `json:"audio,omitempty"`
	Export   ExportSettings `json:"export,omitempty"`
}

type Scene struct {
	ID              string  `json:"id"`
	Start           float64 `json:"start"`
	End             float64 `json:"end"`
	Background      string  `json:"background,omitempty"`
	BackgroundColor string  `json:"background_color,omitempty"`
	Text            string  `json:"text"`
	Subtext         string  `json:"subtext,omitempty"`
	Animation       string  `json:"animation,omitempty"`
	CTA             string  `json:"cta,omitempty"`
	Logo            string  `json:"logo,omitempty"`
	Voiceover       string  `json:"voiceover,omitempty"`
}

type AudioPlan struct {
	MusicPlaceholder       string `json:"music_placeholder,omitempty"`
	SoundEffectPlaceholder string `json:"sound_effect_placeholder,omitempty"`
}

type ExportSettings struct {
	Codec       string `json:"codec,omitempty"`
	PixelFormat string `json:"pixel_format,omitempty"`
	CRF         int    `json:"crf,omitempty"`
}

func DimensionsForFormat(format string) (int, int) {
	switch format {
	case "16:9":
		return 1920, 1080
	case "1:1":
		return 1080, 1080
	default:
		return 1080, 1920
	}
}
