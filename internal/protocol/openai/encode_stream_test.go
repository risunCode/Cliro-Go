package openai

import (
	"testing"

	contract "cliro-go/internal/contract"
)

func TestIRToChat_UsesReasoningContentField(t *testing.T) {
	response := IRToChat(contract.Response{
		ID:       "chat_123",
		Model:    "gpt-5.4",
		Text:     "final answer",
		Thinking: "plan first",
	})

	message := response.Choices[0].Message
	if message.ReasoningContent != "plan first" {
		t.Fatalf("reasoning_content = %q", message.ReasoningContent)
	}
	if message.Content != "final answer" {
		t.Fatalf("content = %#v", message.Content)
	}
}

func TestIRToCompletions_UsesReasoningContentField(t *testing.T) {
	response := IRToCompletions(contract.Response{
		ID:       "cmpl_123",
		Model:    "gpt-5.4",
		Text:     "final answer",
		Thinking: "plan first",
	})

	choice := response.Choices[0]
	if choice.ReasoningContent != "plan first" {
		t.Fatalf("reasoning_content = %q", choice.ReasoningContent)
	}
	if choice.Text != "final answer" {
		t.Fatalf("text = %q", choice.Text)
	}
}

func TestIRToResponses_UsesReasoningContentField(t *testing.T) {
	response := IRToResponses(contract.Response{
		ID:       "resp_123",
		Model:    "gpt-5.4",
		Text:     "final answer",
		Thinking: "plan first",
	})

	if len(response.Output) != 1 {
		t.Fatalf("output count = %d", len(response.Output))
	}
	part := response.Output[0].Content[0]
	if part.ReasoningContent != "plan first" {
		t.Fatalf("reasoning_content = %q", part.ReasoningContent)
	}
	if part.Text != "final answer" {
		t.Fatalf("text = %q", part.Text)
	}
}

func TestIRStreamToChunk_UsesReasoningContentField(t *testing.T) {
	chunk := IRStreamToChunk("chat_123", "gpt-5.4", contract.Event{ThinkDelta: "step 1"})

	if got := chunk.Choices[0].Delta["reasoning_content"]; got != "step 1" {
		t.Fatalf("reasoning_content delta = %#v", got)
	}
	if _, exists := chunk.Choices[0].Delta["reasoning"]; exists {
		t.Fatalf("unexpected legacy reasoning field: %#v", chunk.Choices[0].Delta)
	}
}

func TestIRStreamToChunk_UsesContentFieldForText(t *testing.T) {
	chunk := IRStreamToChunk("chat_123", "gpt-5.4", contract.Event{TextDelta: "hello"})

	if got := chunk.Choices[0].Delta["content"]; got != "hello" {
		t.Fatalf("content delta = %#v", got)
	}
	if _, exists := chunk.Choices[0].Delta["reasoning_content"]; exists {
		t.Fatalf("unexpected reasoning_content field: %#v", chunk.Choices[0].Delta)
	}
}
