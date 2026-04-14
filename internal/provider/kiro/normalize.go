package kiro

import (
	"encoding/json"
	"regexp"
	"strings"

	provider "cliro/internal/provider"
)

type normalizedMessage struct {
	Role        string
	Content     string
	ToolUses    []toolUsePayload
	ToolResults []toolResult
}

var internalMetadataBlockPattern = regexp.MustCompile(`(?is)<environment_details>.*?</environment_details>`)

func appendVisibleFragment(parts []string, fragment string) []string {
	if strings.TrimSpace(fragment) == "" {
		return parts
	}
	return append(parts, fragment)
}

func normalizeRequest(req provider.ChatRequest) ([]normalizedMessage, string, error) {
	messages := toNormalizedMessages(req.Messages)
	systemPrompt, messages := splitLeadingSystemPrompt(messages)

	if len(req.Tools) == 0 {
		messages = stripToolContent(messages)
	} else {
		messages = convertOrphanToolResults(messages)
	}

	messages = mergeAdjacentMessages(messages)

	// Enhanced sanitization pipeline
	messages = ensureStartsWithUserMessage(messages)
	messages = removeEmptyUserMessages(messages)
	messages = normalizeMessageRoles(messages)
	messages = ensureAlternatingRoles(messages)
	messages = ensureEndsWithUserMessage(messages)

	if len(messages) == 0 {
		return nil, systemPrompt, errMessagesEmpty
	}
	return messages, systemPrompt, nil
}

func toNormalizedMessages(messages []provider.Message) []normalizedMessage {
	result := make([]normalizedMessage, 0, len(messages))
	for _, message := range messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role == "" {
			role = "user"
		}
		originalRole := role

		toolUses := extractToolUses(message)
		toolResults := extractToolResults(message)
		text := sanitizePromptText(messageTextContent(message.Content))

		// Append thinking blocks to text content
		if len(message.ThinkingBlocks) > 0 {
			thinkingParts := make([]string, 0, len(message.ThinkingBlocks))
			for _, block := range message.ThinkingBlocks {
				if thinking := strings.TrimSpace(block.Thinking); thinking != "" {
					thinkingParts = append(thinkingParts, thinking)
				}
			}
			if len(thinkingParts) > 0 {
				text = joinNonEmpty(strings.Join(thinkingParts, "\n\n"), text)
			}
		}

		switch role {
		case "tool":
			role = "user"
			text = ""
		case "system", "developer", "assistant", "user":
		default:
			role = "user"
		}

		if role == "assistant" && text == "" && len(toolUses) == 0 {
			continue
		}
		if role == "user" && text == "" && len(toolResults) == 0 && originalRole != "tool" {
			continue
		}

		result = append(result, normalizedMessage{
			Role:        role,
			Content:     text,
			ToolUses:    toolUses,
			ToolResults: toolResults,
		})
	}
	return result
}

func splitLeadingSystemPrompt(messages []normalizedMessage) (string, []normalizedMessage) {
	if len(messages) == 0 {
		return "", nil
	}

	parts := make([]string, 0)
	index := 0
	for index < len(messages) {
		role := strings.ToLower(strings.TrimSpace(messages[index].Role))
		if role != "system" && role != "developer" {
			break
		}
		if text := sanitizePromptText(messages[index].Content); text != "" {
			parts = append(parts, text)
		}
		index++
	}

	if len(parts) == 0 {
		return "", messages
	}
	return strings.Join(parts, "\n\n"), messages[index:]
}

func stripToolContent(messages []normalizedMessage) []normalizedMessage {
	result := make([]normalizedMessage, 0, len(messages))
	for _, message := range messages {
		if len(message.ToolUses) == 0 && len(message.ToolResults) == 0 {
			result = append(result, message)
			continue
		}

		parts := make([]string, 0, 1+len(message.ToolUses)+len(message.ToolResults))
		if text := strings.TrimSpace(message.Content); text != "" {
			parts = append(parts, text)
		}
		if len(message.ToolUses) > 0 {
			parts = append(parts, toolUsesToText(message.ToolUses))
		}
		if len(message.ToolResults) > 0 {
			parts = append(parts, toolResultsToText(message.ToolResults))
		}

		message.Content = joinNonEmpty(parts...)
		message.ToolUses = nil
		message.ToolResults = nil
		result = append(result, message)
	}
	return result
}

func convertOrphanToolResults(messages []normalizedMessage) []normalizedMessage {
	result := make([]normalizedMessage, 0, len(messages))
	for _, message := range messages {
		if len(message.ToolResults) > 0 {
			hasAssistantToolUse := hasPriorAssistantToolUse(result)
			if !hasAssistantToolUse {
				message.Content = joinNonEmpty(message.Content, toolResultsToText(message.ToolResults))
				message.ToolResults = nil
			}
		}
		result = append(result, message)
	}
	return result
}

func hasPriorAssistantToolUse(messages []normalizedMessage) bool {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "assistant" {
			return len(messages[index].ToolUses) > 0
		}
	}
	return false
}

func mergeAdjacentMessages(messages []normalizedMessage) []normalizedMessage {
	if len(messages) < 2 {
		return messages
	}

	merged := make([]normalizedMessage, 0, len(messages))
	for _, message := range messages {
		if len(merged) == 0 || merged[len(merged)-1].Role != message.Role {
			merged = append(merged, message)
			continue
		}

		current := &merged[len(merged)-1]
		current.Content = joinNonEmpty(current.Content, message.Content)
		current.ToolUses = append(current.ToolUses, message.ToolUses...)
		current.ToolResults = append(current.ToolResults, message.ToolResults...)
	}
	return merged
}

func ensureStartsWithUserMessage(messages []normalizedMessage) []normalizedMessage {
	if len(messages) == 0 || messages[0].Role == "user" {
		return messages
	}
	return append([]normalizedMessage{{Role: "user", Content: "Hello"}}, messages...)
}

func removeEmptyUserMessages(messages []normalizedMessage) []normalizedMessage {
	if len(messages) == 0 {
		return messages
	}
	result := make([]normalizedMessage, 0, len(messages))
	for i, msg := range messages {
		// Keep first message even if empty
		if i == 0 {
			result = append(result, msg)
			continue
		}
		// Remove empty user messages (no content, no tool results)
		if msg.Role == "user" && strings.TrimSpace(msg.Content) == "" && len(msg.ToolResults) == 0 {
			continue
		}
		result = append(result, msg)
	}
	return result
}

func ensureEndsWithUserMessage(messages []normalizedMessage) []normalizedMessage {
	if len(messages) == 0 {
		return messages
	}
	if messages[len(messages)-1].Role == "user" {
		return messages
	}
	return append(messages, normalizedMessage{Role: "user", Content: "Continue"})
}

func normalizeMessageRoles(messages []normalizedMessage) []normalizedMessage {
	result := make([]normalizedMessage, 0, len(messages))
	for _, message := range messages {
		if message.Role != "user" && message.Role != "assistant" {
			message.Role = "user"
		}
		result = append(result, message)
	}
	return result
}

func ensureAlternatingRoles(messages []normalizedMessage) []normalizedMessage {
	if len(messages) < 2 {
		return messages
	}

	result := make([]normalizedMessage, 0, len(messages)*2)
	result = append(result, messages[0])

	for _, message := range messages[1:] {
		prev := result[len(result)-1]

		if prev.Role == "user" && message.Role == "user" {
			result = append(result, normalizedMessage{Role: "assistant"})
		} else if prev.Role == "assistant" && message.Role == "assistant" {
			result = append(result, normalizedMessage{Role: "user", Content: "Continue"})
		}

		result = append(result, message)
	}
	return result
}

func extractToolUses(message provider.Message) []toolUsePayload {
	toolUses := make([]toolUsePayload, 0, len(message.ToolCalls))
	for _, toolCall := range message.ToolCalls {
		name := strings.TrimSpace(toolCall.Function.Name)
		if name == "" {
			continue
		}
		toolUses = append(toolUses, toolUsePayload{ToolUseID: strings.TrimSpace(toolCall.ID), Name: name, Input: parseToolArguments(toolCall.Function.Arguments)})
	}
	toolUses = append(toolUses, extractInlineToolUses(message.Content)...)
	return toolUses
}

func extractToolResults(message provider.Message) []toolResult {
	results := make([]toolResult, 0, 1)
	if strings.EqualFold(strings.TrimSpace(message.Role), "tool") && strings.TrimSpace(message.ToolCallID) != "" {
		results = append(results, toolResult{ToolUseID: strings.TrimSpace(message.ToolCallID), Status: "success", Content: []toolResultContent{{Text: defaultIfEmpty(messageTextContent(message.Content), "(empty result)")}}})
	}
	results = append(results, extractInlineToolResults(message.Content)...)
	return results
}

func extractInlineToolUses(content any) []toolUsePayload {
	blocks, ok := content.([]any)
	if !ok {
		return nil
	}
	toolUses := make([]toolUsePayload, 0)
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok || !strings.EqualFold(asString(block["type"]), "tool_use") {
			continue
		}
		name := strings.TrimSpace(asString(block["name"]))
		if name == "" {
			continue
		}
		toolUses = append(toolUses, toolUsePayload{ToolUseID: strings.TrimSpace(asString(block["id"])), Name: name, Input: anyToMap(block["input"])})
	}
	return toolUses
}

func extractInlineToolResults(content any) []toolResult {
	blocks, ok := content.([]any)
	if !ok {
		return nil
	}
	results := make([]toolResult, 0)
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok || !strings.EqualFold(asString(block["type"]), "tool_result") {
			continue
		}
		toolUseID := strings.TrimSpace(asString(block["tool_use_id"]))
		if toolUseID == "" {
			continue
		}
		results = append(results, toolResult{ToolUseID: toolUseID, Status: "success", Content: []toolResultContent{{Text: defaultIfEmpty(messageTextContent(block["content"]), "(empty result)")}}})
	}
	return results
}

func messageTextContent(content any) string {
	switch typed := content.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				parts = appendVisibleFragment(parts, messageTextContent(item))
				continue
			}
			switch strings.ToLower(strings.TrimSpace(asString(block["type"]))) {
			case "text", "input_text", "output_text":
				if text := asString(block["text"]); strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			case "refusal":
				if text := asString(block["refusal"]); strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			case "thinking":
				if text := asString(block["thinking"]); strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			case "tool_use", "tool_result", "image", "image_url":
				continue
			default:
				if text := asString(block["text"]); strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	case map[string]any:
		if text := strings.TrimSpace(asString(typed["text"])); text != "" {
			return text
		}
		if thinking := strings.TrimSpace(asString(typed["thinking"])); thinking != "" {
			return thinking
		}
		if nested, ok := typed["content"]; ok {
			return messageTextContent(nested)
		}
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	default:
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	}
}

func toolUsesToText(toolUses []toolUsePayload) string {
	parts := make([]string, 0, len(toolUses))
	for _, toolUse := range toolUses {
		encoded, _ := json.Marshal(defaultIfNilMap(toolUse.Input))
		if strings.TrimSpace(toolUse.ToolUseID) != "" {
			parts = append(parts, "[Tool: "+toolUse.Name+" ("+strings.TrimSpace(toolUse.ToolUseID)+")]\n"+string(encoded))
			continue
		}
		parts = append(parts, "[Tool: "+toolUse.Name+"]\n"+string(encoded))
	}
	return strings.Join(parts, "\n\n")
}

func toolResultsToText(results []toolResult) string {
	parts := make([]string, 0, len(results))
	for _, result := range results {
		text := "(empty result)"
		if len(result.Content) > 0 && strings.TrimSpace(result.Content[0].Text) != "" {
			text = strings.TrimSpace(result.Content[0].Text)
		}
		if strings.TrimSpace(result.ToolUseID) != "" {
			parts = append(parts, "[Tool Result ("+strings.TrimSpace(result.ToolUseID)+")]\n"+text)
			continue
		}
		parts = append(parts, "[Tool Result]\n"+text)
	}
	return strings.Join(parts, "\n\n")
}

func sanitizePromptText(text string) string {
	return collapseBlankLines(stripInternalMetadataBlocks(text))
}

func sanitizeModelOutputText(text string) string {
	return collapseBlankLines(stripInternalMetadataBlocks(text))
}

func sanitizeModelOutputDelta(text string) string {
	if text == "" {
		return ""
	}
	return internalMetadataBlockPattern.ReplaceAllString(text, "")
}

func stripInternalMetadataBlocks(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	// Fast path: skip regex when no metadata block is present (common case).
	if !strings.Contains(trimmed, "<environment_details>") {
		return trimmed
	}
	cleaned := internalMetadataBlockPattern.ReplaceAllString(trimmed, "")
	return strings.TrimSpace(cleaned)
}

func collapseBlankLines(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	// Fast path: no newlines means nothing to collapse.
	if !strings.Contains(trimmed, "\n") {
		return trimmed
	}
	var b strings.Builder
	b.Grow(len(trimmed))
	blankPending := false
	start := 0
	for i := 0; i <= len(trimmed); i++ {
		if i == len(trimmed) || trimmed[i] == '\n' {
			line := strings.TrimRight(trimmed[start:i], " \t\r")
			if strings.TrimSpace(line) == "" {
				if b.Len() > 0 {
					blankPending = true
				}
			} else {
				if b.Len() > 0 {
					b.WriteByte('\n')
					if blankPending {
						b.WriteByte('\n')
						blankPending = false
					}
				}
				b.WriteString(line)
			}
			start = i + 1
		}
	}
	return b.String()
}

func parseToolArguments(arguments string) map[string]any {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return map[string]any{}
	}
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return map[string]any{}
	}
	return anyToMap(parsed)
}

func anyToMap(value any) map[string]any {
	switch typed := value.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return typed
	default:
		return map[string]any{}
	}
}

func asString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func joinNonEmpty(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if text := strings.TrimSpace(part); text != "" {
			filtered = append(filtered, text)
		}
	}
	return strings.Join(filtered, "\n\n")
}

func defaultIfEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultIfNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}
