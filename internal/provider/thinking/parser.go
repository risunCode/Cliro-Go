package thinking

import "strings"

const defaultMaxLeadingBytes = 256
const defaultMaxThinkingBytes = 64 * 1024

type ParseResult struct {
	Thinking string
	Text     string
}

type LeadingParser struct {
	state           parserState
	openTags        []string
	maxLeadingBytes int
	leadingBuffer   string
	thinkingBuffer  string
	literalPrefix   string
	closeTag        string
	parsed          bool
}

type parserState int

const (
	stateLeading parserState = iota
	stateThinking
	stateText
)

func NewLeadingParser(openTags []string, maxLeadingBytes int) *LeadingParser {
	normalized := normalizeOpenTags(openTags)
	if len(normalized) == 0 {
		normalized = []string{"<thinking>", "<think>"}
	}
	if maxLeadingBytes <= 0 {
		maxLeadingBytes = defaultMaxLeadingBytes
	}

	return &LeadingParser{
		state:           stateLeading,
		openTags:        normalized,
		maxLeadingBytes: maxLeadingBytes,
	}
}

func (p *LeadingParser) Feed(chunk string) ParseResult {
	switch p.state {
	case stateLeading:
		return p.feedLeading(chunk)
	case stateThinking:
		return p.feedThinking(chunk)
	default:
		return ParseResult{Text: chunk}
	}
}

func (p *LeadingParser) Finalize() ParseResult {
	switch p.state {
	case stateLeading:
		text := p.leadingBuffer
		p.leadingBuffer = ""
		p.state = stateText
		return ParseResult{Text: text}
	case stateThinking:
		text := p.literalPrefix + p.thinkingBuffer
		p.literalPrefix = ""
		p.thinkingBuffer = ""
		p.closeTag = ""
		p.state = stateText
		p.parsed = false
		return ParseResult{Text: text}
	default:
		return ParseResult{}
	}
}

func (p *LeadingParser) Parsed() bool {
	return p.parsed
}

func (p *LeadingParser) feedLeading(chunk string) ParseResult {
	p.leadingBuffer += chunk
	if len(p.leadingBuffer) > p.maxLeadingBytes {
		return p.flushLeadingAsText()
	}

	trimmed := strings.TrimLeft(p.leadingBuffer, " \t\r\n")
	leadingLen := len(p.leadingBuffer) - len(trimmed)
	for _, openTag := range p.openTags {
		if strings.HasPrefix(trimmed, openTag) {
			p.literalPrefix = p.leadingBuffer[:leadingLen] + openTag
			p.closeTag = closeTagFor(openTag)
			p.leadingBuffer = ""
			p.state = stateThinking
			return p.feedThinking(trimmed[len(openTag):])
		}
	}

	if couldBeOpenTagPrefix(trimmed, p.openTags) {
		return ParseResult{}
	}

	return p.flushLeadingAsText()
}

func (p *LeadingParser) feedThinking(chunk string) ParseResult {
	p.thinkingBuffer += chunk
	if closeIdx := strings.Index(p.thinkingBuffer, p.closeTag); closeIdx >= 0 {
		thinkingText := p.thinkingBuffer[:closeIdx]
		text := p.thinkingBuffer[closeIdx+len(p.closeTag):]
		p.thinkingBuffer = ""
		p.literalPrefix = ""
		p.closeTag = ""
		p.state = stateText
		p.parsed = true
		return ParseResult{Thinking: thinkingText, Text: text}
	}
	if len(p.literalPrefix)+len(p.thinkingBuffer) > defaultMaxThinkingBytes {
		text := p.literalPrefix + p.thinkingBuffer
		p.literalPrefix = ""
		p.thinkingBuffer = ""
		p.closeTag = ""
		p.state = stateText
		p.parsed = false
		return ParseResult{Text: text}
	}
	return ParseResult{}
}

func (p *LeadingParser) flushLeadingAsText() ParseResult {
	text := p.leadingBuffer
	p.leadingBuffer = ""
	p.state = stateText
	return ParseResult{Text: text}
}

func normalizeOpenTags(openTags []string) []string {
	seen := make(map[string]struct{}, len(openTags))
	normalized := make([]string, 0, len(openTags))
	for _, tag := range openTags {
		tag = strings.TrimSpace(tag)
		if tag == "" || !strings.HasPrefix(tag, "<") || !strings.HasSuffix(tag, ">") || strings.HasPrefix(tag, "</") {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	return normalized
}

func closeTagFor(openTag string) string {
	return "</" + strings.TrimPrefix(openTag, "<")
}

func couldBeOpenTagPrefix(text string, openTags []string) bool {
	if text == "" {
		return true
	}
	for _, tag := range openTags {
		if len(text) <= len(tag) && strings.HasPrefix(tag, text) {
			return true
		}
	}
	return false
}
