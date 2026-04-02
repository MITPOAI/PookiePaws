package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type MockSalesmanagoAdapter struct{}

func NewMockSalesmanagoAdapter() *MockSalesmanagoAdapter {
	return &MockSalesmanagoAdapter{}
}

func (a *MockSalesmanagoAdapter) Name() string {
	return "salesmanago"
}

func (a *MockSalesmanagoAdapter) Execute(_ context.Context, action engine.AdapterAction, _ engine.SecretProvider) (engine.AdapterResult, error) {
	if action.Operation != "route_lead" {
		return engine.AdapterResult{}, fmt.Errorf("unsupported CRM operation %q", action.Operation)
	}
	return engine.AdapterResult{
		Adapter:   a.Name(),
		Operation: action.Operation,
		Status:    "mocked",
		Details: map[string]any{
			"executed_at": time.Now().UTC(),
			"payload":     action.Payload,
		},
	}, nil
}

type MockMittoAdapter struct{}

func NewMockMittoAdapter() *MockMittoAdapter {
	return &MockMittoAdapter{}
}

func (a *MockMittoAdapter) Name() string {
	return "mitto"
}

func (a *MockMittoAdapter) Execute(_ context.Context, action engine.AdapterAction, _ engine.SecretProvider) (engine.AdapterResult, error) {
	if action.Operation != "send_sms" {
		return engine.AdapterResult{}, fmt.Errorf("unsupported SMS operation %q", action.Operation)
	}
	return engine.AdapterResult{
		Adapter:   a.Name(),
		Operation: action.Operation,
		Status:    "mocked",
		Details: map[string]any{
			"executed_at": time.Now().UTC(),
			"payload":     action.Payload,
		},
	}, nil
}

type MockWhatsAppAdapter struct{}

func NewMockWhatsAppAdapter() *MockWhatsAppAdapter {
	return &MockWhatsAppAdapter{}
}

func (a *MockWhatsAppAdapter) Name() string {
	return "meta_cloud"
}

func (a *MockWhatsAppAdapter) Channel() string {
	return "whatsapp"
}

func (a *MockWhatsAppAdapter) Status(_ engine.SecretProvider) engine.ChannelProviderStatus {
	return engine.ChannelProviderStatus{
		Provider:     a.Name(),
		Channel:      a.Channel(),
		Configured:   true,
		Healthy:      true,
		Message:      "Mock WhatsApp adapter ready.",
		Capabilities: []string{"text", "template", "delivery_status"},
	}
}

func (a *MockWhatsAppAdapter) Test(_ context.Context, _ engine.SecretProvider) (engine.ChannelProviderStatus, error) {
	return a.Status(nil), nil
}

func (a *MockWhatsAppAdapter) Send(_ context.Context, req engine.ChannelSendRequest, _ engine.SecretProvider) (engine.ChannelSendResult, error) {
	if req.Type != "text" && req.Type != "template" {
		return engine.ChannelSendResult{}, fmt.Errorf("unsupported WhatsApp operation %q", req.Type)
	}
	return engine.ChannelSendResult{
		MessageID:  req.MessageID,
		ExternalID: fmt.Sprintf("wamid.mock.%d", time.Now().UTC().UnixNano()),
		Provider:   a.Name(),
		Channel:    a.Channel(),
		Status:     "sent",
		Details: map[string]any{
			"executed_at": time.Now().UTC(),
			"payload":     req,
		},
	}, nil
}

func (a *MockWhatsAppAdapter) ParseDeliveryEvents(payload map[string]any) []engine.ChannelDeliveryEvent {
	if payload == nil {
		return nil
	}
	return []engine.ChannelDeliveryEvent{{
		Provider:  a.Name(),
		Channel:   a.Channel(),
		Status:    "delivered",
		Timestamp: time.Now().UTC(),
		Raw:       payload,
	}}
}

func (a *MockWhatsAppAdapter) ParseIncomingMessages(_ map[string]any) []engine.ChannelIncomingMessage {
	return nil
}
