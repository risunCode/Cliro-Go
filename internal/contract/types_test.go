package contract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestContractStructuresRoundTripJSON(t *testing.T) {
	request := Request{
		Protocol: ProtocolAnthropic,
		Endpoint: EndpointAnthropicMessages,
		Model:    "claude-sonnet-4.5",
		Thinking: ThinkingConfig{
			Requested: true,
			Mode:      ThinkingModeAuto,
		},
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
		Metadata: map[string]any{"trace": "abc123"},
	}
	encodedRequest, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var decodedRequest Request
	if err := json.Unmarshal(encodedRequest, &decodedRequest); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if !reflect.DeepEqual(decodedRequest, request) {
		t.Fatalf("request mismatch: %#v", decodedRequest)
	}

	message := Message{
		Role:    RoleAssistant,
		Content: "answer",
		ToolCalls: []ToolCall{{
			ID:        "toolu_1",
			Name:      "Read",
			Arguments: `{"path":"main.go"}`,
		}},
		ThinkingBlocks: []ThinkingBlock{{Thinking: "plan", Signature: "sig_plan"}},
	}
	encodedMessage, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	var decodedMessage Message
	if err := json.Unmarshal(encodedMessage, &decodedMessage); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	if !reflect.DeepEqual(decodedMessage, message) {
		t.Fatalf("message mismatch: %#v", decodedMessage)
	}

	response := Response{
		ID:                "msg_test",
		Model:             "claude-sonnet-4.5",
		Text:              "done",
		Thinking:          "reasoning",
		ThinkingSignature: "sig_reasoning",
		ToolCalls:         message.ToolCalls,
		Usage:             Usage{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8, InputTokens: 3, OutputTokens: 5},
		StopReason:        "tool_calls",
	}
	encodedResponse, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var decodedResponse Response
	if err := json.Unmarshal(encodedResponse, &decodedResponse); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !reflect.DeepEqual(decodedResponse, response) {
		t.Fatalf("response mismatch: %#v", decodedResponse)
	}
}
