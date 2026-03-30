package anthropic

type StreamEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index,omitempty"`
	Delta any    `json:"delta,omitempty"`
}
