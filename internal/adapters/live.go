package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/conv"
	"github.com/mitpoai/pookiepaws/internal/engine"
)

type SalesmanagoAdapter struct {
	client *http.Client
}

func NewSalesmanagoAdapter() *SalesmanagoAdapter {
	return &SalesmanagoAdapter{
		client: newAdapterClient(),
	}
}

func (a *SalesmanagoAdapter) Name() string {
	return "salesmanago"
}

func (a *SalesmanagoAdapter) Execute(ctx context.Context, action engine.AdapterAction, secrets engine.SecretProvider) (engine.AdapterResult, error) {
	if action.Operation != "route_lead" {
		return engine.AdapterResult{}, fmt.Errorf("unsupported CRM operation %q", action.Operation)
	}

	apiKey, err := secrets.Get("salesmanago_api_key")
	if err != nil {
		return engine.AdapterResult{}, fmt.Errorf("salesmanago_api_key: %w", err)
	}
	baseURL := secretWithFallback(secrets, "salesmanago_base_url", "https://api.salesmanago.com/v3/keyInformation/upsert")
	owner, _ := secrets.Get("salesmanago_owner")

	email, contactID := resolveSalesmanagoIdentifiers(action.Payload)
	if email == "" && contactID == "" {
		return engine.AdapterResult{}, fmt.Errorf("salesmanago route_lead requires email or contact_id")
	}

	text := make([]map[string]string, 0, 5)
	appendText := func(name, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		text = append(text, map[string]string{
			"name":  name,
			"value": value,
		})
	}
	appendText("Route queue", conv.AsString(action.Payload["route_queue"]))
	appendText("Segment", conv.AsString(action.Payload["segment"]))
	appendText("Priority", conv.AsString(action.Payload["priority"]))
	appendText("Lead name", conv.AsString(action.Payload["name"]))
	appendText("Lead phone", conv.AsString(action.Payload["phone"]))

	body := map[string]any{
		"keyInformation": map[string]any{
			"text": text,
		},
	}
	if owner != "" {
		body["owner"] = owner
	}
	if email != "" {
		body["email"] = email
	}
	if contactID != "" {
		body["contactid"] = contactID
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return engine.AdapterResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(payload))
	if err != nil {
		return engine.AdapterResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("API-KEY", apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return engine.AdapterResult{}, err
	}
	defer resp.Body.Close()

	decoded, err := readAdapterResponse(resp, "salesmanago")
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
			"email":       email,
			"contact_id":  contactID,
		},
	}, nil
}

type MittoAdapter struct {
	client *http.Client
}

func NewMittoAdapter() *MittoAdapter {
	return &MittoAdapter{
		client: newAdapterClient(),
	}
}

func (a *MittoAdapter) Name() string {
	return "mitto"
}

func (a *MittoAdapter) Execute(ctx context.Context, action engine.AdapterAction, secrets engine.SecretProvider) (engine.AdapterResult, error) {
	if action.Operation != "send_sms" {
		return engine.AdapterResult{}, fmt.Errorf("unsupported SMS operation %q", action.Operation)
	}

	apiKey, err := secrets.Get("mitto_api_key")
	if err != nil {
		return engine.AdapterResult{}, fmt.Errorf("mitto_api_key: %w", err)
	}
	baseURL := strings.TrimRight(secretWithFallback(secrets, "mitto_base_url", "https://rest.mittoapi.net"), "/")
	from := strings.TrimSpace(conv.AsString(action.Payload["from"]))
	if from == "" {
		from, _ = secrets.Get("mitto_from")
	}
	if from == "" {
		return engine.AdapterResult{}, fmt.Errorf("mitto sender is required via payload.from or mitto_from secret")
	}

	recipients := conv.AsStringSlice(action.Payload["recipients"])
	if len(recipients) == 0 {
		return engine.AdapterResult{}, fmt.Errorf("mitto recipients are required")
	}
	message := strings.TrimSpace(conv.AsString(action.Payload["message"]))
	if message == "" {
		return engine.AdapterResult{}, fmt.Errorf("mitto message is required")
	}

	endpoint := baseURL + "/sms"
	body := map[string]any{
		"from": from,
		"text": message,
		"to":   recipients[0],
	}
	if len(recipients) > 1 {
		endpoint = baseURL + "/smsbulk"
		body["to"] = recipients
	}
	if testMode, ok := action.Payload["test"].(bool); ok && testMode {
		body["test"] = true
	}
	if campaign := strings.TrimSpace(conv.AsString(action.Payload["campaign_name"])); campaign != "" {
		body["reference"] = campaign
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return engine.AdapterResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return engine.AdapterResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Mitto-API-Key", apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return engine.AdapterResult{}, err
	}
	defer resp.Body.Close()

	decoded, err := readAdapterResponse(resp, "mitto")
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
			"recipients":  recipients,
		},
	}, nil
}

func resolveSalesmanagoIdentifiers(payload map[string]any) (string, string) {
	email := strings.TrimSpace(conv.AsString(payload["email"]))
	contactID := strings.TrimSpace(conv.AsString(payload["contact_id"]))
	leadID := strings.TrimSpace(conv.AsString(payload["lead_id"]))
	if email == "" && strings.Contains(leadID, "@") {
		email = leadID
	}
	if contactID == "" && leadID != "" && !strings.Contains(leadID, "@") {
		contactID = leadID
	}
	return email, contactID
}

func newAdapterClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func readAdapterResponse(resp *http.Response, adapterName string) (map[string]any, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("%s API failed with status %d: %s", adapterName, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded map[string]any
	if len(body) > 0 {
		_ = json.Unmarshal(body, &decoded)
	}
	return decoded, nil
}

func secretWithFallback(secrets engine.SecretProvider, key, fallback string) string {
	if configured, err := secrets.Get(key); err == nil && strings.TrimSpace(configured) != "" {
		return strings.TrimSpace(configured)
	}
	return fallback
}

// ── MarketingChannel conformance for SalesmanagoAdapter ────────────────────

var _ engine.MarketingChannel = (*SalesmanagoAdapter)(nil)

func (a *SalesmanagoAdapter) Kind() string { return "crm" }

func (a *SalesmanagoAdapter) Status(secrets engine.SecretProvider) engine.ChannelProviderStatus {
	apiKey, _ := secrets.Get("salesmanago_api_key")
	configured := strings.TrimSpace(apiKey) != ""
	msg := "SALESmanago API key configured"
	if !configured {
		msg = "SALESmanago API key not set"
	}
	return engine.ChannelProviderStatus{
		Provider:   "salesmanago",
		Channel:    "crm",
		Configured: configured,
		Healthy:    configured,
		Message:    msg,
	}
}

func (a *SalesmanagoAdapter) Test(ctx context.Context, secrets engine.SecretProvider) (engine.ChannelProviderStatus, error) {
	status := a.Status(secrets)
	if !status.Configured {
		return status, fmt.Errorf("salesmanago_api_key is not configured")
	}
	return status, nil
}

func (a *SalesmanagoAdapter) SecretKeys() []string {
	return []string{"salesmanago_api_key", "salesmanago_base_url", "salesmanago_owner"}
}

// ── MarketingChannel conformance for MittoAdapter ──────────────────────────

var _ engine.MarketingChannel = (*MittoAdapter)(nil)

func (a *MittoAdapter) Kind() string { return "sms" }

func (a *MittoAdapter) Status(secrets engine.SecretProvider) engine.ChannelProviderStatus {
	apiKey, _ := secrets.Get("mitto_api_key")
	configured := strings.TrimSpace(apiKey) != ""
	msg := "Mitto API key configured"
	if !configured {
		msg = "Mitto API key not set"
	}
	return engine.ChannelProviderStatus{
		Provider:   "mitto",
		Channel:    "sms",
		Configured: configured,
		Healthy:    configured,
		Message:    msg,
	}
}

func (a *MittoAdapter) Test(ctx context.Context, secrets engine.SecretProvider) (engine.ChannelProviderStatus, error) {
	status := a.Status(secrets)
	if !status.Configured {
		return status, fmt.Errorf("mitto_api_key is not configured")
	}
	return status, nil
}

func (a *MittoAdapter) SecretKeys() []string {
	return []string{"mitto_api_key", "mitto_base_url", "mitto_from"}
}
