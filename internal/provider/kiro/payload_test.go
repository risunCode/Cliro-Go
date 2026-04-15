package kiro

import (
	"testing"

	"cliro/internal/config"
	models "cliro/internal/proxy/models"
)

func TestBuildPayloadPreservesMetadataToolsAndImages(t *testing.T) {
	req := models.Request{
		Model:    "claude-sonnet-4.5",
		Thinking: models.ThinkingConfig{Requested: true},
		Metadata: map[string]any{"conversationId": "conv-1", "profileArn": "arn:test"},
		Tools:    []models.Tool{{Name: "search", Description: "Search", Schema: map[string]any{"type": "object"}}},
		Messages: []models.Message{{
			Role: models.RoleUser,
			Content: []models.ContentBlock{
				models.TextBlock("hello"),
				models.ImageDataBlock("image/png", "ZmFrZQ=="),
			},
		}},
	}
	account := config.Account{ID: "acc-1", AccountID: "arn:acct"}
	payload, err := BuildPayload(req, account)
	if err != nil {
		t.Fatalf("BuildPayload error: %v", err)
	}
	if payload.ConversationState.ConversationID != "conv-1" {
		t.Fatalf("conversation id = %q", payload.ConversationState.ConversationID)
	}
	if payload.ProfileArn != "arn:test" {
		t.Fatalf("profile arn = %q", payload.ProfileArn)
	}
	userInput := payload.ConversationState.CurrentMessage["userInputMessage"].(map[string]any)
	if _, ok := userInput["images"]; !ok {
		t.Fatalf("expected images in payload")
	}
	ctx := userInput["userInputMessageContext"].(map[string]any)
	if _, ok := ctx["tools"]; !ok {
		t.Fatalf("expected tools in payload context")
	}
}

func TestBuildPayloadPreservesToolResultsAndSystemHistory(t *testing.T) {
	req := models.Request{
		Model: "claude-sonnet-4.5",
		Messages: []models.Message{
			{Role: models.RoleSystem, Content: "system prompt"},
			{Role: models.RoleAssistant, ToolCalls: []models.ToolCall{{ID: "toolu_1", Name: "search", Arguments: `{"q":"golang"}`}}},
			{Role: models.RoleTool, ToolCallID: "toolu_1", Content: []models.ContentBlock{models.ToolResultContentBlock("toolu_1", "done", false, nil)}},
			{Role: models.RoleUser, Content: "next"},
		},
	}
	payload, err := BuildPayload(req, config.Account{ID: "acc-1"})
	if err != nil {
		t.Fatalf("BuildPayload error: %v", err)
	}
	if len(payload.ConversationState.History) == 0 {
		t.Fatalf("expected history entries")
	}
	first := payload.ConversationState.History[0]["userInputMessage"].(map[string]any)
	if first["content"] == "" {
		t.Fatalf("expected system prompt content in history")
	}
	current := payload.ConversationState.CurrentMessage["userInputMessage"].(map[string]any)
	ctx := current["userInputMessageContext"].(map[string]any)
	if got := len(ctx["toolResults"].([]any)); got < 1 {
		t.Fatalf("toolResults = %d", got)
	}
}
