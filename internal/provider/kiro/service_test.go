package kiro

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"cliro-go/internal/config"
	provider "cliro-go/internal/provider"
)

func TestCompactBody_ExtractsJSONMessage(t *testing.T) {
	body := []byte(`{"message":"The bearer token included in the request is invalid.","reason":null}`)
	if got := compactBody(body); got != "The bearer token included in the request is invalid." {
		t.Fatalf("compactBody = %q", got)
	}
}

func TestBuildRequest_PrimaryEndpointUsesCodeWhispererHeaders(t *testing.T) {
	service := &Service{}
	payload, err := buildPayload(provider.ChatRequest{Model: "claude-sonnet-4.5", Messages: []provider.Message{{Role: "user", Content: "hello"}}}, "claude-sonnet-4.5", false)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	req, err := service.buildRequest(context.Background(), config.Account{AccessToken: "token"}, endpoints[0], payload, provider.ChatRequest{Model: "claude-sonnet-4.5"})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if req.URL.String() != "https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse" {
		t.Fatalf("unexpected primary endpoint url: %q", req.URL.String())
	}
	if req.Header.Get("X-Amz-Target") != "AmazonCodeWhispererStreamingService.GenerateAssistantResponse" {
		t.Fatalf("unexpected primary target: %q", req.Header.Get("X-Amz-Target"))
	}
}

func TestBuildRequest_FallbackEndpointOmitsAmzTarget(t *testing.T) {
	service := &Service{}
	payload, err := buildPayload(provider.ChatRequest{Model: "claude-sonnet-4.5", Messages: []provider.Message{{Role: "user", Content: "hello"}}}, "claude-sonnet-4.5", false)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	req, err := service.buildRequest(context.Background(), config.Account{AccessToken: "token"}, endpoints[1], payload, provider.ChatRequest{Model: "claude-sonnet-4.5"})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if req.URL.String() != "https://q.us-east-1.amazonaws.com/generateAssistantResponse" {
		t.Fatalf("unexpected fallback endpoint url: %q", req.URL.String())
	}
	if req.Header.Get("X-Amz-Target") != "" {
		t.Fatalf("expected fallback to omit X-Amz-Target, got %q", req.Header.Get("X-Amz-Target"))
	}
}

func TestBuildPayload_PreservesConversationMetadata(t *testing.T) {
	payload, err := buildPayload(provider.ChatRequest{
		Model:    "claude-sonnet-4.5",
		Messages: []provider.Message{{Role: "user", Content: "hello"}},
		Metadata: map[string]any{"conversationId": "conv-1", "continuationId": "cont-1"},
	}, "claude-sonnet-4.5", false)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	if payload.ConversationState.ConversationID != "conv-1" {
		t.Fatalf("conversation id = %q", payload.ConversationState.ConversationID)
	}
	if payload.ConversationState.AgentContinuationID != "cont-1" {
		t.Fatalf("continuation id = %q", payload.ConversationState.AgentContinuationID)
	}
}

func TestBuildPayload_UsesMinimalCurrentOriginAndNoHistoryOrigin(t *testing.T) {
	payload, err := buildPayload(provider.ChatRequest{
		Model: "claude-sonnet-4.5",
		Messages: []provider.Message{
			{Role: "system", Content: "follow instructions"},
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "ok"},
			{Role: "user", Content: "second"},
		},
	}, "claude-sonnet-4.5", false)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	if payload.ConversationState.CurrentMessage.UserInputMessage.Origin != kiroConversationOrigin {
		t.Fatalf("current origin = %q", payload.ConversationState.CurrentMessage.UserInputMessage.Origin)
	}
	for _, item := range payload.ConversationState.History {
		if item.UserInputMessage != nil && item.UserInputMessage.Origin != "" {
			t.Fatalf("expected empty history origin, got %q", item.UserInputMessage.Origin)
		}
	}
	encoded, _ := json.Marshal(payload)
	if string(encoded) == "" {
		t.Fatalf("expected encoded payload")
	}
	if string(encoded) != "" && jsonContains(encoded, `"agentTaskType"`) {
		t.Fatalf("unexpected agentTaskType in payload: %s", string(encoded))
	}
}

func TestBuildPayload_PlacesToolsOnlyOnCurrentMessageAndNormalizesSchema(t *testing.T) {
	payload, err := buildPayload(provider.ChatRequest{
		Model:    "claude-sonnet-4.5",
		Messages: []provider.Message{{Role: "user", Content: "use a tool"}},
		Tools:    []provider.Tool{{Type: "function", Function: provider.ToolFunction{Name: "ReadFile", Description: "Read a file", Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}}}},
	}, "claude-sonnet-4.5", false)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	ctx := payload.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
	if ctx == nil || len(ctx.Tools) != 1 {
		t.Fatalf("expected current message tools, got %#v", ctx)
	}
	schema, ok := ctx.Tools[0].ToolSpecification.InputSchema.JSON.(map[string]any)
	if !ok {
		t.Fatalf("expected schema object, got %#v", ctx.Tools[0].ToolSpecification.InputSchema.JSON)
	}
	if _, ok := schema["required"]; !ok {
		t.Fatalf("expected required array in schema: %#v", schema)
	}
	for _, item := range payload.ConversationState.History {
		if item.UserInputMessage != nil && item.UserInputMessage.UserInputMessageContext != nil && len(item.UserInputMessage.UserInputMessageContext.Tools) > 0 {
			t.Fatalf("history should not contain tools: %#v", item.UserInputMessage)
		}
	}
}

func TestShouldUseFakeReasoning_FollowsThinkingSuffixOnly(t *testing.T) {
	if !shouldUseFakeReasoning(true) {
		t.Fatalf("expected thinking suffix to enable fake reasoning fallback")
	}
	if shouldUseFakeReasoning(false) {
		t.Fatalf("expected non-thinking model to disable fake reasoning fallback")
	}
}

func jsonContains(data []byte, fragment string) bool {
	return string(data) != "" && strings.Contains(string(data), fragment)
}
