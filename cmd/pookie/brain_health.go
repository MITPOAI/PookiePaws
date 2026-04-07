package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/brain"
	"github.com/mitpoai/pookiepaws/internal/cli"
)

func checkStackBrainHealth(ctx context.Context, stack *appStack) brain.ProviderHealth {
	if stack == nil {
		return brain.ProviderHealth{
			Provider:    "OpenAI-compatible",
			Mode:        "disabled",
			FailureCode: brain.ProviderFailureNotConfigured,
			Detail:      "Runtime stack is not available.",
			CheckedAt:   time.Now().UTC(),
		}
	}
	return brain.CheckProviderHealth(ctx, stack.secrets)
}

func printBrainHealth(p *cli.Printer, health brain.ProviderHealth) {
	if p == nil {
		return
	}
	p.Box("Brain Health", [][2]string{
		{"provider", firstValue(health.Provider, "-")},
		{"mode", firstValue(health.Mode, "-")},
		{"model", firstValue(health.Model, "-")},
		{"endpoint", firstValue(health.BaseURL, "-")},
		{"config present", fmt.Sprintf("%t", health.ConfigPresent)},
		{"endpoint reachable", fmt.Sprintf("%t", health.EndpointReachable)},
		{"credentials accepted", fmt.Sprintf("%t", health.CredentialsAccepted)},
		{"model accepted", fmt.Sprintf("%t", health.ModelAccepted)},
		{"detail", firstValue(health.Detail, "-")},
		{"error", firstValue(health.Error, "-")},
	})
	p.Blank()
}

func printBrainRemediation(p *cli.Printer, health brain.ProviderHealth) {
	if p == nil || health.Healthy() {
		return
	}
	p.Warning("%s", brainHealthRemediation(health))
	p.Blank()
}

func brainHealthRemediation(health brain.ProviderHealth) string {
	switch health.FailureCode {
	case brain.ProviderFailureNotConfigured:
		return "Run pookie init and complete the brain provider setup before using chat."
	case brain.ProviderFailureEndpointUnusable, brain.ProviderFailureEndpointRejected:
		return "Check llm_base_url first. The saved chat-completions endpoint is not usable."
	case brain.ProviderFailureCredentials:
		return "Check llm_api_key. The provider responded, but it rejected the saved credentials."
	case brain.ProviderFailureModel:
		return fmt.Sprintf("Check llm_model. The provider rejected %q for %s.", firstValue(health.Model, "the saved model"), firstValue(health.Provider, "the configured provider"))
	case brain.ProviderFailureUnsupported:
		return "This provider type is configured, but doctor/smoke validation is not implemented for it yet."
	default:
		return "Run pookie doctor --brain to inspect the provider, endpoint, and model validation details."
	}
}

func technicalDispatchError(err error) string {
	var friendly brain.FriendlyError
	if errors.As(err, &friendly) && friendly.Technical != nil {
		return strings.TrimSpace(friendly.Technical.Error())
	}
	return strings.TrimSpace(err.Error())
}
