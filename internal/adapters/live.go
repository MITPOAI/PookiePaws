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

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type SalesmanagoAdapter struct {
	client *http.Client
}

func NewSalesmanagoAdapter() *SalesmanagoAdapter {
	return &SalesmanagoAdapter{
		client: &http.Client{Timeout: 30 * time.Second},
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
	baseURL := "https://api.salesmanago.com/v3/keyInformation/upsert"
	if configured, err := secrets.Get("salesmanago_base_url"); err == nil && strings.TrimSpace(configured) != "" {
		baseURL = strings.TrimSpace(configured)
	}
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
	appendText("Route queue", asString(action.Payload["route_queue"]))
	appendText("Segment", asString(action.Payload["segment"]))
	appendText("Priority", asString(action.Payload["priority"]))
	appendText("Lead name", asString(action.Payload["name"]))
	appendText("Lead phone", asString(action.Payload["phone"]))

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

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return engine.AdapterResult{}, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return engine.AdapterResult{}, fmt.Errorf("salesmanago API failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var decoded map[string]any
	if len(responseBody) > 0 {
		_ = json.Unmarshal(responseBody, &decoded)
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
		client: &http.Client{Timeout: 30 * time.Second},
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
	baseURL := "https://rest.mittoapi.net"
	if configured, err := secrets.Get("mitto_base_url"); err == nil && strings.TrimSpace(configured) != "" {
		baseURL = strings.TrimRight(strings.TrimSpace(configured), "/")
	}
	from := strings.TrimSpace(asString(action.Payload["from"]))
	if from == "" {
		from, _ = secrets.Get("mitto_from")
	}
	if from == "" {
		return engine.AdapterResult{}, fmt.Errorf("mitto sender is required via payload.from or mitto_from secret")
	}

	recipients := asStringSlice(action.Payload["recipients"])
	if len(recipients) == 0 {
		return engine.AdapterResult{}, fmt.Errorf("mitto recipients are required")
	}
	message := strings.TrimSpace(asString(action.Payload["message"]))
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
	if campaign := strings.TrimSpace(asString(action.Payload["campaign_name"])); campaign != "" {
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

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return engine.AdapterResult{}, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return engine.AdapterResult{}, fmt.Errorf("mitto API failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var decoded map[string]any
	if len(responseBody) > 0 {
		_ = json.Unmarshal(responseBody, &decoded)
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
	email := strings.TrimSpace(asString(payload["email"]))
	contactID := strings.TrimSpace(asString(payload["contact_id"]))
	leadID := strings.TrimSpace(asString(payload["lead_id"]))
	if email == "" && strings.Contains(leadID, "@") {
		email = leadID
	}
	if contactID == "" && leadID != "" && !strings.Contains(leadID, "@") {
		contactID = leadID
	}
	return email, contactID
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func asStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			items = append(items, asString(item))
		}
		return items
	default:
		return nil
	}
}
