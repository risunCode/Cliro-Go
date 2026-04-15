package proxyhttp

import (
	"context"
	"fmt"
	"testing"

	"cliro/internal/config"
	"cliro/internal/logger"
	codexprovider "cliro/internal/provider/codex"
	kiroprovider "cliro/internal/provider/kiro"
	models "cliro/internal/proxy/models"
)

type fakeCodexExecutor struct {
	outcome codexprovider.CompletionOutcome
	status  int
	message string
	err     error
}

func (f fakeCodexExecutor) ExecuteFromIR(_ context.Context, _ models.Request) (codexprovider.CompletionOutcome, int, string, error) {
	return f.outcome, f.status, f.message, f.err
}

func TestPrepareExecutionRequestRoutesClaudeToKiro(t *testing.T) {
	server := newTestServer(t)
	req := models.Request{Protocol: models.ProtocolOpenAI, Endpoint: models.EndpointOpenAIChat, Model: "claude-sonnet-4.5"}
	_, resolution, _, _, err := server.prepareExecutionRequest(req)
	if err != nil {
		t.Fatalf("prepareExecutionRequest error: %v", err)
	}
	if resolution.Provider != models.ProviderKiro {
		t.Fatalf("provider = %q", resolution.Provider)
	}
}

func TestPrepareExecutionRequestRoutesGPTToCodex(t *testing.T) {
	server := newTestServer(t)
	req := models.Request{Protocol: models.ProtocolOpenAI, Endpoint: models.EndpointOpenAIChat, Model: "gpt-5.4"}
	_, resolution, _, _, err := server.prepareExecutionRequest(req)
	if err != nil {
		t.Fatalf("prepareExecutionRequest error: %v", err)
	}
	if resolution.Provider != models.ProviderCodex {
		t.Fatalf("provider = %q", resolution.Provider)
	}
}

func TestExecuteRequestMapsCodexOutcomeToCanonicalResponse(t *testing.T) {
	server := newTestServer(t)
	server.codex = fakeCodexExecutor{outcome: codexprovider.CompletionOutcome{ID: "resp_1", Model: "gpt-5.4", Text: "hello", Thinking: "reason", Usage: config.ProxyStats{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3}, Provider: "codex", AccountLabel: "acct"}}
	req := models.Request{Protocol: models.ProtocolOpenAI, Endpoint: models.EndpointOpenAIChat, Model: "gpt-5.4", Messages: []models.Message{{Role: models.RoleUser, Content: "hi"}}}
	resp, status, message, err := server.executeRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("executeRequest error: %v status=%d message=%q", err, status, message)
	}
	if resp.Text != "hello" || resp.Thinking != "reason" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Usage.TotalTokens != 3 {
		t.Fatalf("usage total = %d", resp.Usage.TotalTokens)
	}
}

func TestKiroOutcomeToResponseMapsToolUses(t *testing.T) {
	outcome := kiroprovider.CompletionOutcome{ID: "msg_1", Model: "claude-sonnet-4.5", Text: "hello", ToolUses: []kiroprovider.ToolUse{{ID: "toolu_1", Name: "search", Input: map[string]any{"q": "golang"}}}, Usage: config.ProxyStats{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8}}
	resp := kiroOutcomeToResponse(outcome, "claude-sonnet-4.5")
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d", len(resp.ToolCalls))
	}
	if resp.StopReason != "tool_calls" {
		t.Fatalf("stop reason = %q", resp.StopReason)
	}
	if resp.ToolCalls[0].Arguments == "" {
		t.Fatalf("expected tool arguments")
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	store, err := config.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return &Server{store: store, log: logger.New(50), codex: fakeCodexExecutor{err: fmt.Errorf("not configured")}}
}
