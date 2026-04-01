package kiro

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"io"
	"testing"

	provider "cliro-go/internal/provider"
)

func TestStreamParser_ParsesAWSFramesAndToolUses(t *testing.T) {
	body := bytes.NewReader(bytes.Join([][]byte{
		awsEventFrame(t, "assistantResponseEvent", map[string]any{"content": "hel"}),
		awsEventFrame(t, "assistantResponseEvent", map[string]any{"content": "hello"}),
		awsEventFrame(t, "reasoningContentEvent", map[string]any{"text": "plan"}),
		awsEventFrame(t, "reasoningContentEvent", map[string]any{"text": "plan more"}),
		awsEventFrame(t, "toolUseEvent", map[string]any{"toolUseId": "tool_1", "name": "Read", "input": `{"path":"a`}),
		awsEventFrame(t, "toolUseEvent", map[string]any{"toolUseId": "tool_1", "input": `bc"}`}),
		awsEventFrame(t, "toolUseEvent", map[string]any{"toolUseId": "tool_1", "stop": true}),
		awsEventFrame(t, "meteringEvent", map[string]any{"usage": map[string]any{"inputTokens": 4, "outputTokens": 6, "totalTokens": 10}}),
	}, nil))

	parser := NewStreamParser(body)
	text := ""
	thinking := ""
	usage := UsageSnapshot{}
	for {
		event, err := parser.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		text += event.Text
		thinking += event.Thinking
		mergeUsageSnapshot(&usage, event.Usage)
	}

	if text != "hello" {
		t.Fatalf("unexpected text: %q", text)
	}
	if thinking != "plan more" {
		t.Fatalf("unexpected thinking: %q", thinking)
	}
	if usage.PromptTokens != 4 || usage.CompletionTokens != 6 || usage.TotalTokens != 10 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
	toolUses := parser.ToolUses()
	if len(toolUses) != 1 {
		t.Fatalf("unexpected tool uses: %#v", toolUses)
	}
	if toolUses[0].Name != "Read" || toolUses[0].Input["path"] != "abc" {
		t.Fatalf("unexpected tool use payload: %#v", toolUses[0])
	}
}

func TestCollectCompletion_ExtractsBracketToolCalls(t *testing.T) {
	body := bytes.NewReader(bytes.Join([][]byte{
		awsEventFrame(t, "assistantResponseEvent", map[string]any{"content": `[{"id":"tool_1","name":"Read","arguments":{"path":"README.md"}}]`}),
		awsEventFrame(t, "meteringEvent", map[string]any{"usage": map[string]any{"inputTokens": 2, "outputTokens": 3, "totalTokens": 5}}),
	}, nil))

	outcome, err := collectCompletion(body, provider.ChatRequest{Model: "claude-sonnet-4.5"})
	if err != nil {
		t.Fatalf("collectCompletion: %v", err)
	}
	if outcome.Text != "" {
		t.Fatalf("expected bracket tool payload to be extracted from text, got %q", outcome.Text)
	}
	if len(outcome.ToolUses) != 1 {
		t.Fatalf("unexpected tool uses: %#v", outcome.ToolUses)
	}
	if outcome.ToolUses[0].ID != "tool_1" || outcome.ToolUses[0].Input["path"] != "README.md" {
		t.Fatalf("unexpected extracted tool use: %#v", outcome.ToolUses[0])
	}
	if outcome.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage: %#v", outcome.Usage)
	}
}

func TestCollectCompletion_EstimatesUsageWhenStreamOmitsIt(t *testing.T) {
	body := bytes.NewReader(bytes.Join([][]byte{
		awsEventFrame(t, "reasoningContentEvent", map[string]any{"text": "plan carefully"}),
		awsEventFrame(t, "assistantResponseEvent", map[string]any{"content": "hello world"}),
	}, nil))

	outcome, err := collectCompletion(body, provider.ChatRequest{
		Model:    "claude-sonnet-4.5",
		Messages: []provider.Message{{Role: "user", Content: "please help summarize this project"}},
	})
	if err != nil {
		t.Fatalf("collectCompletion: %v", err)
	}
	if outcome.Usage.PromptTokens <= 0 {
		t.Fatalf("expected estimated prompt tokens, got %#v", outcome.Usage)
	}
	if outcome.Usage.CompletionTokens <= 0 {
		t.Fatalf("expected estimated completion tokens, got %#v", outcome.Usage)
	}
	if outcome.Usage.TotalTokens != outcome.Usage.PromptTokens+outcome.Usage.CompletionTokens {
		t.Fatalf("unexpected estimated total tokens, got %#v", outcome.Usage)
	}
	if outcome.Text != "hello world" {
		t.Fatalf("unexpected text: %q", outcome.Text)
	}
}

func awsEventFrame(t *testing.T, eventType string, payload any) []byte {
	t.Helper()
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	headers := append(awsHeader(":message-type", "event"), awsHeader(":event-type", eventType)...)
	totalLen := 12 + len(headers) + len(payloadBytes) + 4
	prelude := make([]byte, 8)
	binary.BigEndian.PutUint32(prelude[0:4], uint32(totalLen))
	binary.BigEndian.PutUint32(prelude[4:8], uint32(len(headers)))
	preludeCRC := crc32.ChecksumIEEE(prelude)
	frame := make([]byte, 12)
	copy(frame[:8], prelude)
	binary.BigEndian.PutUint32(frame[8:12], preludeCRC)
	frame = append(frame, headers...)
	frame = append(frame, payloadBytes...)
	messageCRC := crc32.ChecksumIEEE(frame)
	trailer := make([]byte, 4)
	binary.BigEndian.PutUint32(trailer, messageCRC)
	return append(frame, trailer...)
}

func awsHeader(name string, value string) []byte {
	encoded := make([]byte, 0, 1+len(name)+1+2+len(value))
	encoded = append(encoded, byte(len(name)))
	encoded = append(encoded, []byte(name)...)
	encoded = append(encoded, byte(7))
	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(value)))
	encoded = append(encoded, length...)
	encoded = append(encoded, []byte(value)...)
	return encoded
}

func mergeUsageSnapshot(target *UsageSnapshot, update UsageSnapshot) {
	if target == nil {
		return
	}
	if update.PromptTokens > 0 {
		target.PromptTokens = update.PromptTokens
	}
	if update.CompletionTokens > 0 {
		target.CompletionTokens = update.CompletionTokens
	}
	if update.TotalTokens > 0 {
		target.TotalTokens = update.TotalTokens
	}
}
