package codex

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"

	"cliro/internal/config"
	models "cliro/internal/proxy/models"
)

type ChatRequest struct {
	RouteFamily string                  `json:"-"`
	Model       string                  `json:"model"`
	Thinking    models.ThinkingConfig `json:"-"`
	Messages    []Message               `json:"messages"`
	Stream      bool                    `json:"stream"`
	Temperature *float64                `json:"temperature,omitempty"`
	TopP        *float64                `json:"top_p,omitempty"`
	MaxTokens   *int                    `json:"max_tokens,omitempty"`
	User        string                  `json:"user,omitempty"`
	Tools       []Tool                  `json:"tools,omitempty"`
	ToolChoice  any                     `json:"tool_choice,omitempty"`
	Metadata    map[string]any          `json:"-"`
}

type Message struct {
	Role           string          `json:"role"`
	Content        any             `json:"content"`
	Name           string          `json:"name,omitempty"`
	ToolCalls      []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID     string          `json:"tool_call_id,omitempty"`
	ThinkingBlocks []ThinkingBlock `json:"-"`
}

type ThinkingBlock struct {
	Thinking  string `json:"thinking"`
	Signature string `json:"signature,omitempty"`
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
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function ToolCallTarget `json:"function"`
}

type ToolCallTarget struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type AnthropicMessagesRequest struct {
	Model       string             `json:"model"`
	Messages    []AnthropicMessage `json:"messages"`
	System      any                `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
	User        string             `json:"user,omitempty"`
	Tools       []AnthropicTool    `json:"tools,omitempty"`
	ToolChoice  any                `json:"tool_choice,omitempty"`
}

type AnthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type AnthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

type CompletionOutcome struct {
	Text              string
	Thinking          string
	ThinkingSignature string
	ThinkingSource    string
	ToolUses          []ToolUse
	Usage             config.ProxyStats
	ID                string
	Model             string
	Provider          string
	AccountID         string
	AccountLabel      string
}

type ToolUse struct {
	ID    string
	Name  string
	Input map[string]any
}

func RequestFromIR(request models.Request) ChatRequest {
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
		toolType := tool.Type
		if toolType == "" {
			toolType = "function"
		}
		tools = append(tools, Tool{
			Type: toolType,
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

const DefaultToolNameLimit = 64

type ToolNameMapping struct {
	forward map[string]string
	reverse map[string]string
}

func RemapChatRequestToolNames(req ChatRequest, mapping ToolNameMapping) ChatRequest {
	cloned := req
	if len(req.Tools) > 0 {
		cloned.Tools = append([]Tool(nil), req.Tools...)
		for index := range cloned.Tools {
			cloned.Tools[index].Function.Name = mapping.Remap(cloned.Tools[index].Function.Name)
			cloned.Tools[index].Function.Parameters = NormalizeToolSchema(cloned.Tools[index].Function.Parameters)
		}
	}
	if len(req.Messages) > 0 {
		cloned.Messages = append([]Message(nil), req.Messages...)
		for index := range cloned.Messages {
			if len(req.Messages[index].ToolCalls) == 0 {
				continue
			}
			cloned.Messages[index].ToolCalls = append([]ToolCall(nil), req.Messages[index].ToolCalls...)
			for toolIndex := range cloned.Messages[index].ToolCalls {
				cloned.Messages[index].ToolCalls[toolIndex].Function.Name = mapping.Remap(cloned.Messages[index].ToolCalls[toolIndex].Function.Name)
			}
		}
	}
	return cloned
}

func RestoreToolUseNames(toolUses []ToolUse, mapping ToolNameMapping) []ToolUse {
	if len(toolUses) == 0 {
		return nil
	}
	cloned := append([]ToolUse(nil), toolUses...)
	for index := range cloned {
		cloned[index].Name = mapping.Restore(cloned[index].Name)
	}
	return cloned
}

func NormalizeToolsForProvider(providerName string, tools []Tool) []Tool {
	if len(tools) == 0 {
		return nil
	}
	normalized := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		if !ToolSupportedByProvider(providerName, tool) {
			continue
		}
		normalized = append(normalized, tool)
	}
	return normalized
}

func ToolSupportedByProvider(providerName string, tool Tool) bool {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	_ = providerName
	_ = tool

	return true
}

func BuildToolNameMapping(tools []Tool, messages []Message, maxLen int) ToolNameMapping {
	if maxLen <= 0 {
		maxLen = DefaultToolNameLimit
	}
	mapping := ToolNameMapping{forward: map[string]string{}, reverse: map[string]string{}}
	seenShort := make(map[string]struct{})
	register := func(name string) {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return
		}
		if _, ok := mapping.forward[trimmed]; ok {
			return
		}
		short := shortenToolName(trimmed, maxLen)
		for short != trimmed {
			if _, exists := seenShort[short]; !exists {
				break
			}
			short = shortenToolName(trimmed+"_", maxLen)
		}
		mapping.forward[trimmed] = short
		mapping.reverse[short] = trimmed
		seenShort[short] = struct{}{}
	}
	for _, tool := range tools {
		register(tool.Function.Name)
	}
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			register(call.Function.Name)
		}
	}
	return mapping
}

func (m ToolNameMapping) Remap(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	if mapped, ok := m.forward[trimmed]; ok {
		return mapped
	}
	return trimmed
}

func (m ToolNameMapping) Restore(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	if restored, ok := m.reverse[trimmed]; ok {
		return restored
	}
	return trimmed
}

func NormalizeToolSchema(schema any) any {
	mapSchema, ok := schema.(map[string]any)
	if !ok || mapSchema == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}}
	}
	cloned := make(map[string]any, len(mapSchema)+2)
	for key, value := range mapSchema {
		cloned[key] = value
	}
	if _, ok := cloned["type"]; !ok {
		cloned["type"] = "object"
	}
	if _, ok := cloned["properties"]; !ok {
		cloned["properties"] = map[string]any{}
	}
	if _, ok := cloned["required"]; !ok {
		cloned["required"] = []any{}
	}
	return cloned
}

func shortenToolName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	sum := sha1.Sum([]byte(name))
	suffix := "_" + hex.EncodeToString(sum[:])[:10]
	limit := maxLen - len(suffix)
	if limit <= 0 {
		return name[:maxLen]
	}
	return name[:limit] + suffix
}
