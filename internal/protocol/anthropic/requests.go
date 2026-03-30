package anthropic

import (
	"encoding/json"
	"io"
)

type MessagesRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	System      any       `json:"system,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	User        string    `json:"user,omitempty"`
	Tools       []Tool    `json:"tools,omitempty"`
	ToolChoice  any       `json:"tool_choice,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
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
