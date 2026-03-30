package encode

import (
	"encoding/json"
	"strings"
	"time"

	"cliro-go/internal/adapter/ir"
	"cliro-go/internal/protocol/openai"

	"github.com/google/uuid"
)

func IRToOpenAIChat(resp ir.Response) openai.ChatResponse {
	message := openai.ChatMessage{Role: "assistant", Content: resp.Text}
	if resp.Thinking != "" {
		message.ReasoningContent = resp.Thinking
	}
	finishReason := firstNonEmpty(resp.StopReason, "stop")
	if len(resp.ToolCalls) > 0 {
		message.ToolCalls = irToolCallsToOpenAIToolCalls(resp.ToolCalls)
		message.Content = nil
		if finishReason == "stop" {
			finishReason = "tool_calls"
		}
	}

	return openai.ChatResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: []openai.ChatChoice{{
			Index:        0,
			Message:      message,
			FinishReason: finishReason,
		}},
		Usage: openai.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
}

func IRToOpenAICompletions(resp ir.Response) openai.CompletionsResponse {
	return openai.CompletionsResponse{
		ID:      resp.ID,
		Object:  "text_completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: []openai.CompletionsChoice{{
			Index:        0,
			Text:         resp.Text,
			FinishReason: firstNonEmpty(resp.StopReason, "stop"),
		}},
		Usage: openai.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
}

func IRToOpenAIResponses(resp ir.Response) openai.ResponsesResponse {
	output := make([]openai.ResponsesOutputItem, 0, 1+len(resp.ToolCalls))
	if strings.TrimSpace(resp.Text) != "" || len(resp.ToolCalls) == 0 {
		output = append(output, openai.ResponsesOutputItem{
			ID:     resp.ID,
			Type:   "message",
			Role:   "assistant",
			Status: "completed",
			Content: []openai.ResponsesContentPart{{
				Type:        "output_text",
				Text:        resp.Text,
				Annotations: []any{},
			}},
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
		output = append(output, openai.ResponsesOutputItem{
			ID:        "fc_" + id,
			Type:      "function_call",
			Status:    "completed",
			CallID:    id,
			Name:      name,
			Arguments: arguments,
		})
	}

	return openai.ResponsesResponse{
		ID:         firstNonEmpty(resp.ID, "resp_"+uuid.NewString()[:8]),
		Object:     "response",
		CreatedAt:  time.Now().Unix(),
		Status:     "completed",
		Model:      resp.Model,
		Output:     output,
		OutputText: resp.Text,
		Usage: openai.ResponsesUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
	}
}

func irToolCallsToOpenAIToolCalls(calls []ir.ToolCall) []map[string]any {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
