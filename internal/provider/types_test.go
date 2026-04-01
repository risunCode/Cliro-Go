package provider

import (
	"testing"

	contract "cliro-go/internal/contract"
)

func TestChatRequestThinkingFieldsAreAvailable(t *testing.T) {
	request := ChatRequest{
		Model: "claude-sonnet-4.5",
		Thinking: contract.ThinkingConfig{
			Requested: true,
			Mode:      contract.ThinkingModeForce,
		},
		Messages: []Message{{
			Role:    "assistant",
			Content: "hello",
			ThinkingBlocks: []ThinkingBlock{{
				Thinking:  "plan",
				Signature: "sig_plan",
			}},
		}},
	}

	if !request.Thinking.Requested {
		t.Fatal("Thinking.Requested = false")
	}
	if request.Thinking.Mode != contract.ThinkingModeForce {
		t.Fatalf("Thinking.Mode = %q", request.Thinking.Mode)
	}
	if len(request.Messages) != 1 || len(request.Messages[0].ThinkingBlocks) != 1 {
		t.Fatalf("ThinkingBlocks = %#v", request.Messages)
	}
	if request.Messages[0].ThinkingBlocks[0] != (ThinkingBlock{Thinking: "plan", Signature: "sig_plan"}) {
		t.Fatalf("unexpected thinking block: %#v", request.Messages[0].ThinkingBlocks[0])
	}
}
