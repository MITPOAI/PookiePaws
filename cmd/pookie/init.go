package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/cli"
)

// cmdInit runs the interactive first-run setup wizard. It collects API keys
// and provider URLs, explains how they are stored, and writes
// ~/.pookiepaws/.security.json with mode 0600.
func cmdInit(args []string) {
	p := cli.Stdout()
	p.PinkBanner()
	p.Accent("Pookie setup")
	p.Dim("Professional, local-first setup for your brain and marketing channels.")
	p.Dim("Your keys stay on this machine in ~/.pookiepaws/.security.json.")
	p.Dim("Arrow keys now drive brain provider and model setup.")
	p.Dim("Marketing channels now use a checkbox setup flow with a final review screen.")
	p.Blank()

	cmdInitClassic(args, p)
}

type initAskFunc func(label, key, hint string, secret bool)

type brainConnectivityOutcome int

const (
	brainConnectivityApply brainConnectivityOutcome = iota
	brainConnectivityContinueLocal
	brainConnectivityRetry
	brainConnectivityReenter
	brainConnectivitySkip
)

func cmdInitClassic(args []string, p *cli.Printer) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	home := fs.String("home", "", "override runtime home directory")
	_ = fs.Parse(args)

	runtimeRoot, secretsPath, err := resolveSecretsPath(*home)
	if err != nil {
		p.Error("resolve config path: %v", err)
		os.Exit(1)
	}

	existing := map[string]string{}
	if data, readErr := os.ReadFile(secretsPath); readErr == nil {
		data = trimBOM(data)
		_ = json.Unmarshal(data, &existing)
		p.Warning("Existing configuration found at %s", secretsPath)
		p.Dim("Leave any prompt blank to keep the current value.")
		p.Blank()
	}

	next := make(map[string]string, len(existing))
	for k, v := range existing {
		next[k] = v
	}

	reader := bufio.NewReader(os.Stdin)
	wizard := cli.NewWizard(p)

	ask := func(label, key, hint string, secret bool) {
		cur := next[key]
		if secret {
			promptHint := strings.TrimSpace(hint)
			switch {
			case cur != "":
				promptHint = firstNonEmpty(promptHint, "Masked input.")
			case promptHint == "":
				promptHint = "Masked input."
			}

			val, ok := wizard.PromptSecret(label, promptHint, cur != "")
			if !ok {
				return
			}
			val = strings.TrimSpace(val)
			if val == "" {
				return
			}
			next[key] = val
			return
		}

		if cur != "" {
			p.Plain("%s  (current: %s - blank to keep):", label, cur)
		} else {
			p.Plain("%s%s:", label, hint)
		}
		fmt.Fprint(os.Stdout, "  > ")

		var val string
		var inputErr error
		val, inputErr = reader.ReadString('\n')
		if inputErr != nil {
			return
		}
		val = strings.TrimSpace(val)
		if val == "" {
			return
		}
		next[key] = val
	}

	configureBrainProvider(reader, p, wizard, next, ask)
	if !configureIntegrationsAndReview(reader, p, wizard, next, ask, secretsPath) {
		p.Warning("Setup cancelled before writing the configuration file.")
		p.Blank()
		return
	}

	for k, v := range next {
		if strings.TrimSpace(v) == "" {
			delete(next, k)
		}
	}

	spin := p.NewSpinner("Saving configuration...")
	spin.Start()

	if saveErr := saveSecurityConfig(runtimeRoot, secretsPath, next); saveErr != nil {
		spin.Stop(false, "Failed to save configuration")
		p.Error("%v", saveErr)
		os.Exit(1)
	}

	spin.Stop(true, fmt.Sprintf("Configuration saved to %s", secretsPath))

	p.Blank()
	p.Plain("Start your workspace with:")
	p.Blank()
	p.Accent("  pookie start")
	p.Blank()
}

func configureBrainProvider(reader *bufio.Reader, p *cli.Printer, wizard *cli.Wizard, next map[string]string, ask initAskFunc) {
	p.Rule("Brain Provider")
	p.Blank()
	p.Dim("Choose a hosted or local OpenAI-compatible brain.")
	p.Dim("The wizard stores the exact chat-completions URL and model ID.")
	p.Blank()

	if !cli.InteractiveAvailable() {
		p.Dim("Interactive terminal not detected. Falling back to manual prompts.")
		p.Blank()
		configureBrainProviderFallback(next, ask)
		return
	}

	checker := cli.NewConnectivityChecker(nil)

	presets := cli.DefaultProviderPresets()
	quickPresets := cli.QuickStartPresets()
	hasCurrent := strings.TrimSpace(next["llm_provider"]) != "" ||
		strings.TrimSpace(next["llm_base_url"]) != "" ||
		strings.TrimSpace(next["llm_model"]) != ""

	type providerChoice struct {
		kind       string
		preset     cli.ProviderPreset
		quickStart *cli.QuickStartPreset
	}

	choices := make([]providerChoice, 0, len(quickPresets)+len(presets)+3)
	items := make([]cli.MenuItem, 0, len(quickPresets)+len(presets)+3)
	fallback := 0

	if hasCurrent {
		items = append(items, cli.MenuItem{
			Label: "Keep current brain configuration",
			Hint: fmt.Sprintf("Current: %s / %s",
				currentBrainLabel(next),
				firstNonEmpty(next["llm_model"], "model not set")),
		})
		choices = append(choices, providerChoice{kind: "keep"})
	}

	// Recommended quick-start models at the top of the list.
	for i := range quickPresets {
		qs := &quickPresets[i]
		if qs.IsCustom {
			continue // skip the "Custom" entry, the full providers below cover that
		}
		items = append(items, cli.MenuItem{
			Label: qs.Label,
			Hint:  qs.Hint,
		})
		choices = append(choices, providerChoice{kind: "quickstart", quickStart: qs})
	}

	// Full provider list below the recommendations.
	for _, preset := range presets {
		items = append(items, cli.MenuItem{
			Label: preset.Label,
			Hint:  preset.Hint,
		})
		choices = append(choices, providerChoice{kind: "provider", preset: preset})
	}

	items = append(items, cli.MenuItem{
		Label: "Skip brain setup for now",
		Hint:  "Leave the current brain settings untouched.",
	})
	choices = append(choices, providerChoice{kind: "skip"})

providerSelection:
	for {
		choiceIndex, ok := wizard.Select(
			"Choose a brain provider",
			"Arrow keys move. Enter confirms.",
			items,
			fallback,
		)
		if !ok {
			if hasCurrent {
				p.Info("Keeping the current brain configuration.")
			} else {
				p.Warning("Brain setup skipped for now.")
			}
			p.Blank()
			return
		}

		choice := choices[choiceIndex]
		switch choice.kind {
		case "keep":
			p.Success("Keeping %s / %s",
				currentBrainLabel(next),
				firstNonEmpty(next["llm_model"], "current model"))
			p.Blank()
			return
		case "skip":
			p.Warning("Brain setup skipped for now.")
			p.Blank()
			return
		case "quickstart":
			qs := choice.quickStart
			// Build a synthetic provider preset so the connectivity flow works.
			syntheticPreset := cli.ProviderPreset{
				Label:          qs.Label,
				ProviderKind:   qs.ProviderKind,
				BaseURL:        qs.BaseURL,
				RequiresAPIKey: qs.RequiresKey,
				CheckMode:      qs.CheckMode,
			}

			candidateAPIKey := ""
			if qs.RequiresKey {
				sameURL := strings.EqualFold(strings.TrimSpace(next["llm_base_url"]), qs.BaseURL)
				if sameURL {
					candidateAPIKey = strings.TrimSpace(next["llm_api_key"])
				}
				keyHint := fmt.Sprintf("%s requires an API key. Leave blank to keep the current value.", qs.Label)
				keyInput, keyOK := wizard.PromptSecret("LLM API key", keyHint, candidateAPIKey != "")
				if !keyOK {
					p.Blank()
					continue
				}
				if trimmed := strings.TrimSpace(keyInput); trimmed != "" {
					candidateAPIKey = trimmed
				}
			}

			for {
				outcome := runBrainConnectivityCheck(p, wizard, checker, syntheticPreset, qs.ModelID, candidateAPIKey)
				switch outcome {
				case brainConnectivityApply:
					next["llm_provider"] = qs.ProviderKind
					next["llm_base_url"] = qs.BaseURL
					next["llm_model"] = qs.ModelID
					if qs.RequiresKey {
						next["llm_api_key"] = candidateAPIKey
					} else {
						delete(next, "llm_api_key")
					}
					p.Success("Brain configured: %s", qs.Label)
					p.Blank()
					return
				case brainConnectivityRetry:
					continue
				case brainConnectivityReenter:
					keyInput, keyOK := wizard.PromptSecret("LLM API key", "Re-enter your API key.", candidateAPIKey != "")
					if !keyOK {
						break
					}
					if trimmed := strings.TrimSpace(keyInput); trimmed != "" {
						candidateAPIKey = trimmed
					}
					continue
				case brainConnectivityContinueLocal:
					next["llm_provider"] = qs.ProviderKind
					next["llm_base_url"] = qs.BaseURL
					next["llm_model"] = qs.ModelID
					delete(next, "llm_api_key")
					p.Warning("Continuing with %s even though it is not reachable right now.", qs.Label)
					p.Blank()
					return
				case brainConnectivitySkip:
					p.Warning("Brain setup skipped for now.")
					p.Blank()
					return
				}
				break // break inner retry loop if we get an unhandled outcome
			}
			continue providerSelection

		case "provider":
			modelID, modelLabel, ok := selectProviderModel(reader, p, wizard, choice.preset, next["llm_model"])
			if !ok {
				p.Blank()
				continue
			}

			samePreset := strings.EqualFold(strings.TrimSpace(next["llm_base_url"]), choice.preset.BaseURL)
			candidateAPIKey := ""
			if samePreset {
				candidateAPIKey = strings.TrimSpace(next["llm_api_key"])
			}

		promptKey:
			for {
				if choice.preset.RequiresAPIKey {
					keyHint := fmt.Sprintf("%s requires an API key. Leave blank to keep the current value.", choice.preset.Label)
					keyInput, ok := wizard.PromptSecret("LLM API key", keyHint, candidateAPIKey != "")
					if !ok {
						p.Blank()
						continue providerSelection
					}
					if trimmed := strings.TrimSpace(keyInput); trimmed != "" {
						candidateAPIKey = trimmed
					}
				} else {
					candidateAPIKey = ""
				}

			checkConnectivity:
				for {
					outcome := runBrainConnectivityCheck(p, wizard, checker, choice.preset, modelID, candidateAPIKey)
					switch outcome {
					case brainConnectivityApply:
						next["llm_provider"] = choice.preset.ProviderKind
						next["llm_base_url"] = choice.preset.BaseURL
						next["llm_model"] = modelID
						if choice.preset.RequiresAPIKey {
							next["llm_api_key"] = candidateAPIKey
						} else {
							delete(next, "llm_api_key")
						}
						p.Success("Brain configured: %s / %s", choice.preset.Label, modelLabel)
						p.Blank()
						return
					case brainConnectivityContinueLocal:
						next["llm_provider"] = choice.preset.ProviderKind
						next["llm_base_url"] = choice.preset.BaseURL
						next["llm_model"] = modelID
						delete(next, "llm_api_key")
						p.Warning("Continuing with %s even though it is not reachable right now.", choice.preset.Label)
						p.Blank()
						return
					case brainConnectivityRetry:
						continue checkConnectivity
					case brainConnectivityReenter:
						continue promptKey
					case brainConnectivitySkip:
						p.Warning("Brain setup skipped for now.")
						p.Blank()
						return
					}
				}
			}
		}
	}
}

func configureBrainProviderFallback(next map[string]string, ask initAskFunc) {
	previousBaseURL := next["llm_base_url"]
	previousModel := next["llm_model"]
	previousAPIKey := next["llm_api_key"]

	ask("LLM base URL", "llm_base_url",
		"  (e.g. http://127.0.0.1:11434/v1/chat/completions)", false)
	ask("LLM model name", "llm_model",
		"  (e.g. gpt-5.1, claude-opus-4-1-20250805, llama3.2:latest)", false)
	ask("LLM API key", "llm_api_key",
		"  (leave blank for local or unauthenticated models)", true)

	if next["llm_base_url"] != previousBaseURL ||
		next["llm_model"] != previousModel ||
		next["llm_api_key"] != previousAPIKey {
		next["llm_provider"] = "openai-compatible"
	}
}

func runBrainConnectivityCheck(p *cli.Printer, wizard *cli.Wizard, checker *cli.ConnectivityChecker, preset cli.ProviderPreset, modelID, apiKey string) brainConnectivityOutcome {
	spin := p.NewSpinner(fmt.Sprintf("Checking %s connectivity...", preset.Label))
	spin.Start()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	result, err := checker.Check(ctx, preset, modelID, apiKey)
	cancel()

	if err == nil {
		spin.Stop(true, result.Message)
		if strings.TrimSpace(result.Endpoint) != "" {
			p.Dim("Verified endpoint: %s", result.Endpoint)
		}
		return brainConnectivityApply
	}

	spin.Stop(false, "Connectivity check failed")
	p.Warning("%v", err)
	if strings.TrimSpace(result.Endpoint) != "" {
		p.Dim("Endpoint: %s", result.Endpoint)
	}
	p.Blank()

	type option struct {
		item    cli.MenuItem
		outcome brainConnectivityOutcome
	}

	options := []option{
		{
			item: cli.MenuItem{
				Label: "Retry connectivity check",
				Hint:  "Run the same verification again.",
			},
			outcome: brainConnectivityRetry,
		},
	}
	if preset.RequiresAPIKey {
		options = append(options, option{
			item: cli.MenuItem{
				Label: "Re-enter API key",
				Hint:  "Update the API key and try again.",
			},
			outcome: brainConnectivityReenter,
		})
	} else {
		options = append(options, option{
			item: cli.MenuItem{
				Label: "Continue anyway",
				Hint:  "Keep the local provider settings even if the server is offline right now.",
			},
			outcome: brainConnectivityContinueLocal,
		})
	}
	options = append(options, option{
		item: cli.MenuItem{
			Label: "Skip brain setup for now",
			Hint:  "Leave the previous brain configuration untouched.",
		},
		outcome: brainConnectivitySkip,
	})

	items := make([]cli.MenuItem, 0, len(options))
	for _, option := range options {
		items = append(items, option.item)
	}

	index, ok := wizard.Select(
		"Connectivity check failed",
		"Choose how to proceed.",
		items,
		0,
	)
	if !ok {
		return brainConnectivitySkip
	}
	return options[index].outcome
}

func selectProviderModel(reader *bufio.Reader, p *cli.Printer, wizard *cli.Wizard, preset cli.ProviderPreset, currentModel string) (string, string, bool) {
	models := append([]cli.ModelPreset(nil), preset.Models...)

	if preset.DiscoverModels {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		discovered, err := cli.DiscoverOpenAICompatibleModels(ctx, preset.BaseURL)
		cancel()
		if err != nil {
			p.Warning("Could not discover local models at %s", cli.ModelsEndpoint(preset.BaseURL))
			p.Dim("Start the local server first, or enter the model ID manually.")
		} else {
			models = append(discovered, preset.Models...)
			p.Success("Discovered %d local model(s) from %s", len(discovered), preset.Label)
		}
		p.Blank()
	}

	for {
		items := make([]cli.MenuItem, 0, len(models))
		fallback := 0
		for index, model := range models {
			items = append(items, cli.MenuItem{
				Label: model.Label,
				Hint:  model.Hint,
			})
			if strings.EqualFold(strings.TrimSpace(currentModel), model.ID) && model.ID != "" {
				fallback = index
			}
		}

		index, ok := wizard.Select(
			fmt.Sprintf("Choose a model for %s", preset.Label),
			"The exact model ID will be saved to your local config.",
			items,
			fallback,
		)
		if !ok {
			return "", "", false
		}

		selected := models[index]
		if strings.TrimSpace(selected.ID) != "" {
			return selected.ID, selected.Label, true
		}

		modelID, ok := promptManualModel(reader, p, currentModel)
		if ok {
			return modelID, modelID, true
		}

		p.Warning("A model ID is required for manual local configuration.")
		p.Blank()
	}
}

func promptManualModel(reader *bufio.Reader, p *cli.Printer, current string) (string, bool) {
	if strings.TrimSpace(current) != "" {
		p.Plain("Model ID  (current: %s - blank to keep):", current)
	} else {
		p.Plain("Model ID  (e.g. llama3.2:latest):")
	}
	fmt.Fprint(os.Stdout, "  > ")

	value, err := reader.ReadString('\n')
	if err != nil {
		return "", false
	}

	value = strings.TrimSpace(value)
	if value == "" {
		if strings.TrimSpace(current) != "" {
			return current, true
		}
		return "", false
	}
	return value, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func currentBrainLabel(config map[string]string) string {
	baseURL := strings.TrimSpace(config["llm_base_url"])
	for _, preset := range cli.DefaultProviderPresets() {
		if strings.EqualFold(baseURL, preset.BaseURL) {
			return preset.Label
		}
	}
	return firstNonEmpty(config["llm_provider"], "configured")
}

func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}
