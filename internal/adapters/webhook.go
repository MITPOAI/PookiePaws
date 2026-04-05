package adapters

import (
	"context"
	"time"
)

// WebhookPayload carries a formatted summary for delivery to Slack or Discord.
type WebhookPayload struct {
	Title     string
	Summary   string
	Timestamp time.Time
	Fields    map[string]string
}

// WebhookNotifier sends structured payloads to external webhook endpoints.
type WebhookNotifier interface {
	Send(ctx context.Context, payload WebhookPayload) error
}
