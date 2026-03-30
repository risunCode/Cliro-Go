package openai

import (
	"encoding/json"
	"io"
)

type ChatRequest struct {
	Model       string         `json:"model"`
	Messages    []Message      `json:"messages"`
	Stream      bool           `json:"stream"`
	Temperature *float64       `json:"temperature,omitempty"`
	TopP        *float64       `json:"top_p,omitempty"`
	MaxTokens   *int           `json:"max_tokens,omitempty"`
	User        string         `json:"user,omitempty"`
	Tools       []Tool         `json:"tools,omitempty"`
	ToolChoice  any            `json:"tool_choice,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type Message struct {
	Role             string         `json:"role"`
	Content          any            `json:"content"`
	Name             string         `json:"name,omitempty"`
	ToolCalls        []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	AdditionalKwargs map[string]any `json:"additional_kwargs,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type CompletionsRequest struct {
	Model       string   `json:"model"`
	Prompt      any      `json:"prompt"`
	Stream      bool     `json:"stream"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	User        string   `json:"user,omitempty"`
}

type ResponsesRequest struct {
	Model              string         `json:"model"`
	Input              any            `json:"input"`
	Instructions       string         `json:"instructions,omitempty"`
	Stream             bool           `json:"stream,omitempty"`
	Temperature        *float64       `json:"temperature,omitempty"`
	TopP               *float64       `json:"top_p,omitempty"`
	MaxOutputTokens    *int           `json:"max_output_tokens,omitempty"`
	User               string         `json:"user,omitempty"`
	Tools              []Tool         `json:"tools,omitempty"`
	ToolChoice         any            `json:"tool_choice,omitempty"`
	PreviousResponseID string         `json:"previous_response_id,omitempty"`
	ParallelToolCalls  *bool          `json:"parallel_tool_calls,omitempty"`
	Store              *bool          `json:"store,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}

func DecodeChatRequest(r io.Reader) (ChatRequest, error) {
	var req ChatRequest
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return ChatRequest{}, err
	}
	return req, nil
}

func DecodeCompletionsRequest(r io.Reader) (CompletionsRequest, error) {
	var req CompletionsRequest
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return CompletionsRequest{}, err
	}
	return req, nil
}

func DecodeResponsesRequest(r io.Reader) (ResponsesRequest, error) {
	var req ResponsesRequest
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return ResponsesRequest{}, err
	}
	return req, nil
}
