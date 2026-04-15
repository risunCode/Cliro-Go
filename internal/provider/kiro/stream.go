package kiro

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"cliro/internal/config"
	models "cliro/internal/proxy/models"

	"github.com/google/uuid"
)

type awsEvent struct {
	Type string
	Data []byte
}

type awsEventReader struct{ reader *bufio.Reader }

func newAWSEventReader(r io.Reader) *awsEventReader {
	return &awsEventReader{reader: bufio.NewReader(r)}
}

func (r *awsEventReader) ReadEvent() (*awsEvent, error) {
	prelude := make([]byte, 12)
	if _, err := io.ReadFull(r.reader, prelude); err != nil {
		return nil, err
	}
	totalLength := binary.BigEndian.Uint32(prelude[0:4])
	headersLength := binary.BigEndian.Uint32(prelude[4:8])
	payloadLength := totalLength - headersLength - 16
	headersData := make([]byte, headersLength)
	if _, err := io.ReadFull(r.reader, headersData); err != nil {
		return nil, err
	}
	headers, err := parseEventHeaders(headersData)
	if err != nil {
		return nil, err
	}
	payload := make([]byte, payloadLength)
	if _, err := io.ReadFull(r.reader, payload); err != nil {
		return nil, err
	}
	crc := make([]byte, 4)
	if _, err := io.ReadFull(r.reader, crc); err != nil {
		return nil, err
	}
	_ = crc
	return &awsEvent{Type: headers[":message-type"], Data: payload}, nil
}

func parseEventHeaders(data []byte) (map[string]string, error) {
	headers := make(map[string]string)
	buf := bytes.NewReader(data)
	for buf.Len() > 0 {
		nameLen, err := buf.ReadByte()
		if err != nil {
			break
		}
		name := make([]byte, nameLen)
		if _, err := io.ReadFull(buf, name); err != nil {
			return nil, err
		}
		valueType, err := buf.ReadByte()
		if err != nil {
			return nil, err
		}
		if valueType != 7 {
			return nil, fmt.Errorf("unsupported header value type: %d", valueType)
		}
		var valueLen uint16
		if err := binary.Read(buf, binary.BigEndian, &valueLen); err != nil {
			return nil, err
		}
		value := make([]byte, valueLen)
		if _, err := io.ReadFull(buf, value); err != nil {
			return nil, err
		}
		headers[string(name)] = string(value)
	}
	return headers, nil
}

type parserState struct {
	buffer          string
	lastContent     string
	lastThinking    string
	currentToolCall map[string]any
	toolCalls       []ToolUse
	usage           models.Usage
	textParts       []string
	thinkingParts   []string
	gotUsage        bool
	gotContextUsage bool
}

func collectCompletion(r io.Reader, model string) (CompletionOutcome, error) {
	reader := newAWSEventReader(r)
	state := &parserState{}
	for {
		event, err := reader.ReadEvent()
		if err != nil {
			if err == io.EOF {
				break
			}
			return CompletionOutcome{}, err
		}
		if event.Type != "event" {
			continue
		}
		state.feed(event.Data)
	}
	state.finalizeToolCall()
	thinking := extractThinking(strings.Join(state.textParts, ""))
	text := stripThinkingTags(strings.Join(state.textParts, ""))
	if strings.TrimSpace(thinking) == "" {
		thinking = strings.Join(state.thinkingParts, "")
	}
	return CompletionOutcome{
		ID:                "msg_" + uuid.NewString(),
		Model:             model,
		Text:              strings.TrimSpace(text),
		Thinking:          strings.TrimSpace(thinking),
		ThinkingSignature: models.StableThinkingSignature(thinking),
		ThinkingSource:    thinkingSource(thinking),
		ToolUses:          append([]ToolUse(nil), state.toolCalls...),
		Usage:             config.ProxyStats{PromptTokens: state.usage.InputTokens, CompletionTokens: state.usage.OutputTokens, TotalTokens: maxStreamInt(state.usage.TotalTokens, state.usage.InputTokens+state.usage.OutputTokens)},
	}, nil
}

func (p *parserState) feed(chunk []byte) {
	p.buffer += string(chunk)
	patterns := []struct{ pattern, kind string }{
		{"{\"content\":", "content"},
		{"{\"thinking\":", "thinking"},
		{"{\"name\":", "tool_start"},
		{"{\"input\":", "tool_input"},
		{"{\"stop\":", "tool_stop"},
		{"{\"usage\":", "usage"},
		{"{\"followupPrompt\":", "followup"},
		{"{\"contextUsagePercentage\":", "context_usage"},
	}
	for {
		pos := -1
		kind := ""
		for _, pattern := range patterns {
			idx := strings.Index(p.buffer, pattern.pattern)
			if idx != -1 && (pos == -1 || idx < pos) {
				pos = idx
				kind = pattern.kind
			}
		}
		if pos == -1 {
			break
		}
		end := findMatchingBrace(p.buffer, pos)
		if end == -1 {
			break
		}
		jsonStr := p.buffer[pos : end+1]
		p.buffer = p.buffer[end+1:]
		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}
		p.processEvent(kind, data)
	}
}

func (p *parserState) processEvent(kind string, data map[string]any) {
	switch kind {
	case "content":
		if _, hasFollowup := data["followupPrompt"]; hasFollowup {
			return
		}
		content, _ := data["content"].(string)
		if content == "" || content == p.lastContent {
			return
		}
		p.lastContent = content
		p.textParts = append(p.textParts, content)
	case "thinking":
		thinking, _ := data["thinking"].(string)
		if thinking == "" || thinking == p.lastThinking {
			return
		}
		p.lastThinking = thinking
		p.thinkingParts = append(p.thinkingParts, thinking)
	case "tool_start":
		p.finalizeToolCall()
		name, _ := data["name"].(string)
		toolID, _ := data["toolUseId"].(string)
		if toolID == "" {
			toolID = "call_" + uuid.NewString()[:12]
		}
		p.currentToolCall = map[string]any{"id": toolID, "name": name, "arguments": normalizeToolInput(data["input"])}
		if stop, _ := data["stop"].(bool); stop {
			p.finalizeToolCall()
		}
	case "tool_input":
		if p.currentToolCall != nil {
			p.currentToolCall["arguments"] = stringValue(p.currentToolCall["arguments"]) + normalizeToolInput(data["input"])
		}
	case "tool_stop":
		if stop, _ := data["stop"].(bool); stop {
			p.finalizeToolCall()
		}
	case "usage":
		p.gotUsage = true
		if usageMap, ok := data["usage"].(map[string]any); ok {
			p.usage.InputTokens = maxStreamInt(p.usage.InputTokens, int(numberValue(firstNumber(usageMap["inputTokens"], usageMap["input_tokens"]))))
			p.usage.OutputTokens = maxStreamInt(p.usage.OutputTokens, int(numberValue(firstNumber(usageMap["outputTokens"], usageMap["output_tokens"]))))
			p.usage.TotalTokens = maxStreamInt(p.usage.TotalTokens, int(numberValue(firstNumber(usageMap["totalTokens"], usageMap["total_tokens"]))))
		}
	case "context_usage":
		p.gotContextUsage = true
	}
}

func (p *parserState) finalizeToolCall() {
	if p.currentToolCall == nil {
		return
	}
	args := strings.TrimSpace(stringValue(p.currentToolCall["arguments"]))
	if args == "" {
		args = "{}"
	}
	input := map[string]any{}
	if json.Unmarshal([]byte(args), &input) != nil {
		input = map[string]any{}
	}
	toolUse := ToolUse{ID: stringValue(p.currentToolCall["id"]), Name: stringValue(p.currentToolCall["name"]), Input: input}
	for _, existing := range p.toolCalls {
		if strings.TrimSpace(existing.Name) == strings.TrimSpace(toolUse.Name) && marshalComparable(existing.Input) == marshalComparable(toolUse.Input) {
			p.currentToolCall = nil
			return
		}
	}
	p.toolCalls = append(p.toolCalls, toolUse)
	p.currentToolCall = nil
}

func marshalComparable(value map[string]any) string {
	if len(value) == 0 {
		return "{}"
	}
	data, _ := json.Marshal(value)
	return string(data)
}

func normalizeToolInput(input any) string {
	switch typed := input.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		data, _ := json.Marshal(typed)
		if string(data) == "null" {
			return ""
		}
		return string(data)
	}
}

func findMatchingBrace(text string, startPos int) int {
	if startPos < 0 || startPos >= len(text) || text[startPos] != '{' {
		return -1
	}
	braceCount := 0
	inString := false
	escapeNext := false
	for i := startPos; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if escapeNext {
			escapeNext = false
			i += size
			continue
		}
		if r == '\\' && inString {
			escapeNext = true
			i += size
			continue
		}
		if r == '"' {
			inString = !inString
			i += size
			continue
		}
		if !inString {
			if r == '{' {
				braceCount++
			} else if r == '}' {
				braceCount--
				if braceCount == 0 {
					return i
				}
			}
		}
		i += size
	}
	return -1
}

func extractThinking(content string) string {
	for _, tag := range []string{"thinking", "think", "reasoning", "thought"} {
		openTag := "<" + tag + ">"
		closeTag := "</" + tag + ">"
		start := strings.Index(strings.ToLower(content), openTag)
		end := strings.Index(strings.ToLower(content), closeTag)
		if start >= 0 && end > start {
			return strings.TrimSpace(content[start+len(openTag) : end])
		}
	}
	return ""
}

func stripThinkingTags(content string) string {
	out := content
	for _, tag := range []string{"thinking", "think", "reasoning", "thought"} {
		for {
			lower := strings.ToLower(out)
			openTag := "<" + tag + ">"
			closeTag := "</" + tag + ">"
			start := strings.Index(lower, openTag)
			end := strings.Index(lower, closeTag)
			if start < 0 || end <= start {
				break
			}
			out = strings.TrimSpace(out[:start] + out[end+len(closeTag):])
		}
	}
	return out
}

func thinkingSource(thinking string) string {
	if strings.TrimSpace(thinking) != "" {
		return "parsed"
	}
	return "none"
}

func numberValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		return 0
	}
}

func firstNumber(values ...any) any {
	for _, value := range values {
		switch value.(type) {
		case float64, int, int64, json.Number:
			return value
		}
	}
	return nil
}

func maxStreamInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
