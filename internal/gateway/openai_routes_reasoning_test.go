package gateway

import (
	"net/http/httptest"
	"strings"
	"testing"

	contract "cliro-go/internal/contract"
)

func TestStreamOpenAICompletions_EmitsReasoningContentBeforeText(t *testing.T) {
	server := &Server{}
	recorder := httptest.NewRecorder()

	server.streamOpenAICompletions(recorder, "gpt-5.4", contract.Response{
		ID:       "cmpl_123",
		Model:    "gpt-5.4",
		Thinking: "plan first",
		Text:     "final answer",
	})

	body := recorder.Body.String()
	if !strings.Contains(body, `"reasoning_content":"plan first"`) {
		t.Fatalf("expected reasoning_content in stream: %s", body)
	}
	if !strings.Contains(body, `"text":"final answer"`) {
		t.Fatalf("expected text in stream: %s", body)
	}
	if strings.Index(body, `"reasoning_content":"plan first"`) > strings.Index(body, `"text":"final answer"`) {
		t.Fatalf("expected reasoning_content before text: %s", body)
	}
}

func TestStreamOpenAIResponses_EmitsReasoningContentBeforeText(t *testing.T) {
	server := &Server{}
	recorder := httptest.NewRecorder()

	server.streamOpenAIResponses(recorder, "gpt-5.4", contract.Response{
		ID:       "resp_123",
		Model:    "gpt-5.4",
		Thinking: "plan first",
		Text:     "final answer",
	})

	body := recorder.Body.String()
	if !strings.Contains(body, `"reasoning_content":"plan first"`) {
		t.Fatalf("expected reasoning_content in stream: %s", body)
	}
	if !strings.Contains(body, `"delta":"final answer"`) {
		t.Fatalf("expected text delta in stream: %s", body)
	}
	if strings.Index(body, `"reasoning_content":"plan first"`) > strings.Index(body, `"delta":"final answer"`) {
		t.Fatalf("expected reasoning_content before text delta: %s", body)
	}
}
