package main

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/adapters"
	"github.com/mitpoai/pookiepaws/internal/cli"
	"github.com/mitpoai/pookiepaws/internal/engine"
)

const (
	defaultSalesmanagoBaseURL = "https://api.salesmanago.com/v3/keyInformation/upsert"
	defaultMittoBaseURL       = "https://rest.mittoapi.net"
	defaultWhatsAppProvider   = "meta_cloud"
	defaultWhatsAppBaseURL    = "https://graph.facebook.com/v23.0"
)

type integrationDefinition struct {
	ID    string
	Label string
	Hint  string
	Keys  []string
}

type integrationReviewOutcome int

const (
	integrationReviewSave integrationReviewOutcome = iota
	integrationReviewAdjust
	integrationReviewCancel
)

type integrationValidationOutcome int

const (
	integrationValidationRetry integrationValidationOutcome = iota
	integrationValidationBack
	integrationValidationSkip
)

var integrationDefinitions = []integrationDefinition{
	{
		ID:    "firecrawl",
		Label: "Firecrawl / Jina (Web Research)",
		Hint:  "Turn any URL into clean markdown for competitor research and content analysis.",
		Keys:  []string{"firecrawl_api_key", "jina_api_key"},
	},
	{
		ID:    "resend",
		Label: "Resend (Email)",
		Hint:  "Send marketing and transactional emails through the Resend API.",
		Keys:  []string{"resend_api_key", "resend_from"},
	},
	{
		ID:    "whatsapp",
		Label: "Meta WhatsApp",
		Hint:  "Send approval-gated WhatsApp messages to customers via Meta Cloud API.",
		Keys: []string{
			"whatsapp_provider",
			"whatsapp_access_token",
			"whatsapp_phone_number_id",
			"whatsapp_business_account_id",
			"whatsapp_webhook_verify_token",
			"whatsapp_base_url",
		},
	},
	{
		ID:    "mitto",
		Label: "Mitto (SMS)",
		Hint:  "Send SMS campaigns and transactional messages through Mitto.",
		Keys:  []string{"mitto_api_key", "mitto_base_url", "mitto_from"},
	},
	{
		ID:    "hubspot",
		Label: "HubSpot (CRM)",
		Hint:  "Create and update contacts in HubSpot CRM from your workflows.",
		Keys:  []string{"hubspot_api_key"},
	},
	{
		ID:    "salesmanago",
		Label: "SALESmanago (CRM)",
		Hint:  "Route leads and manage contacts in SALESmanago CRM.",
		Keys:  []string{"salesmanago_base_url", "salesmanago_api_key", "salesmanago_owner"},
	},
	{
		ID:    "slack_webhook",
		Label: "Slack (Notifications)",
		Hint:  "Get daily workflow summaries posted to a Slack channel.",
		Keys:  []string{"slack_webhook_url"},
	},
	{
		ID:    "discord_webhook",
		Label: "Discord (Notifications)",
		Hint:  "Get daily workflow summaries posted to a Discord channel.",
		Keys:  []string{"discord_webhook_url"},
	},
}

type initSecrets map[string]string

func (s initSecrets) Get(name string) (string, error) {
	value, ok := s[name]
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("missing secret %s", name)
	}
	return value, nil
}

func (s initSecrets) RedactMap(payload map[string]any) map[string]any {
	return payload
}

func configureIntegrationsAndReview(reader *bufio.Reader, p *cli.Printer, wizard *cli.Wizard, next map[string]string, ask initAskFunc, secretsPath string) bool {
	selected := currentIntegrationSelection(next)

	// Ask whether to configure channels at all - most users just need the brain.
	hasAnyChannel := false
	for _, v := range selected {
		if v {
			hasAnyChannel = true
			break
		}
	}

	skipIndex := 0
	configIndex := 1
	channelChoice, ok := wizard.Select(
		"Marketing channels",
		"Channels let Pookie send emails, SMS, and research the web for you.",
		[]cli.MenuItem{
			{Label: "Skip for now - I just need the AI brain", Hint: "You can add channels later by running pookie init again."},
			{Label: "Set up marketing channels", Hint: "Choose which services to connect (email, SMS, CRM, research, notifications)."},
		},
		skipIndex,
	)
	if !ok || channelChoice == skipIndex {
		if hasAnyChannel {
			p.Info("Keeping previously configured channels.")
		} else {
			p.Dim("Channels skipped. Run pookie init again anytime to add them.")
		}
		p.Blank()
		// Still go to review so the user can save the brain config.
		switch reviewConfiguration(p, wizard, next, selected, secretsPath) {
		case integrationReviewSave:
			return true
		default:
			return false
		}
	}
	_ = configIndex

	for {
		updatedSelection, ok := chooseIntegrations(wizard, selected)
		if !ok {
			return false
		}
		selected = updatedSelection

		clearDeselectedIntegrationKeys(next, selected)

		ok = configureSelectedIntegrations(reader, p, wizard, next, ask, selected)
		if !ok {
			continue
		}

		switch reviewConfiguration(p, wizard, next, selected, secretsPath) {
		case integrationReviewSave:
			return true
		case integrationReviewAdjust:
			continue
		case integrationReviewCancel:
			return false
		}
	}
}

func chooseIntegrations(wizard *cli.Wizard, current map[string]bool) (map[string]bool, bool) {
	items := make([]cli.CheckboxItem, 0, len(integrationDefinitions))
	for _, definition := range integrationDefinitions {
		items = append(items, cli.CheckboxItem{
			Label:   definition.Label,
			Hint:    definition.Hint,
			Checked: current[definition.ID],
		})
	}

	selectedItems, ok := wizard.MultiSelect(
		"Choose marketing channels",
		"Space toggles a channel. Enter confirms the active set.",
		items,
	)
	if !ok {
		return nil, false
	}

	selected := make(map[string]bool, len(selectedItems))
	for index, item := range selectedItems {
		selected[integrationDefinitions[index].ID] = item.Checked
	}
	return selected, true
}

func configureSelectedIntegrations(reader *bufio.Reader, p *cli.Printer, wizard *cli.Wizard, next map[string]string, ask initAskFunc, selected map[string]bool) bool {
	if selected["firecrawl"] {
		if ok := configureFirecrawlIntegration(p, next, ask); !ok {
			return false
		}
	}
	if selected["resend"] {
		if ok := configureResendIntegration(p, next, ask); !ok {
			return false
		}
	}
	if selected["whatsapp"] {
		if ok := configureWhatsAppIntegration(reader, p, wizard, next, ask); !ok {
			return false
		}
	}
	if selected["mitto"] {
		if ok := configureMittoIntegration(p, wizard, next, ask); !ok {
			return false
		}
	}
	if selected["hubspot"] {
		if ok := configureHubSpotIntegration(p, next, ask); !ok {
			return false
		}
	}
	if selected["salesmanago"] {
		if ok := configureSalesmanagoIntegration(p, wizard, next, ask); !ok {
			return false
		}
	}
	if selected["slack_webhook"] {
		if ok := configureWebhookIntegration(p, next, ask, "Slack", "slack_webhook_url"); !ok {
			return false
		}
	}
	if selected["discord_webhook"] {
		if ok := configureWebhookIntegration(p, next, ask, "Discord", "discord_webhook_url"); !ok {
			return false
		}
	}
	return true
}

func configureWhatsAppIntegration(_ *bufio.Reader, p *cli.Printer, wizard *cli.Wizard, next map[string]string, ask initAskFunc) bool {
	for {
		p.Blank()
		p.Rule("WhatsApp Integration")
		p.Blank()
		p.Dim("WhatsApp runs as an approval-gated outbound channel.")
		p.Dim("The default provider mode expects Meta Cloud API-compatible endpoints.")
		p.Blank()

		ask("WhatsApp provider", "whatsapp_provider", "  (default: meta_cloud)", false)
		ask("WhatsApp access token", "whatsapp_access_token", "Masked input.", true)
		ask("WhatsApp phone number ID", "whatsapp_phone_number_id", "", false)
		ask("WhatsApp business account ID", "whatsapp_business_account_id", "", false)
		ask("WhatsApp webhook verify token", "whatsapp_webhook_verify_token", "Masked input.", true)
		ask("WhatsApp base URL", "whatsapp_base_url", "  (default: https://graph.facebook.com/v23.0)", false)

		next["whatsapp_provider"] = firstNonEmpty(next["whatsapp_provider"], defaultWhatsAppProvider)

		spin := p.NewSpinner("Checking WhatsApp connectivity...")
		spin.Start()
		status, err := testWhatsAppIntegration(next)
		if err == nil {
			spin.Stop(true, status.Message)
			p.Blank()
			return true
		}

		spin.Stop(false, "WhatsApp validation failed")
		p.Warning("%v", err)
		p.Blank()

		outcome := selectValidationOutcome(
			wizard,
			"WhatsApp setup needs attention",
			"Retry the connectivity test, re-enter settings, or skip this channel.",
			[]validationOption{
				{
					Label:   "Retry WhatsApp test",
					Hint:    "Run the same credential check again.",
					Outcome: integrationValidationRetry,
				},
				{
					Label:   "Re-enter WhatsApp settings",
					Hint:    "Edit the token, phone number ID, or base URL.",
					Outcome: integrationValidationBack,
				},
				{
					Label:   "Skip WhatsApp for now",
					Hint:    "Clear WhatsApp activation and continue setup.",
					Outcome: integrationValidationSkip,
				},
			},
		)

		switch outcome {
		case integrationValidationRetry:
			continue
		case integrationValidationBack:
			continue
		case integrationValidationSkip:
			clearIntegrationKeys(next, "whatsapp")
			return true
		}
	}
}

func configureMittoIntegration(p *cli.Printer, wizard *cli.Wizard, next map[string]string, ask initAskFunc) bool {
	for {
		p.Blank()
		p.Rule("Mitto SMS")
		p.Blank()
		p.Dim("Mitto powers outbound campaign delivery.")
		p.Blank()

		ask("Mitto API key", "mitto_api_key", "Masked input.", true)
		ask("Mitto base URL", "mitto_base_url", "  (default: https://rest.mittoapi.net)", false)
		ask("Mitto sender ID", "mitto_from", "  (the From name shown on messages)", false)

		if err := validateMittoIntegration(next); err == nil {
			p.Success("Mitto settings look valid.")
			p.Blank()
			return true
		} else {
			p.Warning("%v", err)
			p.Blank()
		}

		outcome := selectValidationOutcome(
			wizard,
			"Mitto setup needs attention",
			"Re-enter the settings or skip this channel for now.",
			[]validationOption{
				{
					Label:   "Re-enter Mitto settings",
					Hint:    "Update the API key, sender ID, or base URL.",
					Outcome: integrationValidationBack,
				},
				{
					Label:   "Skip Mitto for now",
					Hint:    "Clear Mitto activation and continue setup.",
					Outcome: integrationValidationSkip,
				},
			},
		)
		if outcome == integrationValidationSkip {
			clearIntegrationKeys(next, "mitto")
			return true
		}
	}
}

func configureSalesmanagoIntegration(p *cli.Printer, wizard *cli.Wizard, next map[string]string, ask initAskFunc) bool {
	for {
		p.Blank()
		p.Rule("SALESmanago")
		p.Blank()
		p.Dim("SALESmanago credentials power lead routing and CRM sync.")
		p.Blank()

		ask("SALESmanago base URL", "salesmanago_base_url", "  (e.g. https://api.salesmanago.com/v3/keyInformation/upsert)", false)
		ask("SALESmanago API key", "salesmanago_api_key", "Masked input.", true)
		ask("SALESmanago owner email", "salesmanago_owner", "", false)

		if err := validateSalesmanagoIntegration(next); err == nil {
			p.Success("SALESmanago settings look valid.")
			p.Blank()
			return true
		} else {
			p.Warning("%v", err)
			p.Blank()
		}

		outcome := selectValidationOutcome(
			wizard,
			"SALESmanago setup needs attention",
			"Re-enter the settings or skip this channel for now.",
			[]validationOption{
				{
					Label:   "Re-enter SALESmanago settings",
					Hint:    "Update the API key, owner email, or base URL.",
					Outcome: integrationValidationBack,
				},
				{
					Label:   "Skip SALESmanago for now",
					Hint:    "Clear SALESmanago activation and continue setup.",
					Outcome: integrationValidationSkip,
				},
			},
		)
		if outcome == integrationValidationSkip {
			clearIntegrationKeys(next, "salesmanago")
			return true
		}
	}
}

func testWhatsAppIntegration(config map[string]string) (engine.ChannelProviderStatus, error) {
	if strings.TrimSpace(config["whatsapp_access_token"]) == "" {
		return engine.ChannelProviderStatus{}, fmt.Errorf("whatsapp_access_token is required")
	}
	if strings.TrimSpace(config["whatsapp_phone_number_id"]) == "" {
		return engine.ChannelProviderStatus{}, fmt.Errorf("whatsapp_phone_number_id is required")
	}
	if err := validateHTTPURL(firstNonEmpty(config["whatsapp_base_url"], defaultWhatsAppBaseURL)); err != nil {
		return engine.ChannelProviderStatus{}, fmt.Errorf("whatsapp_base_url: %w", err)
	}

	provider := firstNonEmpty(config["whatsapp_provider"], defaultWhatsAppProvider)
	if provider != "meta_cloud" && provider != "meta" {
		return engine.ChannelProviderStatus{}, fmt.Errorf("unsupported WhatsApp provider %q", provider)
	}

	secrets := initSecrets{
		"whatsapp_provider":             provider,
		"whatsapp_access_token":         config["whatsapp_access_token"],
		"whatsapp_phone_number_id":      config["whatsapp_phone_number_id"],
		"whatsapp_business_account_id":  config["whatsapp_business_account_id"],
		"whatsapp_webhook_verify_token": config["whatsapp_webhook_verify_token"],
		"whatsapp_base_url":             firstNonEmpty(config["whatsapp_base_url"], defaultWhatsAppBaseURL),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return adapters.NewWhatsAppAdapter().Test(ctx, secrets)
}

func configureFirecrawlIntegration(p *cli.Printer, next map[string]string, ask initAskFunc) bool {
	p.Blank()
	p.Rule("Firecrawl / Jina (Web Research)")
	p.Blank()
	p.Dim("Firecrawl converts web pages to clean markdown for competitor research.")
	p.Dim("If you don't have a Firecrawl key, Jina Reader is used as a free fallback.")
	p.Dim("You can leave both blank to use Jina without authentication.")
	p.Blank()

	ask("Firecrawl API key (optional)", "firecrawl_api_key", "Masked input.", true)
	ask("Jina API key (optional fallback)", "jina_api_key", "Masked input.", true)

	p.Success("Research channel configured.")
	p.Blank()
	return true
}

func configureResendIntegration(p *cli.Printer, next map[string]string, ask initAskFunc) bool {
	p.Blank()
	p.Rule("Resend (Email)")
	p.Blank()
	p.Dim("Resend powers outbound email delivery for campaigns and notifications.")
	p.Blank()

	ask("Resend API key", "resend_api_key", "Masked input.", true)
	ask("Default sender email", "resend_from", "  (e.g. marketing@yourdomain.com)", false)

	if strings.TrimSpace(next["resend_api_key"]) == "" {
		p.Warning("Resend API key is required. Skipping email channel.")
		clearIntegrationKeys(next, "resend")
	} else {
		p.Success("Resend email configured.")
	}
	p.Blank()
	return true
}

func configureHubSpotIntegration(p *cli.Printer, next map[string]string, ask initAskFunc) bool {
	p.Blank()
	p.Rule("HubSpot (CRM)")
	p.Blank()
	p.Dim("HubSpot integration creates and updates contacts from your workflows.")
	p.Blank()

	ask("HubSpot API key", "hubspot_api_key", "Masked input.", true)

	if strings.TrimSpace(next["hubspot_api_key"]) == "" {
		p.Warning("HubSpot API key is required. Skipping CRM channel.")
		clearIntegrationKeys(next, "hubspot")
	} else {
		p.Success("HubSpot CRM configured.")
	}
	p.Blank()
	return true
}

func configureWebhookIntegration(p *cli.Printer, next map[string]string, ask initAskFunc, name string, key string) bool {
	p.Blank()
	p.Rule(name + " Notifications")
	p.Blank()
	p.Dim("Paste your " + name + " incoming webhook URL to receive daily workflow summaries.")
	p.Blank()

	ask(name+" webhook URL", key, "  (https://hooks.slack.com/... or https://discord.com/api/webhooks/...)", false)

	webhookURL := strings.TrimSpace(next[key])
	if webhookURL == "" {
		p.Warning(name + " webhook URL not set. Skipping.")
		clearIntegrationKeys(next, strings.ToLower(name)+"_webhook")
	} else if err := validateHTTPURL(webhookURL); err != nil {
		p.Warning("Invalid URL: %v. Skipping %s.", err, name)
		clearIntegrationKeys(next, strings.ToLower(name)+"_webhook")
	} else {
		p.Success(name + " notifications configured.")
	}
	p.Blank()
	return true
}

func validateMittoIntegration(config map[string]string) error {
	if strings.TrimSpace(config["mitto_api_key"]) == "" {
		return fmt.Errorf("mitto_api_key is required")
	}
	if strings.TrimSpace(config["mitto_from"]) == "" {
		return fmt.Errorf("mitto_from is required")
	}
	if err := validateHTTPURL(firstNonEmpty(config["mitto_base_url"], defaultMittoBaseURL)); err != nil {
		return fmt.Errorf("mitto_base_url: %w", err)
	}
	return nil
}

func validateSalesmanagoIntegration(config map[string]string) error {
	if strings.TrimSpace(config["salesmanago_api_key"]) == "" {
		return fmt.Errorf("salesmanago_api_key is required")
	}
	if err := validateHTTPURL(firstNonEmpty(config["salesmanago_base_url"], defaultSalesmanagoBaseURL)); err != nil {
		return fmt.Errorf("salesmanago_base_url: %w", err)
	}
	return nil
}

func validateHTTPURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("URL is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL must use http or https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return fmt.Errorf("URL host is required")
	}
	return nil
}

type validationOption struct {
	Label   string
	Hint    string
	Outcome integrationValidationOutcome
}

func selectValidationOutcome(wizard *cli.Wizard, title, help string, options []validationOption) integrationValidationOutcome {
	items := make([]cli.MenuItem, 0, len(options))
	for _, option := range options {
		items = append(items, cli.MenuItem{Label: option.Label, Hint: option.Hint})
	}
	index, ok := wizard.Select(title, help, items, 0)
	if !ok {
		return integrationValidationSkip
	}
	return options[index].Outcome
}

func reviewConfiguration(p *cli.Printer, wizard *cli.Wizard, next map[string]string, selected map[string]bool, secretsPath string) integrationReviewOutcome {
	p.Blank()
	p.Rule("Review")
	p.Blank()

	p.Box("Brain", [][2]string{
		{"Provider", currentBrainLabel(next)},
		{"Model", firstNonEmpty(next["llm_model"], "[not set]")},
		{"Endpoint", firstNonEmpty(next["llm_base_url"], "[not set]")},
	})
	p.Blank()

	channelRows := [][2]string{{"Activated", activatedIntegrationsLabel(selected)}}
	for _, def := range integrationDefinitions {
		channelRows = append(channelRows, [2]string{def.Label, integrationStatusLabel(selected[def.ID])})
	}
	p.Box("Channels", channelRows)
	p.Blank()

	secretRows := [][2]string{{"LLM API key", redactSecretValue(next["llm_api_key"])}}
	for _, def := range integrationDefinitions {
		for _, key := range def.Keys {
			if strings.Contains(key, "api_key") || strings.Contains(key, "token") || strings.Contains(key, "webhook_url") {
				secretRows = append(secretRows, [2]string{key, redactSecretValue(next[key])})
			}
		}
	}
	p.Box("Secrets", secretRows)
	p.Blank()

	p.Box("Destination", [][2]string{
		{"Config file", secretsPath},
	})
	p.Blank()

	index, ok := wizard.Select(
		"Review configuration",
		"Save, go back to channel setup, or cancel without writing the file.",
		[]cli.MenuItem{
			{Label: "Save configuration", Hint: "Write the reviewed settings to disk."},
			{Label: "Adjust channel setup", Hint: "Return to the channel selection flow."},
			{Label: "Cancel without saving", Hint: "Exit init and leave the file unchanged."},
		},
		0,
	)
	if !ok {
		return integrationReviewCancel
	}

	switch index {
	case 0:
		return integrationReviewSave
	case 1:
		return integrationReviewAdjust
	default:
		return integrationReviewCancel
	}
}

func currentIntegrationSelection(config map[string]string) map[string]bool {
	selected := make(map[string]bool, len(integrationDefinitions))
	for _, definition := range integrationDefinitions {
		for _, key := range definition.Keys {
			if strings.TrimSpace(config[key]) != "" {
				selected[definition.ID] = true
				break
			}
		}
	}
	return selected
}

func clearDeselectedIntegrationKeys(config map[string]string, selected map[string]bool) {
	for _, definition := range integrationDefinitions {
		if selected[definition.ID] {
			continue
		}
		for _, key := range definition.Keys {
			delete(config, key)
		}
	}
}

func clearIntegrationKeys(config map[string]string, integrationID string) {
	for _, definition := range integrationDefinitions {
		if definition.ID != integrationID {
			continue
		}
		for _, key := range definition.Keys {
			delete(config, key)
		}
		return
	}
}

func activatedIntegrationsLabel(selected map[string]bool) string {
	labels := make([]string, 0, len(integrationDefinitions))
	for _, definition := range integrationDefinitions {
		if selected[definition.ID] {
			labels = append(labels, definition.Label)
		}
	}
	if len(labels) == 0 {
		return "None"
	}
	return strings.Join(labels, ", ")
}

func integrationStatusLabel(enabled bool) string {
	if enabled {
		return "Enabled"
	}
	return "Disabled"
}

func redactSecretValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "[not set]"
	}
	if len(value) <= 4 {
		return "[set]"
	}
	return strings.Repeat("*", len(value)-4) + value[len(value)-4:]
}
