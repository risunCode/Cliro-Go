package openai

import (
	"cliro-go/internal/util"
	"encoding/json"
	"strings"
	"time"

	contract "cliro-go/internal/contract"

	"github.com/google/uuid"
)

func IRToChat(resp contract.Response) ChatResponse {
	message := ChatMessage{Role: "assistant", Content: resp.Text}
	if resp.Thinking != "" {
		message.ReasoningContent = resp.Thinking
	}
	finishReason := util.FirstNonEmpty(resp.StopReason, "stop")
	if len(resp.ToolCalls) > 0 {
		message.ToolCalls = irToolCallsToOpenAIToolCalls(resp.ToolCalls)
		message.Content = nil
		if finishReason == "stop" {
			finishReason = "tool_calls"
		}
	}

	return ChatResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: []ChatChoice{{
			Index:        0,
			Message:      message,
			FinishReason: finishReason,
		}},
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
}

func IRToCompletions(resp contract.Response) CompletionsResponse {
	return CompletionsResponse{
		ID:      resp.ID,
		Object:  "text_completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: []CompletionsChoice{{
			Index:            0,
			Text:             resp.Text,
			ReasoningContent: resp.Thinking,
			FinishReason:     util.FirstNonEmpty(resp.StopReason, "stop"),
		}},
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
}

func IRToResponses(resp contract.Response) ResponsesResponse {
	output := make([]ResponsesOutputItem, 0, 1+len(resp.ToolCalls))
	if strings.TrimSpace(resp.Text) != "" || len(resp.ToolCalls) == 0 {
		output = append(output, ResponsesOutputItem{
			ID:      resp.ID,
			Type:    "message",
			Role:    "assistant",
			Status:  "completed",
			Content: []ResponsesContentPart{responseOutputTextPart(resp.Text, resp.Thinking)},
		})
	}
	for _, call := range resp.ToolCalls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = "fc_" + uuid.NewString()[:8]
		}
		arguments := strings.TrimSpace(call.Arguments)
		if arguments == "" {
			arguments = "{}"
		}
		output = append(output, ResponsesOutputItem{
			ID:        "fc_" + id,
			Type:      "function_call",
			Status:    "completed",
			CallID:    id,
			Name:      name,
			Arguments: arguments,
		})
	}

	return ResponsesResponse{
		ID:         util.FirstNonEmpty(resp.ID, "resp_"+uuid.NewString()[:8]),
		Object:     "response",
		CreatedAt:  time.Now().Unix(),
		Status:     "completed",
		Model:      resp.Model,
		Output:     output,
		OutputText: resp.Text,
		Usage: ResponsesUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
	}
}

func responseOutputTextPart(text string, reasoning string) ResponsesContentPart {
	part := ResponsesContentPart{
		Type:        "output_text",
		Text:        text,
		Annotations: []any{},
	}
	if reasoning != "" {
		part.ReasoningContent = reasoning
	}
	return part
}

func irToolCallsToOpenAIToolCalls(calls []contract.ToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = "toolu_" + uuid.NewString()[:8]
		}
		arguments := strings.TrimSpace(call.Arguments)
		if arguments == "" {
			arguments = "{}"
		}

		if !json.Valid([]byte(arguments)) {
			encoded, _ := json.Marshal(map[string]any{"value": arguments})
			arguments = string(encoded)
		}

		out = append(out, map[string]any{
			"id":   id,
			"type": "function",
			"function": map[string]any{
				"name":      name,
				"arguments": arguments,
			},
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

