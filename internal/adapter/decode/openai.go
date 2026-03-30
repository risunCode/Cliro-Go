package decode

import (
	"encoding/json"
	"strings"

	"cliro-go/internal/adapter/ir"
	"cliro-go/internal/protocol/openai"

	"github.com/google/uuid"
)

func OpenAIResponsesToIR(req openai.ResponsesRequest) (ir.Request, error) {
	if err := validateModel(req.Model); err != nil {
		return ir.Request{}, err
	}

	metadata := make(map[string]any)
	mergeMetadata(metadata, req.Metadata)
	if strings.TrimSpace(req.PreviousResponseID) != "" {
		metadata["previousResponseID"] = strings.TrimSpace(req.PreviousResponseID)
	}
	if req.ParallelToolCalls != nil {
		metadata["parallelToolCalls"] = *req.ParallelToolCalls
	}
	if req.Store != nil {
		metadata["store"] = *req.Store
	}

	messages := responseInputToIRMessages(req.Input, metadata)
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		messages = append([]ir.Message{{Role: ir.RoleSystem, Content: instructions}}, messages...)
	}
	if len(messages) == 0 {
		messages = []ir.Message{{Role: ir.RoleUser, Content: req.Input}}
	}

	tools := make([]ir.Tool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		tools = append(tools, ir.Tool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Schema:      tool.Function.Parameters,
		})
	}

	return ir.Request{
		Protocol:    ir.ProtocolOpenAI,
		Endpoint:    ir.EndpointOpenAIResponses,
		Model:       req.Model,
		Messages:    messages,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxOutputTokens,
		Tools:       tools,
		ToolChoice:  req.ToolChoice,
		User:        req.User,
		Metadata:    metadata,
	}, nil
}

func OpenAIChatToIR(req openai.ChatRequest) (ir.Request, error) {
	if err := validateModel(req.Model); err != nil {
		return ir.Request{}, err
	}

	messages := make([]ir.Message, 0, len(req.Messages))
	for _, msg := range req.Messages {
		toolCalls := make([]ir.ToolCall, 0, len(msg.ToolCalls))
		for _, toolCall := range msg.ToolCalls {
			toolCalls = append(toolCalls, ir.ToolCall{
				ID:        toolCall.ID,
				Name:      toolCall.Function.Name,
				Arguments: toolCall.Function.Arguments,
			})
		}
		messages = append(messages, ir.Message{
			Role:       roleFromString(msg.Role),
			Content:    msg.Content,
			Name:       msg.Name,
			ToolCalls:  toolCalls,
			ToolCallID: msg.ToolCallID,
		})
	}

	tools := make([]ir.Tool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		tools = append(tools, ir.Tool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Schema:      tool.Function.Parameters,
		})
	}

	return ir.Request{
		Protocol:    ir.ProtocolOpenAI,
		Endpoint:    ir.EndpointOpenAIChat,
		Model:       req.Model,
		Messages:    messages,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Tools:       tools,
		ToolChoice:  req.ToolChoice,
		User:        req.User,
		Metadata:    metadataFromChatRequest(req),
	}, nil
}

func OpenAICompletionsToIR(req openai.CompletionsRequest) (ir.Request, error) {
	if err := validateModel(req.Model); err != nil {
		return ir.Request{}, err
	}

	messages := []ir.Message{{
		Role:    ir.RoleUser,
		Content: req.Prompt,
	}}

	return ir.Request{
		Protocol:    ir.ProtocolOpenAI,
		Endpoint:    ir.EndpointOpenAICompletions,
		Model:       req.Model,
		Messages:    messages,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		User:        req.User,
		Metadata:    map[string]any{},
	}, nil
}

func responseInputToIRMessages(input any, metadata map[string]any) []ir.Message {
	switch typed := input.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []ir.Message{{Role: ir.RoleUser, Content: typed}}
	case []any:
		messages := make([]ir.Message, 0, len(typed))
		for _, item := range typed {
			messages = append(messages, responsesInputItemToIRMessages(item, metadata)...)
		}
		return messages
	case map[string]any:
		return responsesInputItemToIRMessages(typed, metadata)
	default:
		return []ir.Message{{Role: ir.RoleUser, Content: typed}}
	}
}

func responsesInputItemToIRMessages(item any, metadata map[string]any) []ir.Message {
	object, ok := item.(map[string]any)
	if !ok {
		return responseInputToIRMessages(item, metadata)
	}

	if additional, ok := object["additional_kwargs"].(map[string]any); ok {
		captureAdditionalKwargsMetadata(additional, metadata)
	}

	itemType := strings.ToLower(strings.TrimSpace(asString(object["type"])))
	if itemType == "" && strings.TrimSpace(asString(object["role"])) != "" {
		itemType = "message"
	}

	switch itemType {
	case "message":
		role := roleFromString(strings.TrimSpace(asString(object["role"])))
		if role == "" {
			role = ir.RoleUser
		}
		content := normalizeResponsesMessageContent(object["content"], role)
		if strings.TrimSpace(content) == "" {
			return nil
		}
		return []ir.Message{{Role: role, Content: content}}
	case "function_call", "custom_tool_call":
		callID := strings.TrimSpace(asString(object["call_id"]))
		if callID == "" {
			callID = "toolu_" + uuid.NewString()[:8]
		}
		name := strings.TrimSpace(asString(object["name"]))
		arguments := strings.TrimSpace(asString(object["arguments"]))
		if itemType == "custom_tool_call" {
			arguments = strings.TrimSpace(asString(object["input"]))
		}
		if arguments == "" {
			if encoded, err := json.Marshal(object["input"]); err == nil && string(encoded) != "null" {
				arguments = string(encoded)
			}
		}
		return []ir.Message{{
			Role: ir.RoleAssistant,
			ToolCalls: []ir.ToolCall{{
				ID:        callID,
				Name:      name,
				Arguments: firstNonEmptyString(arguments, "{}"),
			}},
		}}
	case "function_call_output", "custom_tool_call_output":
		toolCallID := strings.TrimSpace(asString(object["call_id"]))
		if toolCallID == "" {
			return nil
		}
		return []ir.Message{{Role: ir.RoleTool, ToolCallID: toolCallID, Content: normalizeResponsesToolOutput(object["output"])}}
	case "input_text":
		text := strings.TrimSpace(asString(object["text"]))
		if text == "" {
			return nil
		}
		return []ir.Message{{Role: ir.RoleUser, Content: text}}
	case "output_text", "text", "refusal":
		text := strings.TrimSpace(asString(object["text"]))
		if text == "" {
			text = strings.TrimSpace(asString(object["refusal"]))
		}
		if text == "" {
			return nil
		}
		return []ir.Message{{Role: ir.RoleAssistant, Content: text}}
	default:
		content := normalizeResponsesMessageContent(object["content"], roleFromString(strings.TrimSpace(asString(object["role"]))))
		if strings.TrimSpace(content) == "" {
			content = strings.TrimSpace(asString(object["text"]))
		}
		if content == "" {
			encoded, _ := json.Marshal(object)
			content = strings.TrimSpace(string(encoded))
		}
		return []ir.Message{{Role: ir.RoleUser, Content: content}}
	}
}

func normalizeResponsesMessageContent(content any, role ir.Role) string {
	switch typed := content.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			partObject, ok := item.(map[string]any)
			if !ok {
				if text := strings.TrimSpace(asString(item)); text != "" {
					parts = append(parts, text)
				}
				continue
			}
			partType := strings.ToLower(strings.TrimSpace(asString(partObject["type"])))
			switch partType {
			case "input_text", "output_text", "text":
				if text := strings.TrimSpace(asString(partObject["text"])); text != "" {
					parts = append(parts, text)
				}
			case "refusal":
				if text := strings.TrimSpace(asString(partObject["refusal"])); text != "" {
					parts = append(parts, text)
				}
			default:
				if text := strings.TrimSpace(asString(partObject["text"])); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	}
}

func normalizeResponsesToolOutput(output any) string {
	switch typed := output.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	}
}

func metadataFromChatRequest(req openai.ChatRequest) map[string]any {
	metadata := make(map[string]any)
	mergeMetadata(metadata, req.Metadata)
	for _, message := range req.Messages {
		captureAdditionalKwargsMetadata(message.AdditionalKwargs, metadata)
	}
	return metadata
}

func captureAdditionalKwargsMetadata(additional map[string]any, metadata map[string]any) {
	if len(additional) == 0 || metadata == nil {
		return
	}
	if conversationID, ok := additional["conversationId"].(string); ok && strings.TrimSpace(conversationID) != "" {
		metadata["conversationId"] = strings.TrimSpace(conversationID)
	}
	if continuationID, ok := additional["continuationId"].(string); ok && strings.TrimSpace(continuationID) != "" {
		metadata["continuationId"] = strings.TrimSpace(continuationID)
	}
	if profileARN, ok := additional["profileArn"].(string); ok && strings.TrimSpace(profileARN) != "" {
		metadata["profileArn"] = strings.TrimSpace(profileARN)
	}
}

func mergeMetadata(dst map[string]any, src map[string]any) {
	if dst == nil || len(src) == 0 {
		return
	}
	for key, value := range src {
		dst[key] = value
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	}
}
