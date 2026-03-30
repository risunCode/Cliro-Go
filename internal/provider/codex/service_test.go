package codex

import (
	"encoding/json"
	"strings"
	"testing"

	"cliro-go/internal/adapter/decode"
	"cliro-go/internal/protocol/anthropic"
	provider "cliro-go/internal/provider"
)

func TestBuildRequestPayload_UsesStrictResponsesContentTypes(t *testing.T) {
	service := &Service{}
	payload, err := service.buildRequestPayload(provider.ChatRequest{
		Model: "gpt-5.4",
		Messages: []provider.Message{
			{Role: "system", Content: "be precise"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: []any{map[string]any{"type": "output_text", "text": "hi"}}},
			{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "call_1", Type: "function", Function: provider.ToolCallTarget{Name: "Read", Arguments: `{"path":"a.txt"}`}}}},
			{Role: "tool", ToolCallID: "call_1", Content: "done"},
		},
	})
	if err != nil {
		t.Fatalf("build request payload: %v", err)
	}
	encoded, _ := json.Marshal(payload)
	body := string(encoded)
	if !containsAll(body,
		`"role":"developer"`,
		`"type":"input_text"`,
		`"role":"assistant"`,
		`"type":"output_text"`,
		`"type":"function_call"`,
		`"type":"function_call_output"`,
		`"instructions":`,
	) {
		t.Fatalf("unexpected payload: %s", body)
	}
	if !containsAll(body, `"store":false`, `"include":["reasoning.encrypted_content"]`) {
		t.Fatalf("expected strict codex payload fields, got %s", body)
	}
}

func TestBuildRequestPayload_IncludesDefaultMarkdownInstructions(t *testing.T) {
	service := &Service{}
	payload, err := service.buildRequestPayload(provider.ChatRequest{
		Model:    "gpt-5.4",
		Messages: []provider.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("build request payload: %v", err)
	}
	instructions, ok := payload["instructions"].(string)
	if !ok || strings.TrimSpace(instructions) == "" {
		t.Fatalf("expected non-empty instructions, got %#v", payload["instructions"])
	}
	if !strings.Contains(instructions, "## Do") || !strings.Contains(instructions, "## Do Not") {
		t.Fatalf("unexpected instructions content: %s", instructions)
	}
}

func TestBuildRequestPayload_FromAnthropicFlowDoesNotEmitAssistantInputText(t *testing.T) {
	service := &Service{}
	irRequest, err := decode.AnthropicMessagesToIR(anthropic.MessagesRequest{
		Model:     "gpt-5.4",
		MaxTokens: 256,
		Messages: []anthropic.Message{
			{Role: "user", Content: []any{map[string]any{"type": "text", "text": "hello"}}},
			{Role: "assistant", Content: []any{map[string]any{"type": "text", "text": "I can help"}}},
			{Role: "user", Content: []any{map[string]any{"type": "text", "text": "continue"}}},
		},
	})
	if err != nil {
		t.Fatalf("AnthropicMessagesToIR: %v", err)
	}
	payload, err := service.buildRequestPayload(RequestFromIR(irRequest))
	if err != nil {
		t.Fatalf("build request payload: %v", err)
	}
	input, ok := payload["input"].([]any)
	if !ok {
		t.Fatalf("input payload missing: %#v", payload)
	}
	assistantFound := false
	for _, item := range input {
		entry, ok := item.(map[string]any)
		if !ok || entry["role"] != "assistant" {
			continue
		}
		assistantFound = true
		content, _ := entry["content"].([]any)
		if len(content) == 0 {
			t.Fatalf("assistant content missing: %#v", entry)
		}
		part, _ := content[0].(map[string]any)
		if part["type"] != "output_text" {
			t.Fatalf("assistant content type = %#v", part["type"])
		}
	}
	if !assistantFound {
		encoded, _ := json.Marshal(payload)
		t.Fatalf("assistant entry missing: %s", string(encoded))
	}
}

func containsAll(body string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(body, fragment) {
			return false
		}
	}
	return true
}
