package anthropic

import "testing"

type recordedStreamEvent struct {
	name    string
	payload map[string]any
}

func TestThinkingBlockLifecycle_EmitsOrderedEventsAndSingleSignature(t *testing.T) {
	var events []recordedStreamEvent
	helper := NewThinkingBlockLifecycle(0, func(name string, payload map[string]any) {
		events = append(events, recordedStreamEvent{name: name, payload: payload})
	})

	helper.EmitThinkingDelta("plan first")
	helper.EmitSignature("sig_test")
	nextIndex := helper.PrepareForNextBlock("sig_test")

	if nextIndex != 1 {
		t.Fatalf("next index = %d, want 1", nextIndex)
	}
	if len(events) != 4 {
		t.Fatalf("event count = %d, want 4", len(events))
	}

	assertRecordedEventName(t, events[0], "content_block_start")
	assertRecordedEventName(t, events[1], "content_block_delta")
	assertRecordedEventName(t, events[2], "content_block_delta")
	assertRecordedEventName(t, events[3], "content_block_stop")

	if got := events[0].payload["index"]; got != 0 {
		t.Fatalf("start index = %v, want 0", got)
	}
	contentBlock, ok := events[0].payload["content_block"].(map[string]any)
	if !ok {
		t.Fatalf("start content_block type = %T, want map[string]any", events[0].payload["content_block"])
	}
	if got := contentBlock["type"]; got != "thinking" {
		t.Fatalf("start content block type = %v, want thinking", got)
	}

	firstDelta, ok := events[1].payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("thinking delta type = %T, want map[string]any", events[1].payload["delta"])
	}
	if got := firstDelta["type"]; got != "thinking_delta" {
		t.Fatalf("delta type = %v, want thinking_delta", got)
	}
	if got := firstDelta["thinking"]; got != "plan first" {
		t.Fatalf("thinking delta = %v, want plan first", got)
	}

	signatureDelta, ok := events[2].payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("signature delta type = %T, want map[string]any", events[2].payload["delta"])
	}
	if got := signatureDelta["type"]; got != "signature_delta" {
		t.Fatalf("delta type = %v, want signature_delta", got)
	}
	if got := signatureDelta["signature"]; got != "sig_test" {
		t.Fatalf("signature delta = %v, want sig_test", got)
	}

	if got := events[3].payload["index"]; got != 0 {
		t.Fatalf("stop index = %v, want 0", got)
	}
	if signatureCount := countSignatureDeltas(events); signatureCount != 1 {
		t.Fatalf("signature delta count = %d, want 1", signatureCount)
	}
}

func TestThinkingBlockLifecycle_NoThinkingDoesNotEmitEvents(t *testing.T) {
	var events []recordedStreamEvent
	helper := NewThinkingBlockLifecycle(0, func(name string, payload map[string]any) {
		events = append(events, recordedStreamEvent{name: name, payload: payload})
	})

	nextIndex := helper.PrepareForNextBlock("sig_unused")
	helper.Close("sig_unused")
	helper.EmitSignature("sig_unused")

	if nextIndex != 0 {
		t.Fatalf("next index = %d, want 0", nextIndex)
	}
	if len(events) != 0 {
		t.Fatalf("event count = %d, want 0", len(events))
	}
}

func assertRecordedEventName(t *testing.T, event recordedStreamEvent, want string) {
	t.Helper()
	if event.name != want {
		t.Fatalf("event name = %q, want %q", event.name, want)
	}
}

func countSignatureDeltas(events []recordedStreamEvent) int {
	count := 0
	for _, event := range events {
		delta, ok := event.payload["delta"].(map[string]any)
		if !ok {
			continue
		}
		if delta["type"] == "signature_delta" {
			count++
		}
	}
	return count
}
