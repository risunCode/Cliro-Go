package kiro

import (
	contract "cliro/internal/contract"
	providerthinking "cliro/internal/provider/thinking"
	"strings"
)

type thinkingTag struct {
	Open  string
	Close string
}

type assistantFallbackParser struct {
	tags                          []thinkingTag
	maxOpenLen                    int
	buffer                        string
	pendingTextBeforeThinking     string
	inThinking                    bool
	thinkingExtracted             bool
	activeTag                     thinkingTag
	stripThinkingLeadingNewline   bool
	stripTextLeadingNewlinesAfter bool
	thinkingBuilder               strings.Builder
}

func newAssistantFallbackParser(tags []string) *assistantFallbackParser {
	normalized := normalizeThinkingTags(tags)
	if len(normalized) == 0 {
		return nil
	}
	return &assistantFallbackParser{tags: normalized, maxOpenLen: maxThinkingOpenTagLength(normalized)}
}

func (p *assistantFallbackParser) Enabled() bool {
	return p != nil && len(p.tags) > 0
}

func (p *assistantFallbackParser) Feed(delta string) []StreamEvent {
	if p == nil || delta == "" {
		if delta == "" {
			return nil
		}
		return []StreamEvent{{Text: delta}}
	}
	p.buffer += delta
	return p.drain(false)
}

func (p *assistantFallbackParser) Finalize() []StreamEvent {
	if p == nil {
		return nil
	}
	return p.drain(true)
}

func (p *assistantFallbackParser) ParsedCandidate() providerthinking.Candidate {
	if p == nil {
		return providerthinking.Candidate{}
	}
	thinking := sanitizeModelOutputText(p.thinkingBuilder.String())
	if thinking == "" {
		return providerthinking.Candidate{}
	}
	return providerthinking.Candidate{
		Thinking:  thinking,
		Signature: contract.StableThinkingSignature(thinking),
	}
}

func (p *assistantFallbackParser) drain(final bool) []StreamEvent {
	if p == nil {
		return nil
	}
	events := make([]StreamEvent, 0)
	appendText := func(text string) {
		if text == "" {
			return
		}
		events = append(events, StreamEvent{Text: text})
	}
	appendThinking := func(text string) {
		if text == "" {
			return
		}
		p.thinkingBuilder.WriteString(text)
	}

	for {
		switch {
		case !p.inThinking && !p.thinkingExtracted:
			startPos, tag, ok := findEarliestThinkingStart(p.buffer, p.tags)
			if ok {
				before := p.buffer[:startPos]
				combined := p.pendingTextBeforeThinking + before
				if !isWhitespaceOnly(combined) {
					appendText(combined)
				}
				p.pendingTextBeforeThinking = ""
				p.buffer = p.buffer[startPos+len(tag.Open):]
				p.activeTag = tag
				p.inThinking = true
				p.stripThinkingLeadingNewline = true
				continue
			}

			safeLen := 0
			if final {
				safeLen = len(p.buffer)
			} else if len(p.buffer) > p.maxOpenLen {
				safeLen = len(p.buffer) - p.maxOpenLen
			}
			if safeLen > 0 {
				safeText := p.buffer[:safeLen]
				p.buffer = p.buffer[safeLen:]
				if isWhitespaceOnly(safeText) {
					remaining := 1024 - len(p.pendingTextBeforeThinking)
					if remaining > 0 {
						p.pendingTextBeforeThinking += safeText[:minInt(len(safeText), remaining)]
					}
				} else {
					combined := p.pendingTextBeforeThinking + safeText
					p.pendingTextBeforeThinking = ""
					appendText(combined)
				}
			}
			if final {
				combined := p.pendingTextBeforeThinking + p.buffer
				p.pendingTextBeforeThinking = ""
				p.buffer = ""
				if !isWhitespaceOnly(combined) {
					appendText(combined)
				}
			}
			return events

		case p.inThinking:
			if p.stripThinkingLeadingNewline {
				switch {
				case strings.HasPrefix(p.buffer, "\r\n"):
					p.buffer = p.buffer[2:]
					p.stripThinkingLeadingNewline = false
				case strings.HasPrefix(p.buffer, "\n"):
					p.buffer = p.buffer[1:]
					p.stripThinkingLeadingNewline = false
				case p.buffer != "":
					p.stripThinkingLeadingNewline = false
				}
			}

			// Use enhanced end tag detection that checks for \n\n after tag
			endPos := findRealThinkingEndTag(p.buffer, p.activeTag.Close, 0)
			if endPos != -1 {
				appendThinking(p.buffer[:endPos])
				p.buffer = p.buffer[endPos+len(p.activeTag.Close):]
				p.inThinking = false
				p.thinkingExtracted = true
				p.activeTag = thinkingTag{}
				p.stripThinkingLeadingNewline = false
				p.stripTextLeadingNewlinesAfter = true
				continue
			}

			safeLen := 0
			if final {
				safeLen = len(p.buffer)
			} else if len(p.buffer) > len(p.activeTag.Close) {
				safeLen = len(p.buffer) - len(p.activeTag.Close)
			}
			if safeLen > 0 {
				appendThinking(p.buffer[:safeLen])
				p.buffer = p.buffer[safeLen:]
			}
			if final {
				appendThinking(p.buffer)
				p.buffer = ""
				p.inThinking = false
				p.thinkingExtracted = true
				p.activeTag = thinkingTag{}
				p.stripTextLeadingNewlinesAfter = true
				continue
			}
			return events

		case p.thinkingExtracted:
			rest := p.buffer
			p.buffer = ""
			if p.stripTextLeadingNewlinesAfter {
				rest = trimTextAfterThinking(rest)
				p.stripTextLeadingNewlinesAfter = false
			}
			if !isWhitespaceOnly(rest) {
				appendText(rest)
			}
			if final {
				return events
			}
			return events
		default:
			return events
		}
	}
}

func normalizeThinkingTags(tags []string) []thinkingTag {
	result := make([]thinkingTag, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, raw := range tags {
		tag, ok := normalizeThinkingTag(raw)
		if !ok {
			continue
		}
		key := tag.Open + "|" + tag.Close
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, tag)
	}
	return result
}

func normalizeThinkingTag(tag string) (thinkingTag, bool) {
	trimmed := strings.TrimSpace(tag)
	if !strings.HasPrefix(trimmed, "<") || !strings.HasSuffix(trimmed, ">") || strings.HasPrefix(trimmed, "</") {
		return thinkingTag{}, false
	}
	inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if inner == "" || strings.Contains(inner, " ") {
		return thinkingTag{}, false
	}
	return thinkingTag{Open: "<" + inner + ">", Close: "</" + inner + ">"}, true
}

func maxThinkingOpenTagLength(tags []thinkingTag) int {
	maxLen := 0
	for _, tag := range tags {
		if len(tag.Open) > maxLen {
			maxLen = len(tag.Open)
		}
	}
	return maxLen
}

func findEarliestThinkingStart(buffer string, tags []thinkingTag) (int, thinkingTag, bool) {
	bestPos := -1
	bestTag := thinkingTag{}
	for _, tag := range tags {
		pos := findRealTag(buffer, tag.Open, 0)
		if pos == -1 {
			continue
		}
		if bestPos == -1 || pos < bestPos {
			bestPos = pos
			bestTag = tag
		}
	}
	if bestPos == -1 {
		return -1, thinkingTag{}, false
	}
	return bestPos, bestTag, true
}

func isQuoteCharAt(text string, index int) bool {
	if index < 0 || index >= len(text) {
		return false
	}
	switch text[index] {
	case '"', '\'', '`':
		return true
	default:
		return false
	}
}

func findRealTag(text string, tag string, startIndex int) int {
	searchStart := maxIndex(0, startIndex)
	for {
		pos := strings.Index(text[searchStart:], tag)
		if pos == -1 {
			return -1
		}
		pos += searchStart
		if !isQuoteCharAt(text, pos-1) && !isQuoteCharAt(text, pos+len(tag)) {
			return pos
		}
		searchStart = pos + 1
	}
}

// findRealThinkingEndTag finds a "real" thinking end tag.
// A real end tag is one that is:
//   - at the end of the buffer (no content after it)
//   - followed by only whitespace
//   - followed by \n\n (double newline separator)
//   - followed by visible text (the tag closes thinking and text begins)
//
// We skip the tag only when the character immediately after is a quote character,
// which suggests the model is referencing the tag name inside thinking content.
func findRealThinkingEndTag(text string, tag string, startIndex int) int {
	searchStart := maxIndex(0, startIndex)
	for {
		pos := findRealTag(text, tag, searchStart)
		if pos == -1 {
			return -1
		}
		afterTag := pos + len(tag)
		// At buffer end — definitely the real end tag
		if afterTag >= len(text) {
			return pos
		}
		remaining := text[afterTag:]
		// Followed by \n\n — real end (clean separator)
		if strings.HasPrefix(remaining, "\n\n") || strings.HasPrefix(remaining, "\r\n\r\n") {
			return pos
		}
		// Followed by only whitespace — real end
		if isWhitespaceOnly(remaining) {
			return pos
		}
		// Followed by visible non-whitespace text — the tag ends thinking and
		// text continues immediately (e.g. </thinking>Visible answer).
		// Accept this as a real end tag.
		firstChar := remaining[0]
		if firstChar != '"' && firstChar != '\'' && firstChar != '`' {
			return pos
		}
		searchStart = pos + 1
	}
}

func trimTextAfterThinking(text string) string {
	switch {
	case strings.HasPrefix(text, "\r\n\r\n"):
		return text[4:]
	case strings.HasPrefix(text, "\n\n"):
		return text[2:]
	case strings.HasPrefix(text, "\r\n"):
		return text[2:]
	case strings.HasPrefix(text, "\n"):
		return text[1:]
	default:
		return text
	}
}

func isWhitespaceOnly(text string) bool {
	return strings.TrimSpace(text) == ""
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxIndex(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
