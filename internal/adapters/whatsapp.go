package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

const (
	whatsAppProviderMetaCloud = "meta_cloud"
	whatsAppChannel           = "whatsapp"
)

type WhatsAppAdapter struct {
	client *http.Client
}

func NewWhatsAppAdapter() *WhatsAppAdapter {
	return &WhatsAppAdapter{client: newAdapterClient()}
}

func (a *WhatsAppAdapter) Name() string {
	return whatsAppProviderMetaCloud
}

func (a *WhatsAppAdapter) Channel() string {
	return whatsAppChannel
}

func (a *WhatsAppAdapter) Status(secrets engine.SecretProvider) engine.ChannelProviderStatus {
	status := engine.ChannelProviderStatus{
		Provider:     a.Name(),
		Channel:      a.Channel(),
		Healthy:      false,
		Configured:   false,
		Capabilities: []string{"text", "template", "delivery_status", "approval_gated_outbound"},
		Message:      "WhatsApp is not configured yet.",
	}

	missing := missingSecrets(secrets, "whatsapp_access_token", "whatsapp_phone_number_id")
	if len(missing) > 0 {
		status.Message = "Missing " + strings.Join(missing, ", ") + "."
		return status
	}

	status.Configured = true
	status.Healthy = true
	status.Message = fmt.Sprintf("Ready for outbound WhatsApp sends via %s.", configuredWhatsAppProvider(secrets))
	return status
}

func (a *WhatsAppAdapter) Test(ctx context.Context, secrets engine.SecretProvider) (engine.ChannelProviderStatus, error) {
	status := a.Status(secrets)
	if !status.Configured {
		return status, fmt.Errorf(status.Message)
	}

	baseURL := strings.TrimRight(secretWithFallback(secrets, "whatsapp_base_url", "https://graph.facebook.com/v23.0"), "/")
	phoneID, _ := secrets.Get("whatsapp_phone_number_id")
	accessToken, _ := secrets.Get("whatsapp_access_token")

	testURL := baseURL + "/" + strings.TrimSpace(phoneID)
	query := url.Values{}
	query.Set("fields", "display_phone_number,verified_name")
	testURL += "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
	if err != nil {
		return status, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := a.client.Do(req)
	if err != nil {
		status.Healthy = false
		status.Message = err.Error()
		return status, err
	}
	defer resp.Body.Close()

	decoded, err := readAdapterResponse(resp, "whatsapp")
	if err != nil {
		status.Healthy = false
		status.Message = err.Error()
		return status, err
	}

	status.Healthy = true
	status.Message = "WhatsApp credentials responded successfully."
	status.Capabilities = append(status.Capabilities, "connection_test")
	_ = decoded
	return status, nil
}

func (a *WhatsAppAdapter) Send(ctx context.Context, req engine.ChannelSendRequest, secrets engine.SecretProvider) (engine.ChannelSendResult, error) {
	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = configuredWhatsAppProvider(secrets)
	}
	if provider != whatsAppProviderMetaCloud && provider != "meta" {
		return engine.ChannelSendResult{}, fmt.Errorf("unsupported WhatsApp provider %q", provider)
	}

	accessToken, err := secrets.Get("whatsapp_access_token")
	if err != nil {
		return engine.ChannelSendResult{}, fmt.Errorf("whatsapp_access_token: %w", err)
	}
	phoneID, err := secrets.Get("whatsapp_phone_number_id")
	if err != nil {
		return engine.ChannelSendResult{}, fmt.Errorf("whatsapp_phone_number_id: %w", err)
	}
	if strings.TrimSpace(req.To) == "" {
		return engine.ChannelSendResult{}, fmt.Errorf("whatsapp recipient is required")
	}
	if strings.TrimSpace(req.Type) == "" {
		req.Type = "text"
	}
	if err := validateWhatsAppRequest(req); err != nil {
		return engine.ChannelSendResult{}, err
	}

	baseURL := strings.TrimRight(secretWithFallback(secrets, "whatsapp_base_url", "https://graph.facebook.com/v23.0"), "/")
	endpoint := baseURL + "/" + strings.TrimSpace(phoneID) + "/messages"
	body := map[string]any{
		"messaging_product": "whatsapp",
		"to":                req.To,
		"type":              req.Type,
	}
	if req.Test {
		body["preview_url"] = false
	}

	switch req.Type {
	case "text":
		body["text"] = map[string]any{
			"preview_url": false,
			"body":        strings.TrimSpace(req.Text),
		}
	case "template":
		template := map[string]any{
			"name": req.TemplateName,
			"language": map[string]any{
				"code": firstNonEmpty(req.TemplateLanguage, "en"),
			},
		}
		if len(req.TemplateVariables) > 0 {
			keys := make([]string, 0, len(req.TemplateVariables))
			for key := range req.TemplateVariables {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			parameters := make([]map[string]any, 0, len(keys))
			for _, key := range keys {
				parameters = append(parameters, map[string]any{
					"type": "text",
					"text": req.TemplateVariables[key],
				})
			}
			template["components"] = []map[string]any{{
				"type":       "body",
				"parameters": parameters,
			}}
		}
		body["template"] = template
	default:
		return engine.ChannelSendResult{}, fmt.Errorf("unsupported WhatsApp message type %q", req.Type)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return engine.ChannelSendResult{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return engine.ChannelSendResult{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return engine.ChannelSendResult{}, err
	}
	defer resp.Body.Close()

	decoded, err := readAdapterResponse(resp, "whatsapp")
	if err != nil {
		return engine.ChannelSendResult{}, err
	}

	externalID := ""
	if messages, ok := decoded["messages"].([]any); ok && len(messages) > 0 {
		if first, ok := messages[0].(map[string]any); ok {
			externalID = strings.TrimSpace(fmt.Sprint(first["id"]))
		}
	}

	return engine.ChannelSendResult{
		MessageID:  req.MessageID,
		ExternalID: externalID,
		Provider:   provider,
		Channel:    a.Channel(),
		Status:     "sent",
		Details: map[string]any{
			"status_code": resp.StatusCode,
			"response":    decoded,
			"recipient":   req.To,
			"type":        req.Type,
		},
	}, nil
}

func (a *WhatsAppAdapter) ParseDeliveryEvents(payload map[string]any) []engine.ChannelDeliveryEvent {
	events := make([]engine.ChannelDeliveryEvent, 0)
	entries, ok := payload["entry"].([]any)
	if !ok {
		return events
	}

	for _, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		changes, ok := entryMap["changes"].([]any)
		if !ok {
			continue
		}
		for _, rawChange := range changes {
			change, ok := rawChange.(map[string]any)
			if !ok {
				continue
			}
			value, ok := change["value"].(map[string]any)
			if !ok {
				continue
			}
			statuses, ok := value["statuses"].([]any)
			if !ok {
				continue
			}
			for _, rawStatus := range statuses {
				statusMap, ok := rawStatus.(map[string]any)
				if !ok {
					continue
				}
				events = append(events, engine.ChannelDeliveryEvent{
					Provider:   whatsAppProviderMetaCloud,
					Channel:    whatsAppChannel,
					ExternalID: strings.TrimSpace(fmt.Sprint(statusMap["id"])),
					Recipient:  strings.TrimSpace(fmt.Sprint(statusMap["recipient_id"])),
					Status:     strings.TrimSpace(fmt.Sprint(statusMap["status"])),
					Timestamp:  parseWhatsAppTimestamp(statusMap["timestamp"]),
					Raw:        statusMap,
				})
			}
		}
	}
	return events
}

func (a *WhatsAppAdapter) ParseIncomingMessages(payload map[string]any) []engine.ChannelIncomingMessage {
	var messages []engine.ChannelIncomingMessage
	entries, ok := payload["entry"].([]any)
	if !ok {
		return messages
	}

	for _, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		changes, ok := entryMap["changes"].([]any)
		if !ok {
			continue
		}
		for _, rawChange := range changes {
			change, ok := rawChange.(map[string]any)
			if !ok {
				continue
			}
			value, ok := change["value"].(map[string]any)
			if !ok {
				continue
			}

			// Build contacts lookup: wa_id → profile name.
			contactNames := map[string]string{}
			if contacts, ok := value["contacts"].([]any); ok {
				for _, rawContact := range contacts {
					contact, ok := rawContact.(map[string]any)
					if !ok {
						continue
					}
					waID := strings.TrimSpace(fmt.Sprint(contact["wa_id"]))
					if profile, ok := contact["profile"].(map[string]any); ok {
						contactNames[waID] = strings.TrimSpace(fmt.Sprint(profile["name"]))
					}
				}
			}

			// Extract incoming messages.
			rawMessages, ok := value["messages"].([]any)
			if !ok {
				continue
			}
			for _, rawMsg := range rawMessages {
				msgMap, ok := rawMsg.(map[string]any)
				if !ok {
					continue
				}
				msgType := strings.TrimSpace(fmt.Sprint(msgMap["type"]))
				from := strings.TrimSpace(fmt.Sprint(msgMap["from"]))
				msgID := strings.TrimSpace(fmt.Sprint(msgMap["id"]))

				msg := engine.ChannelIncomingMessage{
					Provider:  whatsAppProviderMetaCloud,
					Channel:   whatsAppChannel,
					MessageID: msgID,
					From:      from,
					FromName:  contactNames[from],
					Type:      msgType,
					Timestamp: parseWhatsAppTimestamp(msgMap["timestamp"]),
					Raw:       msgMap,
				}

				// Extract text body for text messages.
				if msgType == "text" {
					if textObj, ok := msgMap["text"].(map[string]any); ok {
						msg.Text = strings.TrimSpace(fmt.Sprint(textObj["body"]))
					}
				}

				messages = append(messages, msg)
			}
		}
	}
	return messages
}

func validateWhatsAppRequest(req engine.ChannelSendRequest) error {
	switch req.Type {
	case "text":
		if strings.TrimSpace(req.Text) == "" {
			return fmt.Errorf("whatsapp text message body is required")
		}
	case "template":
		if strings.TrimSpace(req.TemplateName) == "" {
			return fmt.Errorf("whatsapp template_name is required")
		}
	default:
		return fmt.Errorf("unsupported WhatsApp message type %q", req.Type)
	}
	return nil
}

func missingSecrets(secrets engine.SecretProvider, names ...string) []string {
	missing := make([]string, 0, len(names))
	for _, name := range names {
		value, err := secrets.Get(name)
		if err != nil || strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

func configuredWhatsAppProvider(secrets engine.SecretProvider) string {
	provider := secretWithFallback(secrets, "whatsapp_provider", whatsAppProviderMetaCloud)
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "meta", whatsAppProviderMetaCloud:
		return whatsAppProviderMetaCloud
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func parseWhatsAppTimestamp(value any) time.Time {
	raw := strings.TrimSpace(fmt.Sprint(value))
	if raw == "" {
		return time.Now().UTC()
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Now().UTC()
	}
	return time.Unix(seconds, 0).UTC()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
