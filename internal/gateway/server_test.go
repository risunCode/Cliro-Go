package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cliro-go/internal/account"
	"cliro-go/internal/config"
	contract "cliro-go/internal/contract"
	"cliro-go/internal/logger"
	provider "cliro-go/internal/provider"
)

type fakeExecutor struct {
	outcome provider.CompletionOutcome
	status  int
	message string
	err     error
}

func (f fakeExecutor) ExecuteFromIR(_ context.Context, _ contract.Request) (provider.CompletionOutcome, int, string, error) {
	return f.outcome, f.status, f.message, f.err
}

func newTestServer(t *testing.T) (*Server, *config.Manager, *logger.Logger) {
	t.Helper()
	dataDir := t.TempDir()
	store, err := config.NewManager(dataDir)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	log := logger.New(100)
	return &Server{store: store, pool: account.NewPool(store), log: log}, store, log
}

func logContains(entries []logger.Entry, fragment string) bool {
	for _, entry := range entries {
		if strings.Contains(entry.Message, fragment) {
			return true
		}
	}
	return false
}

func findLogEntry(entries []logger.Entry, event string, predicate func(logger.Entry) bool) (logger.Entry, bool) {
	for _, entry := range entries {
		if entry.Event != event {
			continue
		}
		if predicate == nil || predicate(entry) {
			return entry, true
		}
	}
	return logger.Entry{}, false
}

func TestHandleResponses_RespectsNonStreamRequestsAndLogsUsage(t *testing.T) {
	server, _, log := newTestServer(t)
	server.codex = fakeExecutor{outcome: provider.CompletionOutcome{
		Text:         "hello from responses",
		Model:        "gpt-5.4",
		Provider:     "codex",
		AccountID:    "acct-1",
		AccountLabel: "codex@example.com",
		Usage:        config.ProxyStats{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8},
	}}

	req := httptest.NewRequest(http.MethodPost, RouteOpenAIResponses, strings.NewReader(`{"model":"gpt-5.4","input":"hello","stream":false}`))
	rr := httptest.NewRecorder()

	server.handleResponses(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("expected json content type, got %q", contentType)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"object":"response"`) || !strings.Contains(body, "hello from responses") {
		t.Fatalf("unexpected responses body: %s", body)
	}
	entries := log.Entries(50)
	if !logContains(entries, `phase="provider_completed"`) {
		t.Fatalf("expected provider_completed log entry, got %+v", entries)
	}
	if !logContains(entries, `total_tokens=8`) {
		t.Fatalf("expected usage log entry, got %+v", entries)
	}
	if !logContains(entries, `status="completed"`) {
		t.Fatalf("expected completed log entry, got %+v", entries)
	}
}

func TestHandleResponses_StreamsWhenPayloadRequestsTrue(t *testing.T) {
	server, _, _ := newTestServer(t)
	server.codex = fakeExecutor{outcome: provider.CompletionOutcome{
		Text:  "streamed",
		Model: "gpt-5.4",
		Usage: config.ProxyStats{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	}}

	req := httptest.NewRequest(http.MethodPost, RouteOpenAIResponses, strings.NewReader(`{"model":"gpt-5.4","input":"hello","stream":true}`))
	rr := httptest.NewRecorder()

	server.handleResponses(rr, req)

	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("expected event stream content type, got %q", contentType)
	}
	if !strings.Contains(rr.Body.String(), "response.created") {
		t.Fatalf("expected SSE response body, got %s", rr.Body.String())
	}
}

func TestHandleChatCompletions_ReturnsJSONWhenPayloadRequestsFalse(t *testing.T) {
	server, _, _ := newTestServer(t)
	server.codex = fakeExecutor{outcome: provider.CompletionOutcome{Text: "done", Model: "gpt-5.4", Usage: config.ProxyStats{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}}}

	req := httptest.NewRequest(http.MethodPost, RouteOpenAIChatCompletions, strings.NewReader(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`))
	rr := httptest.NewRecorder()

	server.handleChatCompletions(rr, req)

	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("expected json content type, got %q", contentType)
	}
	if !strings.Contains(rr.Body.String(), `"content":"done"`) {
		t.Fatalf("expected JSON chat response body, got %s", rr.Body.String())
	}
}

func TestHandleAnthropicMessages_ReturnsJSONWhenPayloadRequestsFalse(t *testing.T) {
	server, _, _ := newTestServer(t)
	server.codex = fakeExecutor{outcome: provider.CompletionOutcome{Text: "anthropic codex stream", Model: "gpt-5.4", Usage: config.ProxyStats{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}}}

	req := httptest.NewRequest(http.MethodPost, RouteAnthropicMessages, strings.NewReader(`{"model":"gpt-5.4","max_tokens":64,"stream":false,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`))
	rr := httptest.NewRecorder()

	server.handleAnthropicMessages(rr, req)

	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("expected json content type, got %q", contentType)
	}

	if !strings.Contains(rr.Body.String(), `"type":"message"`) {
		t.Fatalf("expected anthropic JSON body, got %s", rr.Body.String())
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status code=%d", rr.Code)
	}

}

func TestHandleAnthropicMessages_LogsThinkingDecisionWithoutContent(t *testing.T) {
	server, _, log := newTestServer(t)
	server.kiro = fakeExecutor{outcome: provider.CompletionOutcome{
		Text:              "done",
		Thinking:          "plan first carefully",
		ThinkingSignature: contract.StableThinkingSignature("plan first carefully"),
		ThinkingSource:    "parsed",
		Model:             "claude-sonnet-4.5",
		Provider:          "kiro",
		Usage:             config.ProxyStats{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	}}

	req := httptest.NewRequest(http.MethodPost, RouteAnthropicMessages, strings.NewReader(`{"model":"claude-sonnet-4.5-thinking","max_tokens":64,"stream":false,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`))
	rr := httptest.NewRecorder()

	server.handleAnthropicMessages(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	entries := log.Entries(50)
	entry, ok := findLogEntry(entries, "thinking.decision", func(entry logger.Entry) bool {
		return entry.Fields["route"] == string(contract.EndpointAnthropicMessages) && entry.Fields["anthropic_signature_emitted"] == true
	})
	if !ok {
		t.Fatalf("expected anthropic thinking.decision log entry, got %+v", entries)
	}
	if entry.Fields["thinking_requested"] != true {
		t.Fatalf("thinking_requested = %#v, want true", entry.Fields["thinking_requested"])
	}
	if entry.Fields["thinking_source"] != "parsed" {
		t.Fatalf("thinking_source = %#v, want parsed", entry.Fields["thinking_source"])
	}
	if entry.Fields["thinking_emitted"] != true {
		t.Fatalf("thinking_emitted = %#v, want true", entry.Fields["thinking_emitted"])
	}
	if strings.Contains(entry.Message, "plan first carefully") {
		t.Fatalf("thinking content leaked in message: %q", entry.Message)
	}
	for key, value := range entry.Fields {
		if strings.Contains(key, "content") {
			t.Fatalf("unexpected content field logged: %q=%#v", key, value)
		}
		if text, ok := value.(string); ok && strings.Contains(text, "plan first carefully") {
			t.Fatalf("thinking content leaked in field %q=%q", key, text)
		}
	}
}

func TestHandleChatCompletions_RoutesKiroModelsToKiroExecutor(t *testing.T) {
	server, _, log := newTestServer(t)
	server.kiro = fakeExecutor{outcome: provider.CompletionOutcome{
		Text:         "kiro stream",
		Model:        "claude-sonnet-4.5",
		Provider:     "kiro",
		AccountID:    "acct-kiro",
		AccountLabel: "kiro@example.com",
		Usage:        config.ProxyStats{PromptTokens: 2, CompletionTokens: 4, TotalTokens: 6},
	}}

	req := httptest.NewRequest(http.MethodPost, RouteOpenAIChatCompletions, strings.NewReader(`{"model":"claude-sonnet-4.5","messages":[{"role":"user","content":"hello"}],"stream":false}`))
	rr := httptest.NewRecorder()

	server.handleChatCompletions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("expected json content type, got %q", contentType)
	}
	if !strings.Contains(rr.Body.String(), "kiro stream") {
		t.Fatalf("unexpected response body: %s", rr.Body.String())
	}
	entries := log.Entries(50)
	if !logContains(entries, `provider="kiro"`) {
		t.Fatalf("expected kiro provider log entry, got %+v", entries)
	}
}

func TestCompatV1AnthropicMessagesRoute(t *testing.T) {
	server, _, _ := newTestServer(t)
	server.codex = fakeExecutor{outcome: provider.CompletionOutcome{Text: "compat anthropic stream", Model: "gpt-5.4", Usage: config.ProxyStats{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}}}

	req := httptest.NewRequest(http.MethodPost, compatV1Path(RouteAnthropicMessages), strings.NewReader(`{"model":"gpt-5.4","max_tokens":64,"stream":false,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`))
	rr := httptest.NewRecorder()

	server.newMux().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("expected json content type, got %q", contentType)
	}
	if !strings.Contains(rr.Body.String(), `"type":"message"`) {
		t.Fatalf("expected anthropic JSON body, got %s", rr.Body.String())
	}
}

func TestHandleAnthropicMessages_StreamsWhenPayloadRequestsTrue(t *testing.T) {
	server, _, _ := newTestServer(t)
	server.codex = fakeExecutor{outcome: provider.CompletionOutcome{Text: "anthropic codex stream", Model: "gpt-5.4", Usage: config.ProxyStats{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}}}

	req := httptest.NewRequest(http.MethodPost, RouteAnthropicMessages, strings.NewReader(`{"model":"gpt-5.4","max_tokens":64,"stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`))
	rr := httptest.NewRecorder()

	server.handleAnthropicMessages(rr, req)

	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("expected stream content type, got %q", contentType)
	}
	if !strings.Contains(rr.Body.String(), "message_start") {
		t.Fatalf("expected anthropic SSE body, got %s", rr.Body.String())
	}
}

func TestCompatV1AnthropicCountTokensRoute(t *testing.T) {
	server, _, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, compatV1Path(RouteAnthropicCountTokens), strings.NewReader(`{"model":"claude-sonnet-4.5","messages":[{"role":"user","content":[{"type":"text","text":"hello there"}]}]}`))
	rr := httptest.NewRecorder()

	server.newMux().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("expected json content type, got %q", contentType)
	}
	if !strings.Contains(rr.Body.String(), `"input_tokens"`) {
		t.Fatalf("expected token count response, got %s", rr.Body.String())
	}
}

func TestCompatV1HealthRoute(t *testing.T) {
	server, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, compatV1Path(RouteHealth), nil)
	rr := httptest.NewRecorder()

	server.newMux().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"ok"`) {
		t.Fatalf("expected health response body, got %s", rr.Body.String())
	}
}

func TestCompatDoubleV1HealthRoute(t *testing.T) {
	server, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, compatV1Path(compatV1Path(RouteHealth)), nil)
	rr := httptest.NewRecorder()

	server.newMux().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"ok"`) {
		t.Fatalf("expected health response body, got %s", rr.Body.String())
	}
}

func TestCompatV1RootRoute(t *testing.T) {
	server, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, compatV1Path("/"), nil)
	rr := httptest.NewRecorder()

	server.newMux().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"name":"CLIro-Go Gateway"`) {
		t.Fatalf("expected gateway metadata body, got %s", rr.Body.String())
	}
}

func TestStreamAnthropicMessages_UsesSharedThinkingLifecycleForBufferedReplay(t *testing.T) {
	server, _, _ := newTestServer(t)
	rr := httptest.NewRecorder()

	server.streamAnthropicMessages(rr, "claude-sonnet-4.5", contract.Response{
		ID:                "msg_test",
		Model:             "claude-sonnet-4.5",
		Thinking:          "plan first",
		ThinkingSignature: "sig_custom",
		Text:              "done",
		ToolCalls:         []contract.ToolCall{{ID: "toolu_1", Name: "Grep", Arguments: `{"query":"needle","paths":["src"]}`}},
		Usage:             contract.Usage{InputTokens: 7, OutputTokens: 9},
	})

	body := rr.Body.String()
	if !strings.Contains(body, `"type":"signature_delta"`) {
		t.Fatalf("missing signature_delta event: %s", body)
	}
	if !strings.Contains(body, `"signature":""`) {
		t.Fatalf("expected empty signature in thinking block start: %s", body)
	}
	if !strings.Contains(body, `partial_json":"{\"path\":\"src\",\"pattern\":\"needle\"}"`) {
		t.Fatalf("expected remapped tool args in stream: %s", body)
	}
	if strings.Contains(body, `\"query\"`) {
		t.Fatalf("expected query key to be remapped out: %s", body)
	}
	if !strings.Contains(body, `"input_tokens":7`) || !strings.Contains(body, `"output_tokens":9`) {
		t.Fatalf("expected usage tokens in stream: %s", body)
	}

	thinkingDeltaIndex := strings.Index(body, `"type":"thinking_delta"`)
	signatureDeltaIndex := strings.Index(body, `"type":"signature_delta"`)
	textStartIndex := strings.Index(body, `"content_block":{"text":"","type":"text"}`)
	contentBlockStopIndex := strings.Index(body, `event: content_block_stop`)
	if thinkingDeltaIndex == -1 || signatureDeltaIndex == -1 || textStartIndex == -1 || contentBlockStopIndex == -1 {
		t.Fatalf("missing expected event sequence: %s", body)
	}
	if !(thinkingDeltaIndex < signatureDeltaIndex && signatureDeltaIndex < contentBlockStopIndex && contentBlockStopIndex < textStartIndex) {
		t.Fatalf("unexpected thinking event order: %s", body)
	}
	if strings.Count(body, `"type":"signature_delta"`) != 1 {
		t.Fatalf("signature_delta count = %d, want 1 body=%s", strings.Count(body, `"type":"signature_delta"`), body)
	}
}

func TestHandleModels_RejectsConflictingSecurityHeaders(t *testing.T) {
	server, store, _ := newTestServer(t)
	if err := store.SetProxyAPIKey("secret-1"); err != nil {
		t.Fatalf("set proxy api key: %v", err)
	}
	if err := store.SetAuthorizationMode(true); err != nil {
		t.Fatalf("set authorization mode: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, RouteModels, nil)
	req.Header.Set("Authorization", "Bearer secret-1")
	req.Header.Set("X-API-Key", "secret-2")
	rr := httptest.NewRecorder()

	server.handleModels(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "conflicting Authorization and X-API-Key headers") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestHandleModels_RequiresKeyWhenAuthorizationModeEnabled(t *testing.T) {
	server, store, _ := newTestServer(t)
	if err := store.SetAuthorizationMode(true); err != nil {
		t.Fatalf("set authorization mode: %v", err)
	}
	if err := store.SetProxyAPIKey("secret-1"); err != nil {
		t.Fatalf("set proxy api key: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, RouteModels, nil)
	rr := httptest.NewRecorder()

	server.handleModels(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rr.Body.String(), "missing proxy API key") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestHandleModels_AcceptsValidKeyWhenAuthorizationModeEnabled(t *testing.T) {
	server, store, _ := newTestServer(t)
	if err := store.SetAuthorizationMode(true); err != nil {
		t.Fatalf("set authorization mode: %v", err)
	}
	if err := store.SetProxyAPIKey("secret-1"); err != nil {
		t.Fatalf("set proxy api key: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, RouteModels, nil)
	req.Header.Set("Authorization", "Bearer secret-1")
	rr := httptest.NewRecorder()

	server.handleModels(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestPoolUnavailableReasonIncludesAvailabilityBreakdown(t *testing.T) {
	server, store, _ := newTestServer(t)
	if err := store.UpsertAccount(config.Account{ID: "codex-cooldown", Provider: "codex", Email: "cooldown@example.com", Enabled: true, HealthState: config.AccountHealthCooldownTransient, CooldownUntil: time.Now().Add(time.Minute).Unix(), CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatalf("upsert cooldown account: %v", err)
	}
	reason := server.pool.ProviderUnavailableReason("codex")
	if !strings.Contains(reason, "cooldown_transient=1") {
		t.Fatalf("unexpected reason: %s", reason)
	}
}
