package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DiscordWebhookNotifier sends formatted messages to a Discord webhook.
type DiscordWebhookNotifier struct {
	client     *http.Client
	webhookURL string
}

var _ WebhookNotifier = (*DiscordWebhookNotifier)(nil)

// NewDiscordWebhookNotifier creates a notifier for the given Discord webhook URL.
func NewDiscordWebhookNotifier(webhookURL string) *DiscordWebhookNotifier {
	return &DiscordWebhookNotifier{
		client:     newAdapterClient(),
		webhookURL: strings.TrimSpace(webhookURL),
	}
}

// Send posts a WebhookPayload to the Discord webhook as an embed message.
func (d *DiscordWebhookNotifier) Send(ctx context.Context, payload WebhookPayload) error {
	if d.webhookURL == "" {
		return fmt.Errorf("discord webhook URL is not configured")
	}

	// Build Discord embed fields.
	fields := make([]map[string]any, 0, len(payload.Fields))
	for key, value := range payload.Fields {
		fields = append(fields, map[string]any{
			"name":   key,
			"value":  value,
			"inline": true,
		})
	}

	embed := map[string]any{
		"title":       payload.Title,
		"description": payload.Summary,
		"color":       0xCF3D74, // PookiePaws pink
		"fields":      fields,
	}
	if !payload.Timestamp.IsZero() {
		embed["timestamp"] = payload.Timestamp.Format("2006-01-02T15:04:05Z")
	}

	body := map[string]any{
		"embeds": []map[string]any{embed},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.webhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("send discord webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("discord webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}
