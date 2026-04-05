package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// SlackWebhookNotifier sends formatted messages to a Slack incoming webhook.
type SlackWebhookNotifier struct {
	client     *http.Client
	webhookURL string
}

var _ WebhookNotifier = (*SlackWebhookNotifier)(nil)

// NewSlackWebhookNotifier creates a notifier for the given Slack webhook URL.
func NewSlackWebhookNotifier(webhookURL string) *SlackWebhookNotifier {
	return &SlackWebhookNotifier{
		client:     newAdapterClient(),
		webhookURL: strings.TrimSpace(webhookURL),
	}
}

// Send posts a WebhookPayload to the Slack webhook as a Block Kit message.
func (s *SlackWebhookNotifier) Send(ctx context.Context, payload WebhookPayload) error {
	if s.webhookURL == "" {
		return fmt.Errorf("slack webhook URL is not configured")
	}

	// Build Slack Block Kit blocks.
	blocks := []map[string]any{
		{
			"type": "header",
			"text": map[string]any{
				"type": "plain_text",
				"text": payload.Title,
			},
		},
		{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": payload.Summary,
			},
		},
	}

	// Add fields as a section with field items.
	if len(payload.Fields) > 0 {
		fields := make([]map[string]any, 0, len(payload.Fields))
		for key, value := range payload.Fields {
			fields = append(fields, map[string]any{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*%s*\n%s", key, value),
			})
		}
		blocks = append(blocks, map[string]any{
			"type":   "section",
			"fields": fields,
		})
	}

	// Add timestamp as context.
	if !payload.Timestamp.IsZero() {
		blocks = append(blocks, map[string]any{
			"type": "context",
			"elements": []map[string]any{
				{
					"type": "mrkdwn",
					"text": fmt.Sprintf("Generated at %s", payload.Timestamp.Format("2006-01-02 15:04 UTC")),
				},
			},
		})
	}

	body := map[string]any{"blocks": blocks}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}
