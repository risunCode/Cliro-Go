package provider

import contract "cliro-go/internal/contract"

func RequestFromIR(request contract.Request) ChatRequest {
	messages := make([]Message, 0, len(request.Messages))
	for _, message := range request.Messages {
		toolCalls := make([]ToolCall, 0, len(message.ToolCalls))
		for _, toolCall := range message.ToolCalls {
			toolCalls = append(toolCalls, ToolCall{
				ID:   toolCall.ID,
				Type: "function",
				Function: ToolCallTarget{
					Name:      toolCall.Name,
					Arguments: toolCall.Arguments,
				},
			})
		}

		var thinkingBlocks []ThinkingBlock
		if len(message.ThinkingBlocks) > 0 {
			thinkingBlocks = make([]ThinkingBlock, 0, len(message.ThinkingBlocks))
			for _, thinkingBlock := range message.ThinkingBlocks {
				thinkingBlocks = append(thinkingBlocks, ThinkingBlock{
					Thinking:  thinkingBlock.Thinking,
					Signature: thinkingBlock.Signature,
				})
			}
		}

		messages = append(messages, Message{
			Role:           string(message.Role),
			Content:        message.Content,
			Name:           message.Name,
			ToolCalls:      toolCalls,
			ToolCallID:     message.ToolCallID,
			ThinkingBlocks: thinkingBlocks,
		})
	}

	tools := make([]Tool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Schema,
			},
		})
	}

	return ChatRequest{
		RouteFamily: string(request.Endpoint),
		Model:       request.Model,
		Thinking:    request.Thinking,
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
