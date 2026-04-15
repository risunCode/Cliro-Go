package anthropic

import (
	"encoding/json"
	"io"
)

type MessagesRequest struct {
	Model       string         `json:"model"`
	Messages    []Message      `json:"messages"`
	System      any            `json:"system,omitempty"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	Temperature *float64       `json:"temperature,omitempty"`
	TopP        *float64       `json:"top_p,omitempty"`
	Stream      bool           `json:"stream,omitempty"`
	User        string         `json:"user,omitempty"`
	Tools       []Tool         `json:"tools,omitempty"`
	ToolChoice  any            `json:"tool_choice,omitempty"`
	Thinking    map[string]any `json:"thinking,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type Tool struct {
	Type        string `json:"type,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema,omitempty"`
}

type CountTokensRequest = MessagesRequest

func DecodeMessagesRequest(r io.Reader) (MessagesRequest, error) {
	var req MessagesRequest
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return MessagesRequest{}, err
	}
	return req, nil
}

func DecodeCountTokensRequest(r io.Reader) (CountTokensRequest, error) {
	return DecodeMessagesRequest(r)
}

type MessagesResponse struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	Model        string `json:"model"`
	Content      any    `json:"content"`
	StopReason   string `json:"stop_reason"`
	StopSequence any    `json:"stop_sequence"`
	Usage        Usage  `json:"usage"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
