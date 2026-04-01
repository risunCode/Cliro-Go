package kiro

import (
	"encoding/json"
	"regexp"
	"strings"

	provider "cliro-go/internal/provider"
)

type normalizedMessage struct {
	Role        string
	Content     string
	ToolUses    []toolUsePayload
	ToolResults []toolResult
}

var internalMetadataBlockPattern = regexp.MustCompile(`(?is)<environment_details>.*?</environment_details>`)

func normalizeRequest(req provider.ChatRequest) ([]normalizedMessage, string, error) {
	messages := toNormalizedMessages(req.Messages)
	systemPrompt, messages := splitLeadingSystemPrompt(messages)

	if len(req.Tools) == 0 {
		messages = stripToolContent(messages)
	} else {
		messages = convertOrphanToolResults(messages)
	}

	messages = mergeAdjacentMessages(messages)
	messages = expandToolUseResultPairs(messages)

	// Enhanced sanitization pipeline
	messages = ensureStartsWithUserMessage(messages)
	messages = removeEmptyUserMessages(messages)
	messages = ensureValidToolUsesAndResults(messages)
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

func ensureValidToolUsesAndResults(messages []normalizedMessage) []normalizedMessage {
	// Collect all tool use IDs from assistant messages
	toolUseIDs := make(map[string]struct{})
	for _, msg := range messages {
		if msg.Role == "assistant" {
			for _, toolUse := range msg.ToolUses {
				if id := strings.TrimSpace(toolUse.ToolUseID); id != "" {
					toolUseIDs[id] = struct{}{}
				}
			}
		}
	}

	// Validate tool results in user messages
	result := make([]normalizedMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "user" && len(msg.ToolResults) > 0 {
			validResults := make([]toolResult, 0, len(msg.ToolResults))
			for _, tr := range msg.ToolResults {
				id := strings.TrimSpace(tr.ToolUseID)
				if id == "" {
					continue
				}
				// If tool use ID not found, mark as error
				if _, exists := toolUseIDs[id]; !exists {
					tr.Status = "error"
					tr.Content = []toolResultContent{{Text: "Tool use not found"}}
				}
				validResults = append(validResults, tr)
			}
			msg.ToolResults = validResults
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

func expandToolUseResultPairs(messages []normalizedMessage) []normalizedMessage {
	if len(messages) == 0 {
		return nil
	}

	expanded := make([]normalizedMessage, 0, len(messages))
	for index := 0; index < len(messages); index++ {
		message := messages[index]
		if message.Role == "assistant" && len(message.ToolUses) > 1 {
			var next *normalizedMessage
			if index+1 < len(messages) && messages[index+1].Role == "user" && len(messages[index+1].ToolResults) > 0 {
				next = &messages[index+1]
			}
			expanded = append(expanded, splitAssistantToolUses(message, next)...)
			if next != nil {
				index++
			}
			continue
		}
		if message.Role == "user" && len(message.ToolResults) > 1 {
			expanded = append(expanded, splitUserToolResults(message)...)
			continue
		}
		expanded = append(expanded, message)
	}
	return expanded
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
				if text := strings.TrimSpace(messageTextContent(item)); text != "" {
					parts = append(parts, text)
				}
				continue
			}
			switch strings.ToLower(strings.TrimSpace(asString(block["type"]))) {
			case "text", "input_text", "output_text":
				if text := strings.TrimSpace(asString(block["text"])); text != "" {
					parts = append(parts, text)
				}
			case "refusal":
				if text := strings.TrimSpace(asString(block["refusal"])); text != "" {
					parts = append(parts, text)
				}
			case "thinking":
				if text := strings.TrimSpace(asString(block["thinking"])); text != "" {
					parts = append(parts, text)
				}
			case "tool_use", "tool_result", "image", "image_url":
				continue
			default:
				if text := strings.TrimSpace(asString(block["text"])); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
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

func stripInternalMetadataBlocks(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	cleaned := internalMetadataBlockPattern.ReplaceAllString(trimmed, "")
	return strings.TrimSpace(cleaned)
}

func collapseBlankLines(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	parts := make([]string, 0, len(lines))
	blankPending := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(line) == "" {
			if len(parts) > 0 {
				blankPending = true
			}
			continue
		}
		if blankPending {
			parts = append(parts, "")
			blankPending = false
		}
		parts = append(parts, line)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
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

func splitAssistantToolUses(message normalizedMessage, next *normalizedMessage) []normalizedMessage {
	if len(message.ToolUses) <= 1 {
		result := []normalizedMessage{message}
		if next != nil {
			result = append(result, *next)
		}
		return result
	}

	resultsByID := make(map[string]toolResult)
	orderedUnmatched := make([]toolResult, 0)
	nextContent := ""
	if next != nil {
		nextContent = strings.TrimSpace(next.Content)
		for _, result := range next.ToolResults {
			id := strings.TrimSpace(result.ToolUseID)
			if id == "" {
				orderedUnmatched = append(orderedUnmatched, result)
				continue
			}
			if _, exists := resultsByID[id]; exists {
				continue
			}
			resultsByID[id] = result
		}
	}

	expanded := make([]normalizedMessage, 0, (len(message.ToolUses)*2)+1)
	emittedUser := false
	for toolIndex, toolUse := range message.ToolUses {
		assistantContent := ""
		if toolIndex == 0 {
			assistantContent = strings.TrimSpace(message.Content)
		}
		expanded = append(expanded, normalizedMessage{
			Role:     "assistant",
			Content:  assistantContent,
			ToolUses: []toolUsePayload{toolUse},
		})

		if next == nil {
			continue
		}

		result, ok := resultsByID[strings.TrimSpace(toolUse.ToolUseID)]
		if !ok {
			continue
		}
		userContent := ""
		if toolIndex == len(message.ToolUses)-1 {
			userContent = nextContent
		}
		expanded = append(expanded, normalizedMessage{
			Role:        "user",
			Content:     userContent,
			ToolResults: []toolResult{result},
		})
		emittedUser = true
		delete(resultsByID, strings.TrimSpace(toolUse.ToolUseID))
	}

	if next == nil {
		return expanded
	}

	leftovers := make([]toolResult, 0, len(resultsByID)+len(orderedUnmatched))
	for _, result := range next.ToolResults {
		id := strings.TrimSpace(result.ToolUseID)
		if id == "" {
			leftovers = append(leftovers, result)
			continue
		}
		if _, ok := resultsByID[id]; ok {
			leftovers = append(leftovers, result)
		}
	}
	if len(leftovers) > 0 || (!emittedUser && nextContent != "") {
		expanded = append(expanded, normalizedMessage{
			Role:        "user",
			Content:     nextContent,
			ToolResults: leftovers,
		})
	}

	return expanded
}

func splitUserToolResults(message normalizedMessage) []normalizedMessage {
	if len(message.ToolResults) <= 1 {
		return []normalizedMessage{message}
	}

	expanded := make([]normalizedMessage, 0, len(message.ToolResults))
	for index, result := range message.ToolResults {
		content := ""
		if index == len(message.ToolResults)-1 {
			content = strings.TrimSpace(message.Content)
		}
		expanded = append(expanded, normalizedMessage{
			Role:        "user",
			Content:     content,
			ToolResults: []toolResult{result},
		})
	}
	return expanded
}
