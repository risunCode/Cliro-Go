package kiro

import (
	"cliro/internal/util"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"strings"
	"unicode/utf8"

	"cliro/internal/config"
	contract "cliro/internal/contract"
	provider "cliro/internal/provider"
	providerthinking "cliro/internal/provider/thinking"

	"github.com/google/uuid"
)

type StreamEvent struct {
	Text     string
	Thinking string
	Usage    UsageSnapshot
}

type UsageSnapshot struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func collectCompletion(body io.Reader, req provider.ChatRequest) (provider.CompletionOutcome, error) {
	return collectCompletionWithTagsAndMapping(body, req, nil, provider.ToolNameMapping{}, nil)
}

func collectCompletionWithTags(body io.Reader, req provider.ChatRequest, fallbackTags []string) (provider.CompletionOutcome, error) {
	return collectCompletionWithTagsAndMapping(body, req, fallbackTags, provider.ToolNameMapping{}, nil)
}

func collectCompletionWithTagsAndMapping(body io.Reader, req provider.ChatRequest, fallbackTags []string, toolNames provider.ToolNameMapping, callback func(StreamEvent)) (provider.CompletionOutcome, error) {
	outcome := provider.CompletionOutcome{
		ID:    "chatcmpl-" + uuid.NewString(),
		Model: req.Model,
	}

	effectiveTags := fallbackTags
	if req.Thinking.Requested && len(effectiveTags) == 0 {
		effectiveTags = []string{"<thinking>", "<think>"}
	}
	parser := NewStreamParserWithFallbackTags(body, effectiveTags, req.Thinking.Requested)
	var textBuilder strings.Builder
	var thinkingBuilder strings.Builder

	for {
		event, err := parser.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return outcome, err
		}
		if event.Text != "" {
			textBuilder.WriteString(event.Text)
		}
		if event.Thinking != "" {
			thinkingBuilder.WriteString(event.Thinking)
		}
		mergeUsage(&outcome.Usage, event.Usage)

		if callback != nil && (event.Text != "" || event.Thinking != "" || event.Usage.TotalTokens > 0) {
			event.Text = sanitizeModelOutputDelta(event.Text)
			event.Thinking = sanitizeModelOutputDelta(event.Thinking)
			if event.Text != "" || event.Thinking != "" || event.Usage.TotalTokens > 0 || event.Usage.PromptTokens > 0 || event.Usage.CompletionTokens > 0 {
				callback(event)
			}
		}
	}

	toolUses := provider.RestoreToolUseNames(deduplicateToolUses(parser.ToolUses()), toolNames)
	text := sanitizeModelOutputText(textBuilder.String())
	nativeThinking := sanitizeModelOutputText(thinkingBuilder.String())
	parsedThinking := parser.FallbackThinkingCandidate()
	parsedText := text
	if strings.TrimSpace(parsedThinking.Thinking) == "" {
		parsedThinking, parsedText = parseFallbackThinking(text, req, effectiveTags)
	}
	selection := providerthinking.Select(providerthinking.Inputs{
		Request: req.Thinking,
		Native:  thinkingCandidate(nativeThinking),
		Parsed:  parsedThinking,
	})
	text = parsedText
	if extracted, ok := extractBracketToolUses(text); ok {
		toolUses = deduplicateToolUses(append(toolUses, extracted...))
		text = ""
	}

	outcome.Text = text
	outcome.Thinking = selection.Thinking
	outcome.ThinkingSignature = resolveThinkingSignature(selection.Thinking, selection.Signature)
	outcome.ThinkingSource = string(selection.Source)
	outcome.ToolUses = toolUses
	estimateUsageIfMissing(&outcome.Usage, req, &outcome)
	return outcome, nil
}

func parseFallbackThinking(text string, req provider.ChatRequest, fallbackTags []string) (providerthinking.Candidate, string) {
	text = sanitizeModelOutputText(text)
	if !req.Thinking.Requested || text == "" {
		return providerthinking.Candidate{}, text
	}

	if parser := newAssistantFallbackParser(fallbackTags); parser != nil && parser.Enabled() {
		visibleEvents := append(parser.Feed(text), parser.Finalize()...)
		var visibleText strings.Builder
		for _, event := range visibleEvents {
			visibleText.WriteString(event.Text)
		}
		if candidate := parser.ParsedCandidate(); strings.TrimSpace(candidate.Thinking) != "" {
			return candidate, sanitizeModelOutputText(visibleText.String())
		}
	}

	parser := providerthinking.NewLeadingParser(fallbackTags, 0)
	parsed := parser.Feed(text)
	finalized := parser.Finalize()
	remainingText := sanitizeModelOutputText(parsed.Text + finalized.Text)
	thinkingText := sanitizeModelOutputText(parsed.Thinking + finalized.Thinking)
	if !parser.Parsed() {
		return providerthinking.Candidate{}, remainingText
	}
	return thinkingCandidate(thinkingText), remainingText
}

func thinkingCandidate(thinking string) providerthinking.Candidate {
	thinking = sanitizeModelOutputText(thinking)
	if thinking == "" {
		return providerthinking.Candidate{}
	}
	return providerthinking.Candidate{
		Thinking:  thinking,
		Signature: contract.StableThinkingSignature(thinking),
	}
}

func resolveThinkingSignature(thinking string, signature string) string {
	if strings.TrimSpace(thinking) == "" {
		return ""
	}
	if strings.TrimSpace(signature) != "" {
		return strings.TrimSpace(signature)
	}
	return contract.StableThinkingSignature(thinking)
}

func mergeUsage(stats *config.ProxyStats, usage UsageSnapshot) {
	if stats == nil {
		return
	}
	if usage.PromptTokens > 0 {
		stats.PromptTokens = usage.PromptTokens
	}
	if usage.CompletionTokens > 0 {
		stats.CompletionTokens = usage.CompletionTokens
	}
	if usage.TotalTokens > 0 {
		stats.TotalTokens = usage.TotalTokens
	}
}

func deduplicateToolUses(toolUses []provider.ToolUse) []provider.ToolUse {
	if len(toolUses) == 0 {
		return nil
	}

	byID := make(map[string]provider.ToolUse)
	orderedIDs := make([]string, 0, len(toolUses))
	withoutID := make([]provider.ToolUse, 0)
	for _, toolUse := range toolUses {
		toolID := strings.TrimSpace(toolUse.ID)
		if toolID == "" {
			withoutID = append(withoutID, toolUse)
			continue
		}
		existing, ok := byID[toolID]
		if !ok {
			orderedIDs = append(orderedIDs, toolID)
			byID[toolID] = toolUse
			continue
		}
		if toolUsePayloadSize(toolUse) > toolUsePayloadSize(existing) {
			byID[toolID] = toolUse
		}
	}

	candidates := make([]provider.ToolUse, 0, len(toolUses))
	for _, toolID := range orderedIDs {
		candidates = append(candidates, byID[toolID])
	}
	candidates = append(candidates, withoutID...)

	seen := make(map[string]struct{}, len(candidates))
	unique := make([]provider.ToolUse, 0, len(candidates))
	for _, toolUse := range candidates {
		key := strings.TrimSpace(toolUse.Name) + "|" + marshalToolInput(toolUse.Input)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, toolUse)
	}
	return unique
}

func toolUsePayloadSize(toolUse provider.ToolUse) int {
	return len(marshalToolInput(toolUse.Input))
}

func marshalToolInput(input map[string]any) string {
	encoded, err := json.Marshal(defaultIfNilMap(input))
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func extractBracketToolUses(text string) ([]provider.ToolUse, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || (!strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "{")) {
		return nil, false
	}

	var rawItems []any
	if strings.HasPrefix(trimmed, "{") {
		rawItems = []any{map[string]any{}}
		if err := json.Unmarshal([]byte(trimmed), &rawItems[0]); err != nil {
			return nil, false
		}
	} else if err := json.Unmarshal([]byte(trimmed), &rawItems); err != nil {
		return nil, false
	}

	toolUses := make([]provider.ToolUse, 0, len(rawItems))
	for _, item := range rawItems {
		toolUse, ok := bracketToolUse(item)
		if !ok {
			return nil, false
		}
		toolUses = append(toolUses, toolUse)
	}
	if len(toolUses) == 0 {
		return nil, false
	}
	return toolUses, true
}

func bracketToolUse(item any) (provider.ToolUse, bool) {
	object, ok := item.(map[string]any)
	if !ok {
		return provider.ToolUse{}, false
	}

	name := strings.TrimSpace(asString(object["name"]))
	arguments := anyToMap(object["input"])
	if function, ok := object["function"].(map[string]any); ok {
		if name == "" {
			name = strings.TrimSpace(asString(function["name"]))
		}
		if len(arguments) == 0 {
			arguments = anyToMap(function["arguments"])
			if len(arguments) == 0 {
				arguments = parseToolArguments(asString(function["arguments"]))
			}
		}
	}
	if len(arguments) == 0 {
		arguments = anyToMap(object["arguments"])
		if len(arguments) == 0 {
			arguments = parseToolArguments(asString(object["arguments"]))
		}
	}
	if name == "" {
		return provider.ToolUse{}, false
	}
	return provider.ToolUse{
		ID:    strings.TrimSpace(util.FirstNonEmpty(asString(object["id"]), asString(object["toolUseId"]), asString(object["call_id"]))),
		Name:  name,
		Input: defaultIfNilMap(arguments),
	}, true
}

type StreamParser struct {
	reader           io.Reader
	assistantContent string
	thinkingContent  string
	currentTool      *toolAccumulator
	toolUses         []provider.ToolUse
	pendingEvents    []StreamEvent
	finalized        bool
	fallbackParser   *assistantFallbackParser
	metadataFilter   *streamMetadataFilter
}

type eventFrame struct {
	EventType   string
	MessageType string
	Payload     []byte
}

type toolAccumulator struct {
	ID         string
	Name       string
	InputParts strings.Builder
	HasInput   bool
}

type streamMetadataFilter struct {
	inMetadata bool
	pending    string
}

func newStreamMetadataFilter() *streamMetadataFilter {
	return &streamMetadataFilter{}
}

func (f *streamMetadataFilter) Feed(delta string) string {
	if f == nil || delta == "" {
		return delta
	}
	f.pending += delta
	return f.drain(false)
}

func (f *streamMetadataFilter) Finalize() string {
	if f == nil {
		return ""
	}
	return f.drain(true)
}

func (f *streamMetadataFilter) drain(final bool) string {
	if f == nil {
		return ""
	}
	const openTag = "<environment_details>"
	const closeTag = "</environment_details>"

	var out strings.Builder
	for {
		if f.inMetadata {
			idx := strings.Index(f.pending, closeTag)
			if idx >= 0 {
				f.pending = f.pending[idx+len(closeTag):]
				f.inMetadata = false
				continue
			}
			if final {
				f.pending = ""
			}
			return out.String()
		}

		idx := strings.Index(f.pending, openTag)
		if idx >= 0 {
			out.WriteString(f.pending[:idx])
			f.pending = f.pending[idx+len(openTag):]
			f.inMetadata = true
			continue
		}

		if final {
			out.WriteString(f.pending)
			f.pending = ""
			return out.String()
		}

		safeLen := len(f.pending) - len(openTag) + 1
		if safeLen > 0 {
			safeLen = utf8SafePrefixLength(f.pending, safeLen)
			if safeLen > 0 {
				out.WriteString(f.pending[:safeLen])
				f.pending = f.pending[safeLen:]
			}
		}
		return out.String()
	}
}

func utf8SafePrefixLength(text string, limit int) int {
	if limit <= 0 {
		return 0
	}
	if limit >= len(text) {
		return len(text)
	}
	for limit > 0 && !utf8.ValidString(text[:limit]) {
		limit--
	}
	return limit
}

func NewStreamParser(reader io.Reader) *StreamParser {
	return NewStreamParserWithFallbackTags(reader, nil, false)
}

func NewStreamParserWithFallbackTags(reader io.Reader, fallbackTags []string, enableFallback bool) *StreamParser {
	parser := &StreamParser{reader: reader, metadataFilter: newStreamMetadataFilter()}
	if enableFallback {
		parser.fallbackParser = newAssistantFallbackParser(fallbackTags)
	}
	return parser
}

func (p *StreamParser) Next() (StreamEvent, error) {
	if len(p.pendingEvents) > 0 {
		event := p.pendingEvents[0]
		p.pendingEvents = p.pendingEvents[1:]
		return event, nil
	}
	for {
		frame, err := readEventFrame(p.reader)
		if err != nil {
			if err == io.EOF {
				if !p.finalized {
					p.finalized = true
					p.finalizeCurrentTool()
					if p.metadataFilter != nil {
						if flushed := p.metadataFilter.Finalize(); flushed != "" {
							p.pendingEvents = append(p.pendingEvents, p.emitAssistantResponseDelta(flushed)...)
						}
					}
					if p.fallbackParser != nil {
						p.pendingEvents = append(p.pendingEvents, p.fallbackParser.Finalize()...)
					}
				}
				if len(p.pendingEvents) > 0 {
					event := p.pendingEvents[0]
					p.pendingEvents = p.pendingEvents[1:]
					return event, nil
				}
			}
			return StreamEvent{}, err
		}
		events, err := p.parseFrame(frame)
		if err != nil {
			return StreamEvent{}, err
		}
		if len(events) == 0 {
			continue
		}
		p.pendingEvents = append(p.pendingEvents, events...)
		if len(p.pendingEvents) > 0 {
			event := p.pendingEvents[0]
			p.pendingEvents = p.pendingEvents[1:]
			return event, nil
		}
	}
}

func (p *StreamParser) ToolUses() []provider.ToolUse {
	p.finalizeCurrentTool()
	return append([]provider.ToolUse(nil), p.toolUses...)
}

func (p *StreamParser) FallbackThinkingCandidate() providerthinking.Candidate {
	if p == nil || p.fallbackParser == nil {
		return providerthinking.Candidate{}
	}
	return p.fallbackParser.ParsedCandidate()
}

func (p *StreamParser) parseFrame(frame eventFrame) ([]StreamEvent, error) {
	if strings.EqualFold(frame.MessageType, "error") || strings.EqualFold(frame.MessageType, "exception") {
		return nil, fmt.Errorf(errorMessageFromPayload(frame.Payload))
	}
	if len(frame.Payload) == 0 {
		return nil, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, nil
	}
	usage := extractUsage(payload)

	switch resolveEventType(frame.EventType, payload) {
	case "assistantResponseEvent":
		delta := deltaFromCumulative(&p.assistantContent, resolveTextField(payload, "content", "text"))
		if p.metadataFilter != nil {
			delta = p.metadataFilter.Feed(delta)
		}
		return attachUsageEvents(p.emitAssistantResponseDelta(delta), usage), nil
	case "reasoningContentEvent":
		return attachUsageEvents(singleEvent(StreamEvent{Thinking: deltaFromCumulative(&p.thinkingContent, resolveTextField(payload, "text", "content"))}), usage), nil
	case "toolUseEvent":
		p.handleToolUseEvent(payload)
		return attachUsageEvents(nil, usage), nil
	default:
		return attachUsageEvents(nil, usage), nil
	}
}

func (p *StreamParser) emitAssistantResponseDelta(delta string) []StreamEvent {
	if delta == "" {
		return nil
	}
	if p == nil || p.fallbackParser == nil || !p.fallbackParser.Enabled() {
		return singleEvent(StreamEvent{Text: delta})
	}
	return p.fallbackParser.Feed(delta)
}

func singleEvent(event StreamEvent) []StreamEvent {
	if event.Text == "" && event.Thinking == "" && event.Usage.TotalTokens == 0 && event.Usage.PromptTokens == 0 && event.Usage.CompletionTokens == 0 {
		return nil
	}
	return []StreamEvent{event}
}

func attachUsageEvents(events []StreamEvent, usage UsageSnapshot) []StreamEvent {
	if usage.TotalTokens == 0 && usage.PromptTokens == 0 && usage.CompletionTokens == 0 {
		return events
	}
	if len(events) == 0 {
		return []StreamEvent{{Usage: usage}}
	}
	events[len(events)-1].Usage = usage
	return events
}

func (p *StreamParser) handleToolUseEvent(payload map[string]any) {
	toolID := strings.TrimSpace(resolveTextField(payload, "toolUseId", "id"))
	toolName := strings.TrimSpace(resolveTextField(payload, "name"))
	stop, _ := payload["stop"].(bool)

	if toolID != "" || toolName != "" {
		if p.currentTool != nil && p.currentTool.ID != "" && toolID != "" && p.currentTool.ID != toolID {
			p.finalizeCurrentTool()
		}
		if p.currentTool == nil {
			p.currentTool = &toolAccumulator{}
		}
		if toolID != "" {
			p.currentTool.ID = toolID
		}
		if toolName != "" {
			p.currentTool.Name = toolName
		}
	}

	if p.currentTool != nil {
		switch input := payload["input"].(type) {
		case string:
			if strings.TrimSpace(input) != "" {
				p.currentTool.InputParts.WriteString(input)
				p.currentTool.HasInput = true
			}
		case map[string]any:
			encoded, _ := json.Marshal(input)
			p.currentTool.InputParts.Reset()
			p.currentTool.InputParts.Write(encoded)
			p.currentTool.HasInput = true
		}
	}

	if stop {
		p.finalizeCurrentTool()
	}
}

func (p *StreamParser) finalizeCurrentTool() {
	if p.currentTool == nil {
		return
	}

	toolName := strings.TrimSpace(p.currentTool.Name)
	if toolName != "" {
		toolUse := provider.ToolUse{
			ID:    strings.TrimSpace(p.currentTool.ID),
			Name:  toolName,
			Input: map[string]any{},
		}
		if p.currentTool.HasInput {
			toolUse.Input = parseToolArguments(p.currentTool.InputParts.String())
		}
		p.toolUses = append(p.toolUses, toolUse)
	}
	p.currentTool = nil
}

func readEventFrame(reader io.Reader) (eventFrame, error) {
	var prelude [12]byte
	if _, err := io.ReadFull(reader, prelude[:]); err != nil {
		return eventFrame{}, err
	}

	totalLen := int(binary.BigEndian.Uint32(prelude[0:4]))
	headersLen := int(binary.BigEndian.Uint32(prelude[4:8]))
	if totalLen < 16 {
		return eventFrame{}, fmt.Errorf("invalid AWS event-stream frame length: %d", totalLen)
	}
	if binary.BigEndian.Uint32(prelude[8:12]) != crc32.ChecksumIEEE(prelude[:8]) {
		return eventFrame{}, fmt.Errorf("invalid AWS event-stream prelude CRC")
	}

	remaining := make([]byte, totalLen-12)
	if _, err := io.ReadFull(reader, remaining); err != nil {
		return eventFrame{}, err
	}
	if crc32.ChecksumIEEE(append(prelude[:], remaining[:len(remaining)-4]...)) != binary.BigEndian.Uint32(remaining[len(remaining)-4:]) {
		return eventFrame{}, fmt.Errorf("invalid AWS event-stream message CRC")
	}
	if headersLen > len(remaining)-4 {
		return eventFrame{}, fmt.Errorf("invalid AWS event-stream headers length")
	}

	headers := remaining[:headersLen]
	payload := remaining[headersLen : len(remaining)-4]
	frame := eventFrame{Payload: payload}
	frame.EventType, frame.MessageType = parseEventHeaders(headers)
	return frame, nil
}

func parseEventHeaders(headers []byte) (string, string) {
	offset := 0
	eventType := ""
	messageType := ""
	for offset < len(headers) {
		nameLen := int(headers[offset])
		offset++
		if offset+nameLen > len(headers) || offset >= len(headers) {
			break
		}
		name := string(headers[offset : offset+nameLen])
		offset += nameLen
		valueType := headers[offset]
		offset++
		if valueType != 7 || offset+2 > len(headers) {
			break
		}
		valueLen := int(binary.BigEndian.Uint16(headers[offset : offset+2]))
		offset += 2
		if offset+valueLen > len(headers) {
			break
		}
		value := string(headers[offset : offset+valueLen])
		offset += valueLen
		switch name {
		case ":event-type":
			eventType = value
		case ":message-type":
			messageType = value
		}
	}
	return eventType, messageType
}

func resolveEventType(headerType string, payload map[string]any) string {
	// O18: evaluate TrimSpace once with an if-init statement.
	if t := strings.TrimSpace(headerType); t != "" {
		return t
	}
	if _, ok := payload["toolUseId"]; ok {
		return "toolUseEvent"
	}
	if _, ok := payload["name"]; ok {
		if _, hasInput := payload["input"]; hasInput {
			return "toolUseEvent"
		}
	}
	if _, ok := payload["text"]; ok {
		return "reasoningContentEvent"
	}
	if _, ok := payload["content"]; ok {
		return "assistantResponseEvent"
	}
	return ""
}

func resolveTextField(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := payload[key].(string); ok && text != "" {
			return text
		}
	}
	return ""
}

func deltaFromCumulative(previous *string, current string) string {
	if current == "" {
		return ""
	}
	if previous == nil || *previous == "" {
		if previous != nil {
			*previous = current
		}
		return current
	}
	if current == *previous {
		return ""
	}
	if strings.HasPrefix(current, *previous) {
		delta := strings.TrimPrefix(current, *previous)
		*previous = current
		return delta
	}
	if strings.HasPrefix(*previous, current) {
		return ""
	}
	previousRunes := []rune(*previous)
	currentRunes := []rune(current)
	maxOverlap := 0
	maxLength := len(previousRunes)
	if len(currentRunes) < maxLength {
		maxLength = len(currentRunes)
	}
	for size := maxLength; size > 0; size-- {
		if string(previousRunes[len(previousRunes)-size:]) == string(currentRunes[:size]) {
			maxOverlap = size
			break
		}
	}
	*previous = current
	if maxOverlap > 0 {
		return string(currentRunes[maxOverlap:])
	}
	return current
}

func extractUsage(payload map[string]any) UsageSnapshot {
	// O10: fast-path — skip alloc when none of the usage keys are present.
	_, hasUsage := payload["usage"]
	_, hasTokenUsage := payload["tokenUsage"]
	_, hasToken_Usage := payload["token_usage"]
	if !hasUsage && !hasTokenUsage && !hasToken_Usage {
		return UsageSnapshot{}
	}
	usageMaps := make([]map[string]any, 0, 4)
	collectUsageMaps(payload, &usageMaps)
	usage := UsageSnapshot{}
	for _, item := range usageMaps {
		if item == nil {
			continue
		}
		if value, ok := readTokenNumber(item, "inputTokens", "promptTokens", "input_tokens", "prompt_tokens", "totalInputTokens", "total_input_tokens"); ok {
			usage.PromptTokens = value
		}
		if value, ok := readTokenNumber(item, "outputTokens", "completionTokens", "output_tokens", "completion_tokens", "totalOutputTokens", "total_output_tokens"); ok {
			usage.CompletionTokens = value
		}
		if value, ok := readTokenNumber(item, "totalTokens", "total_tokens"); ok {
			usage.TotalTokens = value
		}
	}
	if usage.TotalTokens == 0 && (usage.PromptTokens > 0 || usage.CompletionTokens > 0) {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return usage
}

func collectUsageMaps(value any, usageMaps *[]map[string]any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			// O16: check canonical forms first; only fall through to a lowercased comparison as a defensive fallback.
			match := key == "usage" || key == "tokenUsage" || key == "token_usage"
			if !match {
				lowerKey := strings.ToLower(strings.TrimSpace(key))
				match = lowerKey == "usage" || lowerKey == "tokenusage" || lowerKey == "token_usage"
			}
			if match {
				if usage, ok := child.(map[string]any); ok {
					*usageMaps = append(*usageMaps, usage)
				}
			}
			collectUsageMaps(child, usageMaps)
		}
	case []any:
		for _, child := range typed {
			collectUsageMaps(child, usageMaps)
		}
	}
}

func readTokenNumber(values map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed), true
		case int:
			return typed, true
		case int64:
			return int(typed), true
		case json.Number:
			parsed, err := typed.Int64()
			if err == nil {
				return int(parsed), true
			}
		case string:
			var parsed int
			_, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed)
			if err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}

func errorMessageFromPayload(payload []byte) string {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" {
		return "upstream stream error"
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return trimmed
	}
	for _, key := range []string{"message", "Message", "errorMessage"} {
		if value, ok := decoded[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return trimmed
}

func estimateUsageIfMissing(stats *config.ProxyStats, req provider.ChatRequest, outcome *provider.CompletionOutcome) {
	if stats == nil || outcome == nil {
		return
	}
	if stats.PromptTokens <= 0 {
		stats.PromptTokens = estimatePromptTokens(req)
	}
	if stats.CompletionTokens <= 0 {
		stats.CompletionTokens = estimateCompletionTokens(*outcome)
	}
	if stats.TotalTokens <= 0 {
		stats.TotalTokens = stats.PromptTokens + stats.CompletionTokens
	}
	outcome.Usage = *stats
}

func estimatePromptTokens(req provider.ChatRequest) int {
	parts := make([]string, 0, len(req.Messages)+(len(req.Tools)*2)+4)
	if model := strings.TrimSpace(req.Model); model != "" {
		parts = append(parts, model)
	}
	for _, message := range req.Messages {
		if role := strings.TrimSpace(message.Role); role != "" {
			parts = append(parts, role)
		}
		if name := strings.TrimSpace(message.Name); name != "" {
			parts = append(parts, name)
		}
		if text := sanitizePromptText(messageTextContent(message.Content)); text != "" {
			parts = append(parts, text)
		}
		for _, toolCall := range message.ToolCalls {
			parts = append(parts, strings.TrimSpace(toolCall.Function.Name), strings.TrimSpace(toolCall.Function.Arguments))
		}
		if toolCallID := strings.TrimSpace(message.ToolCallID); toolCallID != "" {
			parts = append(parts, toolCallID)
		}
	}
	for _, tool := range provider.NormalizeToolsForProvider("kiro", req.Tools) {
		parts = append(parts, strings.TrimSpace(tool.Function.Name), strings.TrimSpace(tool.Function.Description), marshalAny(tool.Function.Parameters))
	}
	if user := strings.TrimSpace(req.User); user != "" {
		parts = append(parts, user)
	}
	if req.ToolChoice != nil {
		parts = append(parts, marshalAny(req.ToolChoice))
	}
	return estimateTokenText(strings.Join(nonEmptyStrings(parts), "\n"))
}

func estimateCompletionTokens(outcome provider.CompletionOutcome) int {
	parts := []string{
		sanitizeModelOutputText(outcome.Thinking),
		sanitizeModelOutputText(outcome.Text),
	}
	for _, toolUse := range outcome.ToolUses {
		parts = append(parts, strings.TrimSpace(toolUse.Name), marshalAny(toolUse.Input))
	}
	return estimateTokenText(strings.Join(nonEmptyStrings(parts), "\n"))
}

func estimateTokenText(text string) int {
	runeCount := len([]rune(strings.TrimSpace(text)))
	if runeCount <= 0 {
		return 0
	}
	estimated := runeCount / 4
	if estimated <= 0 {
		estimated = 1
	}
	return estimated
}

func marshalAny(value any) string {
	if value == nil {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func nonEmptyStrings(parts []string) []string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		// O17: compute TrimSpace once instead of twice per element.
		if t := strings.TrimSpace(part); t != "" {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
