package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/conv"
	"github.com/mitpoai/pookiepaws/internal/engine"
)

// ResendAdapter implements engine.MarketingChannel for the Resend email API.
type ResendAdapter struct {
	client *http.Client
}

var _ engine.MarketingChannel = (*ResendAdapter)(nil)

// NewResendAdapter creates a ResendAdapter with the shared HTTP client.
func NewResendAdapter() *ResendAdapter {
	return &ResendAdapter{
		client: newAdapterClient(),
	}
}

func (a *ResendAdapter) Name() string {
	return "resend"
}

func (a *ResendAdapter) Kind() string {
	return "email"
}

func (a *ResendAdapter) SecretKeys() []string {
	return []string{"resend_api_key", "resend_from"}
}

func (a *ResendAdapter) Status(secrets engine.SecretProvider) engine.ChannelProviderStatus {
	status := engine.ChannelProviderStatus{
		Provider:     a.Name(),
		Channel:      a.Kind(),
		Configured:   false,
		Healthy:      false,
		Capabilities: []string{"send_email"},
		Message:      "Resend is not configured yet.",
	}

	apiKey, err := secrets.Get("resend_api_key")
	if err != nil || strings.TrimSpace(apiKey) == "" {
		status.Message = "Missing resend_api_key."
		return status
	}

	status.Configured = true
	status.Healthy = true
	status.Message = "Resend email adapter is configured."
	return status
}

func (a *ResendAdapter) Test(ctx context.Context, secrets engine.SecretProvider) (engine.ChannelProviderStatus, error) {
	status := a.Status(secrets)
	if !status.Configured {
		return status, fmt.Errorf(status.Message)
	}

	apiKey, _ := secrets.Get("resend_api_key")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.resend.com/domains", nil)
	if err != nil {
		return status, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))

	resp, err := a.client.Do(req)
	if err != nil {
		status.Healthy = false
		status.Message = err.Error()
		return status, err
	}
	defer resp.Body.Close()

	_, err = readAdapterResponse(resp, "resend")
	if err != nil {
		status.Healthy = false
		status.Message = err.Error()
		return status, err
	}

	status.Healthy = true
	status.Message = "Resend credentials verified successfully."
	return status, nil
}

func (a *ResendAdapter) Execute(ctx context.Context, action engine.AdapterAction, secrets engine.SecretProvider) (engine.AdapterResult, error) {
	if action.Operation != "send_email" {
		return engine.AdapterResult{}, fmt.Errorf("unsupported email operation %q", action.Operation)
	}

	apiKey, err := secrets.Get("resend_api_key")
	if err != nil {
		return engine.AdapterResult{}, fmt.Errorf("resend_api_key: %w", err)
	}

	from := strings.TrimSpace(conv.AsString(action.Payload["from"]))
	if from == "" {
		from = secretWithFallback(secrets, "resend_from", "")
	}
	if from == "" {
		return engine.AdapterResult{}, fmt.Errorf("resend sender is required via payload.from or resend_from secret")
	}

	to := strings.TrimSpace(conv.AsString(action.Payload["to"]))
	if to == "" {
		return engine.AdapterResult{}, fmt.Errorf("resend recipient (to) is required")
	}

	subject := strings.TrimSpace(conv.AsString(action.Payload["subject"]))
	if subject == "" {
		return engine.AdapterResult{}, fmt.Errorf("resend subject is required")
	}

	body := map[string]any{
		"from":    from,
		"to":      to,
		"subject": subject,
	}
	if html := strings.TrimSpace(conv.AsString(action.Payload["html"])); html != "" {
		body["html"] = html
	}
	if text := strings.TrimSpace(conv.AsString(action.Payload["text"])); text != "" {
		body["text"] = text
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return engine.AdapterResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		return engine.AdapterResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))

	resp, err := a.client.Do(req)
	if err != nil {
		return engine.AdapterResult{}, err
	}
	defer resp.Body.Close()

	decoded, err := readAdapterResponse(resp, "resend")
	if err != nil {
		return engine.AdapterResult{}, err
	}

	return engine.AdapterResult{
		Adapter:   a.Name(),
		Operation: action.Operation,
		Status:    "sent",
		Details: map[string]any{
			"status_code": resp.StatusCode,
			"response":    decoded,
			"to":          to,
			"subject":     subject,
		},
	}, nil
}
