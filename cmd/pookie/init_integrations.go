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
		ID:    "whatsapp",
		Label: "Meta WhatsApp",
		Hint:  "Marketing control plane with approval-gated outbound sends.",
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
		Label: "Mitto SMS",
		Hint:  "Outbound SMS campaigns through Mitto.",
		Keys: []string{
			"mitto_api_key",
			"mitto_base_url",
			"mitto_from",
		},
	},
	{
		ID:    "salesmanago",
		Label: "SALESmanago",
		Hint:  "CRM and lead management routing.",
		Keys: []string{
			"salesmanago_base_url",
			"salesmanago_api_key",
			"salesmanago_owner",
		},
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
	if selected["whatsapp"] {
		ok := configureWhatsAppIntegration(reader, p, wizard, next, ask)
		if !ok {
			return false
		}
	}
	if selected["mitto"] {
		ok := configureMittoIntegration(p, wizard, next, ask)
		if !ok {
			return false
		}
	}
	if selected["salesmanago"] {
		ok := configureSalesmanagoIntegration(p, wizard, next, ask)
		if !ok {
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

	p.Box("Channels", [][2]string{
		{"Activated", activatedIntegrationsLabel(selected)},
		{"WhatsApp", integrationStatusLabel(selected["whatsapp"])},
		{"Mitto SMS", integrationStatusLabel(selected["mitto"])},
		{"SALESmanago", integrationStatusLabel(selected["salesmanago"])},
	})
	p.Blank()

	p.Box("Secrets", [][2]string{
		{"LLM API key", redactSecretValue(next["llm_api_key"])},
		{"WhatsApp token", redactSecretValue(next["whatsapp_access_token"])},
		{"WhatsApp webhook", redactSecretValue(next["whatsapp_webhook_verify_token"])},
		{"Mitto API key", redactSecretValue(next["mitto_api_key"])},
		{"SALESmanago API key", redactSecretValue(next["salesmanago_api_key"])},
	})
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
