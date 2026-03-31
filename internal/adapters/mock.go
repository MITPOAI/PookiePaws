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
