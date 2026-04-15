package codex

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
	Reasoning   map[string]any `json:"reasoning,omitempty"`
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
	Reasoning          map[string]any `json:"reasoning,omitempty"`
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

type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   Usage        `json:"usage"`
}

type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type ChatMessage struct {
	Role             string         `json:"role"`
	Content          any            `json:"content"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        any            `json:"tool_calls,omitempty"`
	AdditionalKwargs map[string]any `json:"additional_kwargs,omitempty"`
}

type CompletionsResponse struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []CompletionsChoice `json:"choices"`
	Usage   Usage               `json:"usage"`
}

type CompletionsChoice struct {
	Index            int    `json:"index"`
	Text             string `json:"text"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	FinishReason     string `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ResponsesResponse struct {
	ID         string                `json:"id"`
	Object     string                `json:"object"`
	CreatedAt  int64                 `json:"created_at"`
	Status     string                `json:"status"`
	Model      string                `json:"model"`
	Output     []ResponsesOutputItem `json:"output,omitempty"`
	OutputText string                `json:"output_text,omitempty"`
	Usage      ResponsesUsage        `json:"usage"`
}

type ResponsesOutputItem struct {
	ID        string                 `json:"id,omitempty"`
	Type      string                 `json:"type"`
	Role      string                 `json:"role,omitempty"`
	Status    string                 `json:"status,omitempty"`
	Content   []ResponsesContentPart `json:"content,omitempty"`
	CallID    string                 `json:"call_id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Arguments string                 `json:"arguments,omitempty"`
}

type ResponsesContentPart struct {
	Type             string `json:"type"`
	Text             string `json:"text,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	Annotations      []any  `json:"annotations,omitempty"`
}

type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
