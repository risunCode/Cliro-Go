package models

import "testing"

func TestValidateRequestRejectsAssistantToolResultBlock(t *testing.T) {
	req := Request{
		Messages: []Message{{
			Role:    RoleAssistant,
			Content: []ContentBlock{ToolResultContentBlock("toolu_1", "result", false, nil)},
		}},
	}
	if err := ValidateRequest(req, "kiro"); err == nil {
		t.Fatalf("expected validation error for assistant tool_result block")
	}
}

func TestContentHelpersExtractStructuredBlocks(t *testing.T) {
	content := []ContentBlock{
		TextBlock("hello"),
		ImageDataBlock("image/png", "ZmFrZQ=="),
		ThinkingContentBlock("reason", "sig_123"),
		ToolResultContentBlock("toolu_1", "done", false, nil),
	}
	if got := ContentText(content); got == "" {
		t.Fatalf("expected text extraction")
	}
	if got := len(ContentImages(content)); got != 1 {
		t.Fatalf("images = %d", got)
	}
	if got := len(ContentThinkingBlocks(content)); got != 1 {
		t.Fatalf("thinking blocks = %d", got)
	}
	if got := len(ContentToolResults(content)); got != 1 {
		t.Fatalf("tool results = %d", got)
	}
}

func TestValidateRequestRejectsImageWithoutSource(t *testing.T) {
	req := Request{Messages: []Message{{Role: RoleUser, Content: []ContentBlock{{Type: ContentTypeImage, Image: &ImageBlock{}}}}}}
	if err := ValidateRequest(req, "kiro"); err == nil {
		t.Fatalf("expected validation error for empty image source")
	}
}

func TestValidateRequestRejectsToolRoleWithToolCalls(t *testing.T) {
	req := Request{Messages: []Message{{Role: RoleTool, ToolCallID: "toolu_1", ToolCalls: []ToolCall{{ID: "toolu_1", Name: "bad", Arguments: "{}"}}, Content: "result"}}}
	if err := ValidateRequest(req, "kiro"); err == nil {
		t.Fatalf("expected validation error for tool role with tool calls")
	}
}

func TestValidateRequestAcceptsValidToolRoundTrip(t *testing.T) {
	req := Request{Messages: []Message{{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "toolu_1", Name: "search", Arguments: "{}"}}}, {Role: RoleTool, ToolCallID: "toolu_1", Content: []ContentBlock{ToolResultContentBlock("toolu_1", "done", false, nil)}}}}
	if err := ValidateRequest(req, "kiro"); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
