package provider

import (
	"reflect"
	"testing"

	contract "cliro-go/internal/contract"
)

func TestRequestFromIR(t *testing.T) {
	temperature := 0.7
	topP := 0.9
	maxTokens := 256

	request := contract.Request{
		Endpoint:    contract.EndpointOpenAIChat,
		Model:       "gpt-5.4",
		Thinking:    contract.ThinkingConfig{Requested: true, Mode: contract.ThinkingModeAuto},
		Stream:      true,
		Temperature: &temperature,
		TopP:        &topP,
		MaxTokens:   &maxTokens,
		User:        "user-1",
		ToolChoice:  map[string]any{"type": "function"},
		Metadata:    map[string]any{"conversationId": "conv-1"},
		Messages: []contract.Message{{
			Role:       contract.RoleAssistant,
			Content:    "hello",
			Name:       "assistant-1",
			ToolCallID: "tool-call-id",
			ThinkingBlocks: []contract.ThinkingBlock{{
				Thinking:  "step-by-step",
				Signature: "sig-1",
			}},
			ToolCalls: []contract.ToolCall{{
				ID:        "tool-1",
				Name:      "Read",
				Arguments: `{"path":"README.md"}`,
			}},
		}},
		Tools: []contract.Tool{{
			Name:        "Read",
			Description: "Read file",
			Schema:      map[string]any{"type": "object"},
		}},
	}

	got := RequestFromIR(request)

	if got.RouteFamily != string(contract.EndpointOpenAIChat) {
		t.Fatalf("RouteFamily = %q", got.RouteFamily)
	}
	if got.Model != request.Model || !got.Stream {
		t.Fatalf("unexpected request metadata: %#v", got)
	}
	if !reflect.DeepEqual(got.Thinking, request.Thinking) {
		t.Fatalf("Thinking = %#v", got.Thinking)
	}
	if got.Temperature != &temperature || got.TopP != &topP || got.MaxTokens != &maxTokens {
		t.Fatalf("unexpected scalar pointers: %#v", got)
	}
	if got.User != request.User {
		t.Fatalf("User = %q", got.User)
	}
	if !reflect.DeepEqual(got.ToolChoice, request.ToolChoice) {
		t.Fatalf("ToolChoice = %#v", got.ToolChoice)
	}
	if !reflect.DeepEqual(got.Metadata, request.Metadata) {
		t.Fatalf("Metadata = %#v", got.Metadata)
	}

	if len(got.Messages) != 1 {
		t.Fatalf("Messages len = %d", len(got.Messages))
	}
	message := got.Messages[0]
	if message.Role != string(contract.RoleAssistant) || message.Content != "hello" || message.Name != "assistant-1" || message.ToolCallID != "tool-call-id" {
		t.Fatalf("unexpected message: %#v", message)
	}
	if !reflect.DeepEqual(message.ThinkingBlocks, []ThinkingBlock{{Thinking: "step-by-step", Signature: "sig-1"}}) {
		t.Fatalf("unexpected thinking blocks: %#v", message.ThinkingBlocks)
	}
	if len(message.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d", len(message.ToolCalls))
	}
	if message.ToolCalls[0].Function.Name != "Read" || message.ToolCalls[0].Function.Arguments != `{"path":"README.md"}` {
		t.Fatalf("unexpected tool call: %#v", message.ToolCalls[0])
	}

	if len(got.Tools) != 1 {
		t.Fatalf("Tools len = %d", len(got.Tools))
	}
	if got.Tools[0].Function.Name != "Read" || got.Tools[0].Function.Description != "Read file" {
		t.Fatalf("unexpected tool: %#v", got.Tools[0])
	}
	if !reflect.DeepEqual(got.Tools[0].Function.Parameters, map[string]any{"type": "object"}) {
		t.Fatalf("unexpected tool parameters: %#v", got.Tools[0].Function.Parameters)
	}
}

func TestRequestFromIRLeavesThinkingAbsentWhenNotProvided(t *testing.T) {
	request := contract.Request{
		Endpoint: contract.EndpointAnthropicMessages,
		Model:    "claude-sonnet-4.5",
		Messages: []contract.Message{{
			Role:    contract.RoleUser,
			Content: "hello",
		}},
	}

	got := RequestFromIR(request)

	if got.Thinking != (contract.ThinkingConfig{}) {
		t.Fatalf("Thinking = %#v", got.Thinking)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("Messages len = %d", len(got.Messages))
	}
	if got.Messages[0].ThinkingBlocks != nil {
		t.Fatalf("ThinkingBlocks = %#v", got.Messages[0].ThinkingBlocks)
	}
}
