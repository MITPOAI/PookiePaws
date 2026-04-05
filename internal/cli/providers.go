package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type ConnectivityCheckMode string

const (
	CheckModeListModels ConnectivityCheckMode = "list_models"
	CheckModeChatPing   ConnectivityCheckMode = "chat_ping"
)

type ProviderPreset struct {
	ID             string
	Label          string
	Hint           string
	ProviderKind   string
	BaseURL        string
	RequiresAPIKey bool
	CheckMode      ConnectivityCheckMode
	Models         []ModelPreset
	DiscoverModels bool
}

type ModelPreset struct {
	Label string
	ID    string
	Hint  string
}

func DefaultProviderPresets() []ProviderPreset {
	return []ProviderPreset{
		{
			ID:             "openai",
			Label:          "OpenAI",
			Hint:           "Hosted. Auto-configures the OpenAI chat-completions endpoint.",
			ProviderKind:   "openai-compatible",
			BaseURL:        "https://api.openai.com/v1/chat/completions",
			RequiresAPIKey: true,
			CheckMode:      CheckModeListModels,
			Models: []ModelPreset{
				{Label: "GPT-5.4", ID: "gpt-5.4", Hint: "Balanced frontier default."},
				{Label: "o3", ID: "o3", Hint: "Reasoning-first model."},
			},
		},
		{
			ID:             "anthropic",
			Label:          "Anthropic",
			Hint:           "Hosted via the Anthropic compatibility endpoint.",
			ProviderKind:   "openai-compatible",
			BaseURL:        "https://api.anthropic.com/v1/chat/completions",
			RequiresAPIKey: true,
			CheckMode:      CheckModeChatPing,
			Models: []ModelPreset{
				{Label: "Claude Opus 4.6", ID: "claude-opus-4-6-20260301", Hint: "Highest capability. 1M token context."},
				{Label: "Claude Sonnet 4.6", ID: "claude-sonnet-4-6-20260301", Hint: "Fast + capable. 1M token context."},
			},
		},
		{
			ID:             "google",
			Label:          "Google",
			Hint:           "Hosted via Gemini OpenAI compatibility.",
			ProviderKind:   "openai-compatible",
			BaseURL:        "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
			RequiresAPIKey: true,
			CheckMode:      CheckModeChatPing,
			Models: []ModelPreset{
				{Label: "Gemini 3.1 Pro", ID: "gemini-3.1-pro", Hint: "Best capability. 1M token context."},
				{Label: "Gemini 3.1 Flash", ID: "gemini-3.1-flash", Hint: "Fast streaming default."},
			},
		},
		{
			ID:             "openrouter",
			Label:          "OpenRouter",
			Hint:           "Hosted multi-model access through one OpenAI-compatible endpoint.",
			ProviderKind:   "openai-compatible",
			BaseURL:        "https://openrouter.ai/api/v1/chat/completions",
			RequiresAPIKey: true,
			CheckMode:      CheckModeListModels,
			Models: []ModelPreset{
				{Label: "DeepSeek V3.2", ID: "deepseek/deepseek-v3.2", Hint: "Generalist frontier preset."},
				{Label: "DeepSeek R1", ID: "deepseek/deepseek-r1", Hint: "Reasoning-heavy preset."},
				{Label: "Qwen 3.5 Plus", ID: "qwen/qwen3.5-plus", Hint: "Strong generalist preset."},
				{Label: "Cohere Command R3", ID: "cohere/command-r3", Hint: "Enterprise RAG and grounded generation."},
				{Label: "Meta Llama 4 Instruct", ID: "meta-llama/llama-4-instruct", Hint: "Open-weight instruction-tuned model."},
			},
		},
		{
			ID:             "ollama",
			Label:          "Ollama",
			Hint:           "Local-first. No API key required.",
			ProviderKind:   "openai-compatible",
			BaseURL:        "http://127.0.0.1:11434/v1/chat/completions",
			RequiresAPIKey: false,
			CheckMode:      CheckModeListModels,
			Models: []ModelPreset{
				{Label: "gpt-oss:20b", ID: "gpt-oss:20b", Hint: "Smaller local default."},
				{Label: "gpt-oss:120b", ID: "gpt-oss:120b", Hint: "Highest local preset."},
			},
		},
		{
			ID:             "lmstudio",
			Label:          "LM Studio / Local",
			Hint:           "Generic local OpenAI-compatible server.",
			ProviderKind:   "openai-compatible",
			BaseURL:        "http://127.0.0.1:1234/v1/chat/completions",
			RequiresAPIKey: false,
			CheckMode:      CheckModeListModels,
			DiscoverModels: true,
			Models: []ModelPreset{
				{Label: "Enter model manually", ID: "", Hint: "Use this if the local server is not running yet."},
			},
		},
	}
}

// QuickStartPreset is a flattened provider+model combination for the fast-path
// init wizard. Selecting one auto-configures provider URL, model ID, and auth.
type QuickStartPreset struct {
	Label        string
	Hint         string
	ProviderKind string
	BaseURL      string
	ModelID      string
	RequiresKey  bool
	CheckMode    ConnectivityCheckMode
	IsCustom     bool
}

// QuickStartPresets returns the curated 4+1 model options shown before the
// full provider selection in the init wizard. The last entry is always
// "Custom Model ID" which falls through to the hierarchical flow.
func QuickStartPresets() []QuickStartPreset {
	return []QuickStartPreset{
		{
			Label:        "DeepSeek V3.2 (Speed / Generalist)",
			Hint:         "Via OpenRouter. Fast, balanced frontier model.",
			ProviderKind: "openai-compatible",
			BaseURL:      "https://openrouter.ai/api/v1/chat/completions",
			ModelID:      "deepseek/deepseek-v3.2",
			RequiresKey:  true,
			CheckMode:    CheckModeListModels,
		},
		{
			Label:        "DeepSeek R1 (Complex Reasoning)",
			Hint:         "Via OpenRouter. Deep reasoning for SEO and strategy.",
			ProviderKind: "openai-compatible",
			BaseURL:      "https://openrouter.ai/api/v1/chat/completions",
			ModelID:      "deepseek/deepseek-r1",
			RequiresKey:  true,
			CheckMode:    CheckModeListModels,
		},
		{
			Label:        "Claude Opus 4.6 (Creative / Copywriting)",
			Hint:         "Anthropic direct. 1M context. Best for brand voice.",
			ProviderKind: "openai-compatible",
			BaseURL:      "https://api.anthropic.com/v1/chat/completions",
			ModelID:      "claude-opus-4-6-20260301",
			RequiresKey:  true,
			CheckMode:    CheckModeChatPing,
		},
		{
			Label:        "Meta Llama 4 70B (Open Source / Enterprise)",
			Hint:         "Via OpenRouter. Open-weight instruction model.",
			ProviderKind: "openai-compatible",
			BaseURL:      "https://openrouter.ai/api/v1/chat/completions",
			ModelID:      "meta-llama/llama-4-instruct",
			RequiresKey:  true,
			CheckMode:    CheckModeListModels,
		},
		{
			Label:    "Custom Model ID",
			Hint:     "Opens the full provider and model selection flow.",
			IsCustom: true,
		},
	}
}

func FindProviderPreset(id string) (ProviderPreset, bool) {
	id = strings.TrimSpace(strings.ToLower(id))
	for _, preset := range DefaultProviderPresets() {
		if preset.ID == id {
			return preset, true
		}
	}
	return ProviderPreset{}, false
}

func ModelsEndpoint(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return ""
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}

	cleanPath := strings.TrimRight(parsed.Path, "/")
	switch {
	case cleanPath == "":
		parsed.Path = "/v1/models"
	case strings.HasSuffix(cleanPath, "/chat/completions"):
		parsed.Path = strings.TrimSuffix(cleanPath, "/chat/completions") + "/models"
	case strings.HasSuffix(cleanPath, "/completions"):
		parsed.Path = strings.TrimSuffix(cleanPath, "/completions") + "/models"
	case strings.HasSuffix(cleanPath, "/models"):
		parsed.Path = cleanPath
	default:
		parsed.Path = path.Clean(cleanPath + "/models")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func DiscoverOpenAICompatibleModels(ctx context.Context, baseURL string) ([]ModelPreset, error) {
	modelsURL := ModelsEndpoint(baseURL)
	if modelsURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("model discovery failed with status %d", resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	models := make([]ModelPreset, 0, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		models = append(models, ModelPreset{
			Label: id,
			ID:    id,
			Hint:  "Discovered from the local compatible server.",
		})
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("no models returned")
	}
	return models, nil
}
