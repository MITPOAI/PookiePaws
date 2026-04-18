package brain

import (
	"encoding/json"
	"testing"
)

func TestToolDefinitionJSONShape(t *testing.T) {
	def := ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "web_search",
			Description: "fetch a url",
			Parameters: JSONSchema{
				Type: "object",
				Properties: map[string]SchemaProperty{
					"url": {Type: "string", Description: "target url"},
				},
				Required: []string{"url"},
			},
		},
	}
	b, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["type"] != "function" {
		t.Errorf("want type=function, got %v", out["type"])
	}
	fn, _ := out["function"].(map[string]any)
	if fn["name"] != "web_search" {
		t.Errorf("want function.name=web_search, got %v", fn["name"])
	}
	params, _ := fn["parameters"].(map[string]any)
	if params["type"] != "object" {
		t.Errorf("want parameters.type=object, got %v", params["type"])
	}
}

func TestChatMessageToolCallsRoundTrip(t *testing.T) {
	msg := ChatMessage{
		Role: "assistant",
		ToolCalls: []ToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: ToolCallFunc{Name: "web_search", Arguments: `{"url":"https://example.com"}`},
		}},
	}
	b, _ := json.Marshal(msg)
	var out ChatMessage
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].ID != "call_1" {
		t.Errorf("tool call not preserved: %+v", out)
	}
}
