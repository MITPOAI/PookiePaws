package main

import "testing"

func TestCurrentIntegrationSelection(t *testing.T) {
	selected := currentIntegrationSelection(map[string]string{
		"whatsapp_access_token": "wa-token",
		"mitto_api_key":         "mitto-key",
	})

	if !selected["whatsapp"] {
		t.Fatalf("expected whatsapp to be selected")
	}
	if !selected["mitto"] {
		t.Fatalf("expected mitto to be selected")
	}
	if selected["salesmanago"] {
		t.Fatalf("did not expect salesmanago to be selected")
	}
}

func TestClearDeselectedIntegrationKeys(t *testing.T) {
	config := map[string]string{
		"whatsapp_access_token": "wa-token",
		"mitto_api_key":         "mitto-key",
		"salesmanago_api_key":   "sales-key",
		"llm_model":             "gpt-5.1",
	}

	clearDeselectedIntegrationKeys(config, map[string]bool{
		"whatsapp":    true,
		"mitto":       false,
		"salesmanago": false,
	})

	if _, ok := config["whatsapp_access_token"]; !ok {
		t.Fatalf("expected whatsapp key to remain")
	}
	if _, ok := config["mitto_api_key"]; ok {
		t.Fatalf("expected mitto key to be removed")
	}
	if _, ok := config["salesmanago_api_key"]; ok {
		t.Fatalf("expected salesmanago key to be removed")
	}
	if config["llm_model"] != "gpt-5.1" {
		t.Fatalf("expected unrelated keys to remain untouched")
	}
}

func TestRedactSecretValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: "[not set]"},
		{name: "short", value: "abcd", want: "[set]"},
		{name: "long", value: "secret-token", want: "********oken"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactSecretValue(tt.value); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
