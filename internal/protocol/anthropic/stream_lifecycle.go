package anthropic

import contract "cliro-go/internal/contract"

type StreamEventEmitter func(eventName string, payload map[string]any)

type ThinkingBlockLifecycle struct {
	emit             StreamEventEmitter
	nextIndex        int
	thinkingOpen     bool
	thinkingIndex    int
	signatureEmitted bool
}

func NewThinkingBlockLifecycle(nextIndex int, emit StreamEventEmitter) *ThinkingBlockLifecycle {
	if emit == nil {
		emit = func(string, map[string]any) {}
	}
	return &ThinkingBlockLifecycle{
		emit:      emit,
		nextIndex: nextIndex,
	}
}

func (l *ThinkingBlockLifecycle) EmitThinkingDelta(delta string) {
	if delta == "" {
		return
	}
	l.ensureThinkingBlock()
	event := IRStreamToEvent(contract.Event{ThinkDelta: delta})
	event["index"] = l.thinkingIndex
	l.emit(event["type"].(string), event)
}

func (l *ThinkingBlockLifecycle) EmitSignature(signature string) {
	if signature == "" || !l.thinkingOpen || l.signatureEmitted {
		return
	}
	event := IRStreamToEvent(contract.Event{SignatureDelta: signature})
	event["index"] = l.thinkingIndex
	l.emit(event["type"].(string), event)
	l.signatureEmitted = true
}

func (l *ThinkingBlockLifecycle) Close(signature string) {
	if !l.thinkingOpen {
		return
	}
	l.EmitSignature(signature)
	l.emit("content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": l.thinkingIndex,
	})
	l.thinkingOpen = false
}

func (l *ThinkingBlockLifecycle) PrepareForNextBlock(signature string) int {
	l.Close(signature)
	return l.nextIndex
}

func (l *ThinkingBlockLifecycle) ensureThinkingBlock() {
	if l.thinkingOpen {
		return
	}
	l.thinkingIndex = l.nextIndex
	l.nextIndex++
	l.thinkingOpen = true
	l.signatureEmitted = false
	l.emit("content_block_start", map[string]any{
		"type":  "content_block_start",
		"index": l.thinkingIndex,
		"content_block": map[string]any{
			"type":      "thinking",
			"thinking":  "",
			"signature": "",
		},
	})
}
