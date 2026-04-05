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

// HubSpotAdapter implements engine.MarketingChannel for the HubSpot CRM API.
type HubSpotAdapter struct {
	client *http.Client
}

var _ engine.MarketingChannel = (*HubSpotAdapter)(nil)

// NewHubSpotAdapter creates a HubSpot CRM adapter with a default HTTP client.
func NewHubSpotAdapter() *HubSpotAdapter {
	return &HubSpotAdapter{
		client: newAdapterClient(),
	}
}

func (a *HubSpotAdapter) Name() string {
	return "hubspot"
}

func (a *HubSpotAdapter) Kind() string {
	return "crm"
}

func (a *HubSpotAdapter) SecretKeys() []string {
	return []string{"hubspot_api_key"}
}

func (a *HubSpotAdapter) Status(secrets engine.SecretProvider) engine.ChannelProviderStatus {
	status := engine.ChannelProviderStatus{
		Provider:     a.Name(),
		Channel:      a.Kind(),
		Healthy:      false,
		Configured:   false,
		Capabilities: []string{"create_contact", "update_contact"},
		Message:      "HubSpot is not configured yet.",
	}

	missing := missingSecrets(secrets, "hubspot_api_key")
	if len(missing) > 0 {
		status.Message = "Missing " + strings.Join(missing, ", ") + "."
		return status
	}

	status.Configured = true
	status.Healthy = true
	status.Message = "HubSpot CRM adapter is configured."
	return status
}

func (a *HubSpotAdapter) Test(ctx context.Context, secrets engine.SecretProvider) (engine.ChannelProviderStatus, error) {
	status := a.Status(secrets)
	if !status.Configured {
		return status, fmt.Errorf(status.Message)
	}

	apiKey, _ := secrets.Get("hubspot_api_key")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.hubapi.com/crm/v3/objects/contacts?limit=1", nil)
	if err != nil {
		return status, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		status.Healthy = false
		status.Message = err.Error()
		return status, err
	}
	defer resp.Body.Close()

	_, err = readAdapterResponse(resp, "hubspot")
	if err != nil {
		status.Healthy = false
		status.Message = err.Error()
		return status, err
	}

	status.Healthy = true
	status.Message = "HubSpot credentials verified successfully."
	return status, nil
}

func (a *HubSpotAdapter) Execute(ctx context.Context, action engine.AdapterAction, secrets engine.SecretProvider) (engine.AdapterResult, error) {
	apiKey, err := secrets.Get("hubspot_api_key")
	if err != nil {
		return engine.AdapterResult{}, fmt.Errorf("hubspot_api_key: %w", err)
	}

	switch action.Operation {
	case "create_contact":
		return a.createContact(ctx, apiKey, action)
	case "update_contact":
		return a.updateContact(ctx, apiKey, action)
	default:
		return engine.AdapterResult{}, fmt.Errorf("unsupported HubSpot operation %q", action.Operation)
	}
}

func (a *HubSpotAdapter) createContact(ctx context.Context, apiKey string, action engine.AdapterAction) (engine.AdapterResult, error) {
	body := map[string]any{
		"properties": action.Payload,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return engine.AdapterResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.hubapi.com/crm/v3/objects/contacts", bytes.NewReader(payload))
	if err != nil {
		return engine.AdapterResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return engine.AdapterResult{}, err
	}
	defer resp.Body.Close()

	decoded, err := readAdapterResponse(resp, "hubspot")
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
		},
	}, nil
}

func (a *HubSpotAdapter) updateContact(ctx context.Context, apiKey string, action engine.AdapterAction) (engine.AdapterResult, error) {
	contactID := strings.TrimSpace(conv.AsString(action.Payload["id"]))
	if contactID == "" {
		return engine.AdapterResult{}, fmt.Errorf("hubspot update_contact requires payload.id")
	}

	// Build properties without the id field.
	properties := make(map[string]any, len(action.Payload))
	for k, v := range action.Payload {
		if k == "id" {
			continue
		}
		properties[k] = v
	}

	body := map[string]any{
		"properties": properties,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return engine.AdapterResult{}, err
	}

	endpoint := "https://api.hubapi.com/crm/v3/objects/contacts/" + contactID

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(payload))
	if err != nil {
		return engine.AdapterResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return engine.AdapterResult{}, err
	}
	defer resp.Body.Close()

	decoded, err := readAdapterResponse(resp, "hubspot")
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
			"contact_id":  contactID,
		},
	}, nil
}
