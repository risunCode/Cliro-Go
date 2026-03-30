package codex

import (
	"context"

	"cliro-go/internal/adapter/ir"
	provider "cliro-go/internal/provider"
)

func RequestFromIR(request ir.Request) provider.ChatRequest {
	messages := make([]provider.Message, 0, len(request.Messages))
	for _, message := range request.Messages {
		toolCalls := make([]provider.ToolCall, 0, len(message.ToolCalls))
		for _, toolCall := range message.ToolCalls {
			toolCalls = append(toolCalls, provider.ToolCall{
				ID:   toolCall.ID,
				Type: "function",
				Function: provider.ToolCallTarget{
					Name:      toolCall.Name,
					Arguments: toolCall.Arguments,
				},
			})
		}
		messages = append(messages, provider.Message{
			Role:       string(message.Role),
			Content:    message.Content,
			Name:       message.Name,
			ToolCalls:  toolCalls,
			ToolCallID: message.ToolCallID,
		})
	}

	tools := make([]provider.Tool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, provider.Tool{
			Type: "function",
			Function: provider.ToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Schema,
			},
		})
	}

	return provider.ChatRequest{
		RouteFamily: string(request.Endpoint),
		Model:       request.Model,
		Messages:    messages,
		Stream:      request.Stream,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		MaxTokens:   request.MaxTokens,
		Tools:       tools,
		ToolChoice:  request.ToolChoice,
		User:        request.User,
		Metadata:    request.Metadata,
	}
}

func (s *Service) ExecuteFromIR(ctx context.Context, request ir.Request) (provider.CompletionOutcome, int, string, error) {
	return s.Complete(ctx, RequestFromIR(request))
}

type StreamParser struct{}

func NewStreamParser() *StreamParser {
	return &StreamParser{}
}
