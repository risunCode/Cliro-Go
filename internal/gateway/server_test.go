package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cliro-go/internal/account"
	"cliro-go/internal/adapter/ir"
	"cliro-go/internal/config"
	"cliro-go/internal/logger"
	provider "cliro-go/internal/provider"
)

type fakeExecutor struct {
	outcome provider.CompletionOutcome
	status  int
	message string
	err     error
}

func (f fakeExecutor) ExecuteFromIR(_ context.Context, _ ir.Request) (provider.CompletionOutcome, int, string, error) {
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

func TestHandleResponses_AlwaysStreamsAndLogsUsage(t *testing.T) {
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
	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("expected event stream content type, got %q", contentType)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "response.created") || !strings.Contains(body, "response.output_text.delta") {
		t.Fatalf("unexpected responses body: %s", body)
	}
	entries := log.Entries(50)
	if !logContains(entries, `phase="provider_completed"`) {
		t.Fatalf("expected provider_completed log entry, got %+v", entries)
	}
	if !logContains(entries, `total_tokens=8`) {
		t.Fatalf("expected usage log entry, got %+v", entries)
	}
	if !logContains(entries, `status="streaming"`) {
		t.Fatalf("expected streaming completion log entry, got %+v", entries)
	}
}

func TestHandleResponses_StreamsEvenWhenPayloadRequestsFalse(t *testing.T) {
	server, _, _ := newTestServer(t)
	server.codex = fakeExecutor{outcome: provider.CompletionOutcome{
		Text:  "streamed",
		Model: "gpt-5.4",
		Usage: config.ProxyStats{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	}}

	req := httptest.NewRequest(http.MethodPost, RouteOpenAIResponses, strings.NewReader(`{"model":"gpt-5.4","input":"hello","stream":false}`))
	rr := httptest.NewRecorder()

	server.handleResponses(rr, req)

	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("expected event stream content type, got %q", contentType)
	}
	if !strings.Contains(rr.Body.String(), "response.created") {
		t.Fatalf("expected SSE response body, got %s", rr.Body.String())
	}
}

func TestHandleResponses_StreamsEvenWhenPayloadRequestsTrue(t *testing.T) {
	server, _, _ := newTestServer(t)
	server.codex = fakeExecutor{outcome: provider.CompletionOutcome{Text: "stream", Model: "gpt-5.4", Usage: config.ProxyStats{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}}}

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

func TestHandleAnthropicMessages_AlwaysStreams(t *testing.T) {
	server, _, _ := newTestServer(t)
	server.kiro = fakeExecutor{outcome: provider.CompletionOutcome{Text: "kiro stream", Model: "claude-sonnet-4.5", Usage: config.ProxyStats{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}}}

	req := httptest.NewRequest(http.MethodPost, RouteAnthropicMessages, strings.NewReader(`{"model":"claude-sonnet-4.5","max_tokens":64,"stream":false,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`))
	rr := httptest.NewRecorder()

	server.handleAnthropicMessages(rr, req)

	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("expected stream content type, got %q", contentType)
	}

	reqThinking := httptest.NewRequest(http.MethodPost, RouteAnthropicMessages, strings.NewReader(`{"model":"claude-sonnet-4.5-thinking","max_tokens":64,"stream":false,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`))
	rrThinking := httptest.NewRecorder()

	server.handleAnthropicMessages(rrThinking, reqThinking)

	if contentType := rrThinking.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("expected stream content type with thinking alias, got %q", contentType)
	}
	if !strings.Contains(rrThinking.Body.String(), "message_start") {
		t.Fatalf("expected anthropic SSE body, got %s", rrThinking.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "message_start") {
		t.Fatalf("expected anthropic SSE body without thinking alias, got %s", rr.Body.String())
	}
	if rr.Code != http.StatusOK || rrThinking.Code != http.StatusOK {
		t.Fatalf("unexpected status codes base=%d thinking=%d", rr.Code, rrThinking.Code)
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
