package kiro

import (
	"strings"
	"testing"

	"cliro-go/internal/config"
	contract "cliro-go/internal/contract"
	provider "cliro-go/internal/provider"
)

func TestBuildRequestPayload_StripsToolContentWhenToolsMissing(t *testing.T) {
	payload, err := buildRequestPayload(provider.ChatRequest{
		Model: "claude-sonnet-4.5",
		Messages: []provider.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "tool_1", Type: "function", Function: provider.ToolCallTarget{Name: "Read", Arguments: `{"path":"README.md"}`}}}},
			{Role: "tool", ToolCallID: "tool_1", Content: "done"},
		},
	}, config.Account{}, config.ThinkingSettings{})
	if err != nil {
		t.Fatalf("buildRequestPayload: %v", err)
	}
	if payload.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext != nil {
		t.Fatalf("expected stripped tool context, got %#v", payload.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext)
	}
	if !strings.Contains(payload.ConversationState.CurrentMessage.UserInputMessage.Content, "[Tool Result (tool_1)]") {
		t.Fatalf("expected tool result text in current content, got %#v", payload.ConversationState.CurrentMessage.UserInputMessage.Content)
	}
	history := payload.ConversationState.History
	if len(history) < 2 || history[1].AssistantResponseMessage == nil {
		t.Fatalf("expected assistant history entry, got %#v", history)
	}
	if !strings.Contains(history[1].AssistantResponseMessage.Content, "[Tool: Read (tool_1)]") {
		t.Fatalf("expected assistant tool call text, got %#v", history[1].AssistantResponseMessage.Content)
	}
}

func TestBuildRequestPayload_ConvertsOrphanToolResultsToText(t *testing.T) {
	payload, err := buildRequestPayload(provider.ChatRequest{
		Model: "claude-sonnet-4.5",
		Tools: []provider.Tool{{Type: "function", Function: provider.ToolFunction{Name: "Read", Parameters: map[string]any{"type": "object"}}}},
		Messages: []provider.Message{{
			Role:    "user",
			Content: []any{map[string]any{"type": "tool_result", "tool_use_id": "tool_1", "content": "done"}},
		}},
	}, config.Account{}, config.ThinkingSettings{})
	if err != nil {
		t.Fatalf("buildRequestPayload: %v", err)
	}
	current := payload.ConversationState.CurrentMessage.UserInputMessage
	if current.UserInputMessageContext == nil || len(current.UserInputMessageContext.Tools) != 1 {
		t.Fatalf("expected tool definitions on current message, got %#v", current.UserInputMessageContext)
	}
	if len(current.UserInputMessageContext.ToolResults) != 0 {
		t.Fatalf("expected orphan tool result to be converted into text, got %#v", current.UserInputMessageContext.ToolResults)
	}
	if !strings.Contains(current.Content, "[Tool Result (tool_1)]") {
		t.Fatalf("expected orphan tool result text, got %#v", current.Content)
	}
}

func TestBuildRequestPayload_EnsuresUserFirstAlternationAndSystemInjection(t *testing.T) {
	payload, err := buildRequestPayload(provider.ChatRequest{
		Model: "claude-sonnet-4.5",
		Messages: []provider.Message{
			{Role: "system", Content: "system context\n<environment_details>\nCurrent time: 2026-03-30T20:43:49+07:00\n</environment_details>"},
			{Role: "assistant", Content: "hello"},
			{Role: "developer", Content: "one"},
			{Role: "user", Content: "two"},
		},
	}, config.Account{}, config.ThinkingSettings{})
	if err != nil {
		t.Fatalf("buildRequestPayload: %v", err)
	}
	history := payload.ConversationState.History
	if len(history) < 4 {
		t.Fatalf("expected normalized history entries, got %#v", history)
	}
	if history[0].UserInputMessage == nil || !strings.Contains(history[0].UserInputMessage.Content, "system context") {
		t.Fatalf("expected system prompt injected into first history user message, got %#v", history[0])
	}
	if strings.Contains(history[0].UserInputMessage.Content, "environment_details") {
		t.Fatalf("expected environment metadata stripped from injected system prompt, got %#v", history[0].UserInputMessage.Content)
	}
	if history[2].UserInputMessage == nil || history[2].UserInputMessage.Content != "one" {
		t.Fatalf("expected normalized developer message in history, got %#v", history[2])
	}
	if history[3].AssistantResponseMessage == nil || history[3].AssistantResponseMessage.Content != "(empty)" {
		t.Fatalf("expected synthetic assistant placeholder, got %#v", history[3])
	}
	if payload.ConversationState.CurrentMessage.UserInputMessage.Content != "two" {
		t.Fatalf("unexpected current content: %#v", payload.ConversationState.CurrentMessage.UserInputMessage.Content)
	}
}

func TestBuildRequestPayload_InsertsEmptyAssistantPlaceholderBetweenAdjacentUsers(t *testing.T) {
	payload, err := buildRequestPayload(provider.ChatRequest{
		Model: "claude-sonnet-4.5",
		Messages: []provider.Message{
			{Role: "assistant", Content: "hello"},
			{Role: "developer", Content: "one"},
			{Role: "user", Content: "three"},
		},
	}, config.Account{}, config.ThinkingSettings{})
	if err != nil {
		t.Fatalf("buildRequestPayload: %v", err)
	}
	history := payload.ConversationState.History
	if len(history) != 4 {
		t.Fatalf("expected normalized history entries, got %#v", history)
	}
	if history[1].AssistantResponseMessage == nil || history[1].AssistantResponseMessage.Content != "hello" {
		t.Fatalf("expected preserved assistant message, got %#v", history[1])
	}
	if history[3].AssistantResponseMessage == nil || history[3].AssistantResponseMessage.Content != "(empty)" {
		t.Fatalf("expected synthetic assistant placeholder, got %#v", history[3])
	}
	if payload.ConversationState.CurrentMessage.UserInputMessage.Content != "three" {
		t.Fatalf("unexpected current content: %#v", payload.ConversationState.CurrentMessage.UserInputMessage.Content)
	}
}

func TestBuildRequestPayload_StripsEnvironmentDetailsFromCurrentUserInput(t *testing.T) {
	payload, err := buildRequestPayload(provider.ChatRequest{
		Model:    "claude-sonnet-4.5",
		Messages: []provider.Message{{Role: "user", Content: "<environment_details>\nCurrent time: 2026-03-30T20:43:49+07:00\n</environment_details>\nhello"}},
	}, config.Account{}, config.ThinkingSettings{})
	if err != nil {
		t.Fatalf("buildRequestPayload: %v", err)
	}
	if payload.ConversationState.CurrentMessage.UserInputMessage.Content != "hello" {
		t.Fatalf("expected stripped current input, got %#v", payload.ConversationState.CurrentMessage.UserInputMessage.Content)
	}
}

func TestBuildRequestPayload_OnlyIncludesProfileARNForDesktopStyleAccounts(t *testing.T) {
	payload, err := buildRequestPayload(provider.ChatRequest{
		Model:    "claude-sonnet-4.5",
		Messages: []provider.Message{{Role: "user", Content: "hello"}},
		Metadata: map[string]any{"profileArn": "arn:aws:codewhisperer:us-east-1:123:profile/XYZ", "conversationId": "conv-1", "continuationId": "cont-1"},
	}, config.Account{AccountID: "arn:aws:codewhisperer:us-east-1:123:profile/ABC"}, config.ThinkingSettings{})
	if err != nil {
		t.Fatalf("buildRequestPayload: %v", err)
	}
	if payload.ProfileARN != "arn:aws:codewhisperer:us-east-1:123:profile/XYZ" {
		t.Fatalf("unexpected profile ARN: %#v", payload.ProfileARN)
	}
	if payload.ConversationState.ConversationID != "conv-1" {
		t.Fatalf("unexpected conversation ID: %#v", payload.ConversationState.ConversationID)
	}
	if payload.ConversationState.AgentContinuationID != "cont-1" {
		t.Fatalf("unexpected continuation ID: %#v", payload.ConversationState.AgentContinuationID)
	}

	payload, err = buildRequestPayload(provider.ChatRequest{
		Model:    "claude-sonnet-4.5",
		Messages: []provider.Message{{Role: "user", Content: "hello"}},
		Metadata: map[string]any{"profileArn": "arn:aws:codewhisperer:us-east-1:123:profile/XYZ"},
	}, config.Account{AccountID: "arn:aws:codewhisperer:us-east-1:123:profile/ABC", ClientID: "client", ClientSecret: "secret"}, config.ThinkingSettings{})
	if err != nil {
		t.Fatalf("buildRequestPayload: %v", err)
	}
	if payload.ProfileARN != "" {
		t.Fatalf("expected profile ARN to be omitted for builder-id accounts, got %#v", payload.ProfileARN)
	}
}

func TestBuildRequestPayload_ToolOnlyAssistantAndToolResultsUseKiroSafePlaceholders(t *testing.T) {
	payload, err := buildRequestPayload(provider.ChatRequest{
		Model: "claude-sonnet-4.5",
		Tools: []provider.Tool{{Type: "function", Function: provider.ToolFunction{Name: "Read", Parameters: map[string]any{"type": "object"}}}},
		Messages: []provider.Message{
			{Role: "user", Content: "inspect repo"},
			{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "tool_1", Type: "function", Function: provider.ToolCallTarget{Name: "Read", Arguments: `{"path":"README.md"}`}}}},
			{Role: "tool", ToolCallID: "tool_1", Content: "readme content"},
		},
	}, config.Account{}, config.ThinkingSettings{})
	if err != nil {
		t.Fatalf("buildRequestPayload: %v", err)
	}
	if len(payload.ConversationState.History) != 2 {
		t.Fatalf("unexpected history length: %#v", payload.ConversationState.History)
	}
	assistant := payload.ConversationState.History[1].AssistantResponseMessage
	if assistant == nil {
		t.Fatalf("expected assistant history message")
	}
	if assistant.Content != "." {
		t.Fatalf("expected tool-only assistant placeholder '.', got %#v", assistant.Content)
	}
	current := payload.ConversationState.CurrentMessage.UserInputMessage
	if current.Content != "Tool results provided." {
		t.Fatalf("expected tool-result placeholder content, got content=%#v context=%#v", current.Content, current.UserInputMessageContext)
	}
	if current.UserInputMessageContext == nil || len(current.UserInputMessageContext.ToolResults) != 1 {
		t.Fatalf("expected one tool result in current context, got %#v", current.UserInputMessageContext)
	}
}

func TestBuildRequestPayload_SplitsParallelToolRoundTripsIntoAlternatingHistory(t *testing.T) {
	request := provider.ChatRequest{
		Model: "claude-sonnet-4.5",
		Tools: []provider.Tool{
			{Type: "function", Function: provider.ToolFunction{Name: "Read", Parameters: map[string]any{"type": "object"}}},
			{Type: "function", Function: provider.ToolFunction{Name: "Glob", Parameters: map[string]any{"type": "object"}}},
		},
		Messages: []provider.Message{
			{Role: "user", Content: "inspect repo"},
			{Role: "assistant", ToolCalls: []provider.ToolCall{
				{ID: "tool_1", Type: "function", Function: provider.ToolCallTarget{Name: "Read", Arguments: `{"file_path":"README.md"}`}},
				{ID: "tool_2", Type: "function", Function: provider.ToolCallTarget{Name: "Glob", Arguments: `{"pattern":"internal/**/*.go"}`}},
			}},
			{Role: "tool", ToolCallID: "tool_1", Content: "README contents"},
			{Role: "tool", ToolCallID: "tool_2", Content: "internal files"},
		},
	}
	normalized, _, err := normalizeRequest(request)
	if err != nil {
		t.Fatalf("normalizeRequest: %v", err)
	}
	if len(normalized) != 5 {
		t.Fatalf("expected normalized alternating tool round-trips, got %#v", normalized)
	}
	if len(normalized[1].ToolUses) != 1 || normalized[1].ToolUses[0].ToolUseID != "tool_1" {
		t.Fatalf("expected first split assistant tool use, got %#v", normalized[1])
	}
	if len(normalized[2].ToolResults) != 1 || normalized[2].ToolResults[0].ToolUseID != "tool_1" {
		t.Fatalf("expected first split tool result, got %#v", normalized[2])
	}
	if len(normalized[3].ToolUses) != 1 || normalized[3].ToolUses[0].ToolUseID != "tool_2" {
		t.Fatalf("expected second split assistant tool use, got %#v", normalized[3])
	}
	if len(normalized[4].ToolResults) != 1 || normalized[4].ToolResults[0].ToolUseID != "tool_2" {
		t.Fatalf("expected second split tool result, got %#v", normalized[4])
	}

	payload, err := buildRequestPayload(request, config.Account{}, config.ThinkingSettings{})
	if err != nil {
		t.Fatalf("buildRequestPayload: %v", err)
	}
	history := payload.ConversationState.History
	if len(history) != 4 {
		t.Fatalf("expected alternating split history, got %#v", history)
	}
	if history[1].AssistantResponseMessage == nil || len(history[1].AssistantResponseMessage.ToolUses) != 1 || history[1].AssistantResponseMessage.ToolUses[0].ToolUseID != "tool_1" {
		t.Fatalf("expected first assistant tool use split out, got %#v", history[1])
	}
	if history[2].UserInputMessage == nil || history[2].UserInputMessage.UserInputMessageContext == nil || len(history[2].UserInputMessage.UserInputMessageContext.ToolResults) != 1 || history[2].UserInputMessage.UserInputMessageContext.ToolResults[0].ToolUseID != "tool_1" {
		t.Fatalf("expected first tool result split into history, got %#v", history[2])
	}
	current := payload.ConversationState.CurrentMessage.UserInputMessage
	if current.UserInputMessageContext == nil || len(current.UserInputMessageContext.ToolResults) != 1 || current.UserInputMessageContext.ToolResults[0].ToolUseID != "tool_2" {
		t.Fatalf("expected current tool result to contain last tool round-trip, got %#v", current.UserInputMessageContext)
	}
}

func TestBuildRequestPayload_ForcedThinkingInjectionModes(t *testing.T) {
	tests := []struct {
		name       string
		mode       config.ThinkingMode
		wantInject bool
	}{
		{name: "off", mode: config.ThinkingModeOff},
		{name: "auto", mode: config.ThinkingModeAuto, wantInject: true},
		{name: "force", mode: config.ThinkingModeForce, wantInject: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := buildRequestPayload(provider.ChatRequest{
				RouteFamily: string(contract.EndpointAnthropicMessages),
				Model:       "claude-sonnet-4.5",
				Thinking:    contract.ThinkingConfig{Requested: true},
				Messages:    []provider.Message{{Role: "user", Content: "hello"}},
			}, config.Account{}, config.ThinkingSettings{Mode: tt.mode, ForceForAnthropic: true, MaxForcedThinkingTokens: 2048})
			if err != nil {
				t.Fatalf("buildRequestPayload: %v", err)
			}
			content := payload.ConversationState.CurrentMessage.UserInputMessage.Content
			if got := strings.Contains(content, forcedThinkingModeTag); got != tt.wantInject {
				t.Fatalf("thinking injection present = %v, want %v; content=%q", got, tt.wantInject, content)
			}
			if tt.wantInject && !strings.Contains(content, "<max_thinking_length>2048</max_thinking_length>") {
				t.Fatalf("expected configured max thinking length tag, got %q", content)
			}
		})
	}
}

func TestBuildRequestPayload_ForcedThinkingInjectionStaysOffForNonThinkingTraffic(t *testing.T) {
	payload, err := buildRequestPayload(provider.ChatRequest{
		RouteFamily: string(contract.EndpointAnthropicMessages),
		Model:       "claude-sonnet-4.5",
		Thinking:    contract.ThinkingConfig{},
		Messages:    []provider.Message{{Role: "user", Content: "hello"}},
	}, config.Account{}, config.ThinkingSettings{Mode: config.ThinkingModeForce, ForceForAnthropic: true, MaxForcedThinkingTokens: 2048})
	if err != nil {
		t.Fatalf("buildRequestPayload: %v", err)
	}
	content := payload.ConversationState.CurrentMessage.UserInputMessage.Content
	if strings.Contains(content, forcedThinkingModeTag) {
		t.Fatalf("expected no thinking injection for non-thinking request, got %q", content)
	}
}
