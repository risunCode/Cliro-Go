package decode

import (
	"testing"

	"cliro-go/internal/adapter/ir"
	"cliro-go/internal/protocol/openai"
)

func TestOpenAIResponsesToIR_PreservesConversationMetadata(t *testing.T) {
	request, err := OpenAIResponsesToIR(openai.ResponsesRequest{
		Model: "gpt-5.4",
		Input: []any{map[string]any{
			"type":              "message",
			"role":              "user",
			"content":           []any{map[string]any{"type": "input_text", "text": "hello"}},
			"additional_kwargs": map[string]any{"conversationId": "conv-1", "continuationId": "cont-1"},
		}},
	})
	if err != nil {
		t.Fatalf("OpenAIResponsesToIR: %v", err)
	}
	if request.Metadata["conversationId"] != "conv-1" {
		t.Fatalf("conversationId = %#v", request.Metadata["conversationId"])
	}
	if request.Metadata["continuationId"] != "cont-1" {
		t.Fatalf("continuationId = %#v", request.Metadata["continuationId"])
	}
}

func TestOpenAIResponsesToIR_ParsesAssistantOutputAndToolResult(t *testing.T) {
	request, err := OpenAIResponsesToIR(openai.ResponsesRequest{
		Model: "gpt-5.4",
		Input: []any{
			map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "hello"}}},
			map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "done"},
		},
	})
	if err != nil {
		t.Fatalf("OpenAIResponsesToIR: %v", err)
	}
	if len(request.Messages) != 2 {
		t.Fatalf("message count = %d", len(request.Messages))
	}
	if request.Messages[0].Role != ir.RoleAssistant || request.Messages[0].Content != "hello" {
		t.Fatalf("unexpected assistant message: %+v", request.Messages[0])
	}
	if request.Messages[1].Role != ir.RoleTool || request.Messages[1].ToolCallID != "call_1" || request.Messages[1].Content != "done" {
		t.Fatalf("unexpected tool result message: %+v", request.Messages[1])
	}
}
