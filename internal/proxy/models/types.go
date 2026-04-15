package models

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type Provider string

type Protocol string

const (
	ProtocolOpenAI    Protocol = "openai"
	ProtocolAnthropic Protocol = "anthropic"
)

type Endpoint string

const (
	EndpointOpenAIResponses      Endpoint = "openai_responses"
	EndpointOpenAIChat           Endpoint = "openai_chat"
	EndpointOpenAICompletions    Endpoint = "openai_completions"
	EndpointAnthropicMessages    Endpoint = "anthropic_messages"
	EndpointAnthropicCountTokens Endpoint = "anthropic_count_tokens"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ThinkingConfig struct {
	Requested bool
	RawParams map[string]any
}

type Request struct {
	Protocol    Protocol
	Endpoint    Endpoint
	Model       string
	Thinking    ThinkingConfig
	Messages    []Message
	Stream      bool
	Temperature *float64
	TopP        *float64
	MaxTokens   *int
	Tools       []Tool
	ToolChoice  any
	User        string
	Metadata    map[string]any
}

type Message struct {
	Role           Role
	Content        any
	Name           string
	ToolCalls      []ToolCall
	ToolCallID     string
	ThinkingBlocks []ThinkingBlock
}

type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeImage      ContentType = "image"
	ContentTypeToolResult ContentType = "tool_result"
	ContentTypeThinking   ContentType = "thinking"
)

type ContentBlock struct {
	Type       ContentType
	Text       string
	Image      *ImageBlock
	ToolResult *ToolResultBlock
	Thinking   *ThinkingBlock
}

type ImageBlock struct {
	MediaType string
	Data      string
	URL       string
}

type ToolResultBlock struct {
	ToolCallID string
	Content    string
	IsError    bool
	Images     []ImageBlock
}

type ThinkingBlock struct {
	Thinking  string
	Signature string
}

func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: ContentTypeText, Text: strings.TrimSpace(text)}
}
func ThinkingContentBlock(thinking string, signature string) ContentBlock {
	return ContentBlock{Type: ContentTypeThinking, Thinking: &ThinkingBlock{Thinking: strings.TrimSpace(thinking), Signature: strings.TrimSpace(signature)}}
}
func ImageDataBlock(mediaType string, data string) ContentBlock {
	return ContentBlock{Type: ContentTypeImage, Image: &ImageBlock{MediaType: strings.TrimSpace(mediaType), Data: strings.TrimSpace(data)}}
}
func ImageURLBlock(mediaType string, rawURL string) ContentBlock {
	return ContentBlock{Type: ContentTypeImage, Image: &ImageBlock{MediaType: strings.TrimSpace(mediaType), URL: strings.TrimSpace(rawURL)}}
}
func ToolResultContentBlock(toolCallID string, content string, isError bool, images []ImageBlock) ContentBlock {
	return ContentBlock{Type: ContentTypeToolResult, ToolResult: &ToolResultBlock{ToolCallID: strings.TrimSpace(toolCallID), Content: content, IsError: isError, Images: append([]ImageBlock(nil), images...)}}
}

func ContentEmpty(content any) bool {
	return strings.TrimSpace(ContentText(content)) == "" && len(ContentImages(content)) == 0 && len(ContentToolResults(content)) == 0 && len(ContentThinkingBlocks(content)) == 0
}

func ContentText(content any) string {
	switch typed := content.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []ContentBlock:
		parts := make([]string, 0, len(typed))
		for _, block := range typed {
			switch block.Type {
			case ContentTypeText:
				if strings.TrimSpace(block.Text) != "" {
					parts = append(parts, block.Text)
				}
			case ContentTypeThinking:
				if block.Thinking != nil && strings.TrimSpace(block.Thinking.Thinking) != "" {
					parts = append(parts, block.Thinking.Thinking)
				}
			case ContentTypeToolResult:
				if block.ToolResult != nil && strings.TrimSpace(block.ToolResult.Content) != "" {
					parts = append(parts, block.ToolResult.Content)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := ContentText(item); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case map[string]any:
		for _, key := range []string{"text", "content", "thinking", "refusal"} {
			if value, ok := typed[key]; ok {
				if text := ContentText(value); strings.TrimSpace(text) != "" {
					return text
				}
			}
		}
		return ""
	default:
		return ""
	}
}

func ContentImages(content any) []ImageBlock {
	switch typed := content.(type) {
	case []ContentBlock:
		images := make([]ImageBlock, 0)
		for _, block := range typed {
			if block.Type == ContentTypeImage && block.Image != nil {
				images = append(images, *block.Image)
			}
			if block.Type == ContentTypeToolResult && block.ToolResult != nil && len(block.ToolResult.Images) > 0 {
				images = append(images, block.ToolResult.Images...)
			}
		}
		return images
	case []any:
		images := make([]ImageBlock, 0)
		for _, item := range typed {
			images = append(images, ContentImages(item)...)
		}
		return images
	default:
		return nil
	}
}

func ContentToolResults(content any) []ToolResultBlock {
	switch typed := content.(type) {
	case []ContentBlock:
		results := make([]ToolResultBlock, 0)
		for _, block := range typed {
			if block.Type == ContentTypeToolResult && block.ToolResult != nil {
				results = append(results, *block.ToolResult)
			}
		}
		return results
	case []any:
		results := make([]ToolResultBlock, 0)
		for _, item := range typed {
			results = append(results, ContentToolResults(item)...)
		}
		return results
	default:
		return nil
	}
}

func ContentThinkingBlocks(content any) []ThinkingBlock {
	switch typed := content.(type) {
	case []ContentBlock:
		blocks := make([]ThinkingBlock, 0)
		for _, block := range typed {
			if block.Type == ContentTypeThinking && block.Thinking != nil {
				blocks = append(blocks, *block.Thinking)
			}
		}
		return blocks
	case []any:
		blocks := make([]ThinkingBlock, 0)
		for _, item := range typed {
			blocks = append(blocks, ContentThinkingBlocks(item)...)
		}
		return blocks
	default:
		return nil
	}
}

func StableThinkingSignature(thinking string) string {
	trimmed := strings.TrimSpace(thinking)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return "sig_" + hex.EncodeToString(sum[:])
}

type Tool struct {
	Type        string
	Name        string
	Description string
	Schema      any
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type Response struct {
	ID                string
	Model             string
	Text              string
	Thinking          string
	ThinkingSignature string
	ThinkingSource    string
	ToolCalls         []ToolCall
	Usage             Usage
	StopReason        string
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	InputTokens      int
	OutputTokens     int
}

type Event struct {
	Type           string
	TextDelta      string
	ThinkDelta     string
	SignatureDelta string
	ToolDelta      any
	Done           bool
}
