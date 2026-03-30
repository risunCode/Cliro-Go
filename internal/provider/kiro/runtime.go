package kiro

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"cliro-go/internal/adapter/ir"
	provider "cliro-go/internal/provider"

	"github.com/google/uuid"
)

type EventParser struct{}

func NewEventParser() *EventParser {
	return &EventParser{}
}

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

var bracketToolCallPattern = regexp.MustCompile(`\[Called\s+([\w.:-]+)\s+with\s+args:\s*`)

func ParseBracketToolCalls(content string) (string, []provider.ToolUse) {
	if strings.TrimSpace(content) == "" {
		return content, nil
	}

	matches := bracketToolCallPattern.FindAllStringIndex(content, -1)
	if len(matches) == 0 {
		return content, nil
	}

	toolCalls := make([]provider.ToolUse, 0, len(matches))
	cleanParts := make([]string, 0, len(matches)+1)
	lastEnd := 0

	for _, location := range matches {
		start := location[0]
		patternEnd := location[1]
		if start > lastEnd {
			cleanParts = append(cleanParts, content[lastEnd:start])
		}

		subMatch := bracketToolCallPattern.FindStringSubmatch(content[start:])
		if len(subMatch) < 2 {
			lastEnd = patternEnd
			continue
		}

		jsonStart := patternEnd
		jsonEnd := findMatchingBrace(content, jsonStart)
		if jsonEnd < 0 {
			lastEnd = patternEnd
			continue
		}

		jsonString := content[jsonStart : jsonEnd+1]
		input := map[string]any{}
		_ = json.Unmarshal([]byte(jsonString), &input)

		toolCalls = append(toolCalls, provider.ToolUse{
			ID:    "toolu_" + uuid.NewString()[:8],
			Name:  subMatch[1],
			Input: input,
		})

		end := jsonEnd + 1
		if end < len(content) && content[end] == ']' {
			end++
		}
		lastEnd = end
	}

	if lastEnd < len(content) {
		cleanParts = append(cleanParts, content[lastEnd:])
	}

	cleanContent := strings.TrimSpace(strings.Join(cleanParts, ""))
	return cleanContent, toolCalls
}

func DeduplicateToolCalls(calls []provider.ToolUse) []provider.ToolUse {
	if len(calls) <= 1 {
		return calls
	}

	byID := make(map[string]provider.ToolUse, len(calls))
	orderedIDs := make([]string, 0, len(calls))
	for _, call := range calls {
		current := call
		if strings.TrimSpace(current.ID) == "" {
			current.ID = "toolu_" + uuid.NewString()[:8]
		}
		existing, exists := byID[current.ID]
		if exists {
			if len(current.Input) > len(existing.Input) {
				byID[current.ID] = current
			}
			continue
		}
		byID[current.ID] = current
		orderedIDs = append(orderedIDs, current.ID)
	}

	intermediate := make([]provider.ToolUse, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		intermediate = append(intermediate, byID[id])
	}

	seen := make(map[string]bool, len(intermediate))
	result := make([]provider.ToolUse, 0, len(intermediate))
	for _, call := range intermediate {
		argsJSON, _ := json.Marshal(call.Input)
		key := call.Name + "|" + string(argsJSON)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, call)
	}

	return result
}

func findMatchingBrace(text string, startPos int) int {
	if startPos >= len(text) || text[startPos] != '{' {
		return -1
	}

	depth := 0
	inString := false
	escaped := false

	for index := startPos; index < len(text); index++ {
		char := text[index]
		if escaped {
			escaped = false
			continue
		}

		if char == '\\' && inString {
			escaped = true
			continue
		}

		if char == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		switch char {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return index
			}
		}
	}

	return -1
}

type thinkingParserState int

const (
	statePreContent thinkingParserState = iota
	stateInThinking
	stateStreaming
)

type ThinkingParser struct {
	state          thinkingParserState
	initialBuffer  string
	thinkingBuffer string
	openTags       []string
	maxTagLength   int
	closeTag       string
}

func NewThinkingParser(openTags []string) *ThinkingParser {
	if len(openTags) == 0 {
		openTags = []string{"<thinking>", "<think>", "<reasoning>"}
	}

	maxLength := 0
	for _, tag := range openTags {
		if len(tag) > maxLength {
			maxLength = len(tag)
		}
	}

	return &ThinkingParser{
		state:        statePreContent,
		openTags:     append([]string(nil), openTags...),
		maxTagLength: maxLength * 2,
	}
}

func ExtractTaggedThinking(content string) (string, string) {
	parser := NewThinkingParser(nil)
	regular, thinking := parser.Feed(content)
	finalRegular, finalThinking := parser.Finalize()

	regular = strings.TrimSpace(regular + finalRegular)
	thinking = strings.TrimSpace(thinking + finalThinking)

	if thinking == "" {
		return strings.TrimSpace(content), ""
	}

	return regular, thinking
}

func (p *ThinkingParser) Feed(content string) (string, string) {
	switch p.state {
	case statePreContent:
		return p.handlePreContent(content)
	case stateInThinking:
		return p.handleInThinking(content)
	case stateStreaming:
		return content, ""
	default:
		return "", ""
	}
}

func (p *ThinkingParser) Finalize() (string, string) {
	switch p.state {
	case statePreContent:
		buffered := p.initialBuffer
		p.initialBuffer = ""
		p.state = stateStreaming
		return buffered, ""
	case stateInThinking:
		buffered := p.thinkingBuffer
		p.thinkingBuffer = ""
		p.state = stateStreaming
		return "", buffered
	default:
		return "", ""
	}
}

func (p *ThinkingParser) handlePreContent(content string) (string, string) {
	p.initialBuffer += content
	stripped := strings.TrimLeft(p.initialBuffer, " \t\n\r")

	for _, tag := range p.openTags {
		if strings.HasPrefix(stripped, tag) {
			p.closeTag = "</" + tag[1:]
			p.state = stateInThinking
			afterTag := stripped[len(tag):]
			p.initialBuffer = ""
			return p.handleInThinking(afterTag)
		}
	}

	if p.couldBeTagPrefix(stripped) && len(stripped) < p.maxTagLength {
		return "", ""
	}

	buffered := p.initialBuffer
	p.initialBuffer = ""
	p.state = stateStreaming
	return buffered, ""
}

func (p *ThinkingParser) handleInThinking(content string) (string, string) {
	p.thinkingBuffer += content
	closeIndex := strings.Index(p.thinkingBuffer, p.closeTag)
	if closeIndex >= 0 {
		thinkingContent := p.thinkingBuffer[:closeIndex]
		afterClose := p.thinkingBuffer[closeIndex+len(p.closeTag):]
		p.thinkingBuffer = ""
		p.state = stateStreaming
		return afterClose, thinkingContent
	}

	if len(p.thinkingBuffer) > p.maxTagLength {
		safeLength := len(p.thinkingBuffer) - p.maxTagLength
		safeContent := p.thinkingBuffer[:safeLength]
		p.thinkingBuffer = p.thinkingBuffer[safeLength:]
		return "", safeContent
	}

	return "", ""
}

func (p *ThinkingParser) couldBeTagPrefix(text string) bool {
	if text == "" {
		return true
	}
	for _, tag := range p.openTags {
		if len(text) <= len(tag) && strings.HasPrefix(tag, text) {
			return true
		}
	}
	return false
}
