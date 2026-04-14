package anthropic

import (
	"testing"

	contract "cliro/internal/contract"
)

func TestMessagesToIR_MergesMessagesAndPreservesThinkingBlocks(t *testing.T) {
	request, err := MessagesToIR(MessagesRequest{
		Model:     "claude-sonnet-4.5",
		MaxTokens: 256,
		Messages: []Message{
			{Role: "assistant", Content: []any{
				map[string]any{"type": "thinking", "thinking": "plan", "signature": "sig_plan", "cache_control": map[string]any{"type": "ephemeral"}},
				map[string]any{"type": "tool_use", "id": "call_1", "name": "Grep", "input": map[string]any{"query": "needle"}},
			}},
			{Role: "assistant", Content: []any{map[string]any{"type": "text", "text": "searching"}}},
			{Role: "user", Content: []any{map[string]any{"type": "text", "text": "continue"}}},
			{Role: "user", Content: []any{map[string]any{"type": "redacted_thinking", "data": "secret", "cache_control": map[string]any{"type": "ephemeral"}}}},
		},
	})
	if err != nil {
		t.Fatalf("MessagesToIR: %v", err)
	}
	if len(request.Messages) != 2 {
		t.Fatalf("message count = %d, want 2 messages=%#v", len(request.Messages), request.Messages)
	}

	assistant := request.Messages[0]
	if assistant.Role != contract.RoleAssistant {
		t.Fatalf("assistant role = %q", assistant.Role)
	}
	if assistant.Content != "searching" {
		t.Fatalf("assistant content = %#v", assistant.Content)
	}
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].Name != "Grep" {
		t.Fatalf("assistant tool calls = %#v", assistant.ToolCalls)
	}
	if len(assistant.ThinkingBlocks) != 1 {
		t.Fatalf("assistant thinking blocks = %#v", assistant.ThinkingBlocks)
	}
	if assistant.ThinkingBlocks[0] != (contract.ThinkingBlock{Thinking: "plan", Signature: "sig_plan"}) {
		t.Fatalf("unexpected thinking block: %#v", assistant.ThinkingBlocks[0])
	}

	user := request.Messages[1]
	if user.Role != contract.RoleUser {
		t.Fatalf("user role = %q", user.Role)
	}
	if user.Content != "continue" {
		t.Fatalf("user content = %#v", user.Content)
	}
}

func TestMessagesToIR_PreservesBuiltinToolType(t *testing.T) {
	request, err := MessagesToIR(MessagesRequest{
		Model:      "claude-sonnet-4.5",
		Messages:   []Message{{Role: "user", Content: "search this"}},
		Tools:      []Tool{{Type: "web_search"}},
		ToolChoice: map[string]any{"type": "web_search"},
	})
	if err != nil {
		t.Fatalf("MessagesToIR: %v", err)
	}
	if len(request.Tools) != 1 {
		t.Fatalf("tool count = %d", len(request.Tools))
	}
	if request.Tools[0].Type != "web_search" {
		t.Fatalf("tool type = %q", request.Tools[0].Type)
	}
	if request.Tools[0].Name != "web_search" {
		t.Fatalf("tool name = %q", request.Tools[0].Name)
	}
}

func TestParseThinkingConfig_ConvertsOpenAIReasoningEffortToAnthropicBudgetTokens(t *testing.T) {
	tests := []struct {
		name          string
		input         map[string]any
		wantRequested bool
		wantBudget    int
	}{
		{name: "empty", input: map[string]any{}, wantRequested: false, wantBudget: 0},
		{name: "effort_low", input: map[string]any{"effort": "low"}, wantRequested: true, wantBudget: 4096},
		{name: "effort_medium", input: map[string]any{"effort": "medium"}, wantRequested: true, wantBudget: 10000},
		{name: "effort_high", input: map[string]any{"effort": "high"}, wantRequested: true, wantBudget: 16384},
		{name: "effort_xhigh", input: map[string]any{"effort": "xhigh"}, wantRequested: true, wantBudget: 32768},
		{name: "effort_minimal", input: map[string]any{"effort": "minimal"}, wantRequested: true, wantBudget: 4096},
		{name: "budget_tokens_preserved", input: map[string]any{"budget_tokens": 8192}, wantRequested: true, wantBudget: 8192},
		{name: "type_preserved", input: map[string]any{"type": "enabled"}, wantRequested: true, wantBudget: 0},
		{name: "unknown_param_filtered", input: map[string]any{"unknown": "value"}, wantRequested: true, wantBudget: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := parseThinkingConfig(tt.input)
			if config.Requested != tt.wantRequested {
				t.Errorf("Requested = %v, want %v", config.Requested, tt.wantRequested)
			}
			if tt.wantBudget > 0 {
				budget, ok := config.RawParams["budget_tokens"].(int)
				if !ok || budget != tt.wantBudget {
					t.Errorf("budget_tokens = %v, want %d", config.RawParams["budget_tokens"], tt.wantBudget)
				}
			}
		})
	}
}

func TestMessagesToIR_ConvertsOrphanToolResultsIntoUserMessage(t *testing.T) {
	request, err := MessagesToIR(MessagesRequest{
		Model:     "claude-sonnet-4.5",
		MaxTokens: 128,
		Messages: []Message{
			{Role: "assistant", Content: []any{map[string]any{"type": "text", "text": "hello"}}},
			{Role: "user", Content: []any{map[string]any{"type": "tool_result", "tool_use_id": "missing_call", "content": "orphan result"}}},
		},
	})
	if err != nil {
		t.Fatalf("MessagesToIR: %v", err)
	}
	if len(request.Messages) != 2 {
		t.Fatalf("message count = %d messages=%#v", len(request.Messages), request.Messages)
	}
	if request.Messages[1].Role != contract.RoleUser {
		t.Fatalf("orphan role = %q", request.Messages[1].Role)
	}
	if request.Messages[1].ToolCallID != "" {
		t.Fatalf("unexpected tool_call_id = %q", request.Messages[1].ToolCallID)
	}
	if request.Messages[1].Content != "orphan result" {
		t.Fatalf("content = %#v", request.Messages[1].Content)
	}
}

func TestMessagesToIR_PreservesSplitTextSpacing(t *testing.T) {
	request, err := MessagesToIR(MessagesRequest{
		Model: "claude-sonnet-4.5",
		Messages: []Message{{Role: "assistant", Content: []any{
			map[string]any{"type": "text", "text": "Baik, saya akan cek struktur"},
			map[string]any{"type": "text", "text": " modal dan layout"},
			map[string]any{"type": "text", "text": " app shell."},
		}}},
	})
	if err != nil {
		t.Fatalf("MessagesToIR: %v", err)
	}
	if len(request.Messages) != 1 {
		t.Fatalf("message count = %d", len(request.Messages))
	}
	if request.Messages[0].Content != "Baik, saya akan cek struktur modal dan layout app shell." {
		t.Fatalf("assistant content = %#v", request.Messages[0].Content)
	}
}
