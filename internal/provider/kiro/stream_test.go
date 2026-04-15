package kiro

import (
	"strings"
	"testing"
)

func TestParserStateReconstructsContentThinkingToolAndUsage(t *testing.T) {
	state := &parserState{}
	state.feed([]byte(`{"content":"hello"}{"thinking":"reason"}{"name":"search","toolUseId":"toolu_1","input":{"q":"a"},"stop":false}{"input":{"page":1}}{"stop":true}{"usage":{"inputTokens":5,"outputTokens":7,"totalTokens":12}}`))
	state.finalizeToolCall()
	if got := len(state.textParts); got != 1 {
		t.Fatalf("text parts = %d", got)
	}
	if got := len(state.thinkingParts); got != 1 {
		t.Fatalf("thinking parts = %d", got)
	}
	if got := len(state.toolCalls); got != 1 {
		t.Fatalf("tool calls = %d", got)
	}
	if state.usage.TotalTokens != 12 {
		t.Fatalf("usage total = %d", state.usage.TotalTokens)
	}
}

func TestParserStateDedupesDuplicateToolCalls(t *testing.T) {
	state := &parserState{}
	state.feed([]byte(`{"name":"search","toolUseId":"toolu_1","input":{"q":"a"},"stop":true}{"name":"search","toolUseId":"toolu_2","input":{"q":"a"},"stop":true}`))
	state.finalizeToolCall()
	if got := len(state.toolCalls); got != 1 {
		t.Fatalf("tool calls = %d", got)
	}
}

func TestCollectCompletionParsesThinkingTagsFromContent(t *testing.T) {
	reader := strings.NewReader(buildEventStreamPayload(`{"content":"<thinking>reason</thinking>done"}`))
	outcome, err := collectCompletion(reader, "claude-sonnet-4.5")
	if err != nil {
		t.Fatalf("collectCompletion error: %v", err)
	}
	if outcome.Thinking != "reason" {
		t.Fatalf("thinking = %q", outcome.Thinking)
	}
	if outcome.Text != "done" {
		t.Fatalf("text = %q", outcome.Text)
	}
}

func buildEventStreamPayload(jsonEvents ...string) string {
	frames := make([]string, 0, len(jsonEvents))
	for _, event := range jsonEvents {
		frames = append(frames, string(buildAWSEventFrame([]byte(event))))
	}
	return strings.Join(frames, "")
}

func buildAWSEventFrame(payload []byte) []byte {
	headers := buildAWSEventHeaders(map[string]string{":message-type": "event"})
	totalLen := uint32(len(headers) + len(payload) + 16)
	buf := make([]byte, totalLen)
	putUint32(buf[0:4], totalLen)
	putUint32(buf[4:8], uint32(len(headers)))
	copy(buf[12:], headers)
	copy(buf[12+len(headers):], payload)
	return buf
}

func buildAWSEventHeaders(values map[string]string) []byte {
	buf := make([]byte, 0)
	for key, value := range values {
		buf = append(buf, byte(len(key)))
		buf = append(buf, []byte(key)...)
		buf = append(buf, byte(7))
		buf = append(buf, byte(len(value)>>8), byte(len(value)))
		buf = append(buf, []byte(value)...)
	}
	return buf
}

func putUint32(dst []byte, value uint32) {
	dst[0] = byte(value >> 24)
	dst[1] = byte(value >> 16)
	dst[2] = byte(value >> 8)
	dst[3] = byte(value)
}
