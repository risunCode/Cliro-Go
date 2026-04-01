package kiro

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"cliro-go/internal/account"
	"cliro-go/internal/auth"
	"cliro-go/internal/config"
	contract "cliro-go/internal/contract"
	"cliro-go/internal/logger"
	provider "cliro-go/internal/provider"
)

func TestRuntimeClient_RetriesAfterFirstTokenTimeout(t *testing.T) {
	var mu sync.Mutex
	requestHosts := make([]string, 0, 2)
	requestTargets := make([]string, 0, 2)
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		requestHosts = append(requestHosts, req.URL.Host)
		requestTargets = append(requestTargets, req.Header.Get("X-Amz-Target"))
		attempt := len(requestHosts)
		mu.Unlock()

		if attempt == 1 {
			return &http.Response{StatusCode: http.StatusOK, Body: newBlockingBody()}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(awsEventFrame(t, "assistantResponseEvent", map[string]any{"content": "ok"}))))}, nil
	})

	client := newRuntimeClient(&http.Client{Transport: transport}, 5*time.Millisecond)
	resp, endpoint, err := client.Do(context.Background(), config.Account{AccessToken: "token"}, []byte(`{}`))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()
	if endpoint.Name == "" {
		t.Fatalf("expected resolved endpoint name")
	}
	if len(requestHosts) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requestHosts))
	}
	if requestTargets[0] != "" {
		t.Fatalf("unexpected primary target header: %#v", requestTargets)
	}
}

func TestService_FailsOverAcrossAccounts(t *testing.T) {
	store, authManager, log := newTestDeps(t)
	if err := store.UpsertAccount(config.Account{ID: "acct-fail", Provider: "kiro", Email: "fail@example.com", AccessToken: "token-fail", Enabled: true, CreatedAt: 1, UpdatedAt: 1, HealthState: config.AccountHealthReady}); err != nil {
		t.Fatalf("UpsertAccount fail account: %v", err)
	}
	if err := store.UpsertAccount(config.Account{ID: "acct-ok", Provider: "kiro", Email: "ok@example.com", AccessToken: "token-ok", Enabled: true, CreatedAt: 2, UpdatedAt: 2, HealthState: config.AccountHealthReady}); err != nil {
		t.Fatalf("UpsertAccount ok account: %v", err)
	}

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.Header.Get("Authorization") {
		case "Bearer token-fail":
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader(`{"message":"temporary upstream failure"}`))}, nil
		case "Bearer token-ok":
			body := bytesJoinFrames(t,
				awsEventFrame(t, "assistantResponseEvent", map[string]any{"content": "hello from kiro"}),
				awsEventFrame(t, "meteringEvent", map[string]any{"usage": map[string]any{"inputTokens": 3, "outputTokens": 5, "totalTokens": 8}}),
			)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
		default:
			return nil, io.ErrUnexpectedEOF
		}
	})

	service := NewService(store, authManager, account.NewPool(store), log, &http.Client{Transport: transport, Timeout: 2 * time.Second})
	outcome, status, message, err := service.Complete(context.Background(), provider.ChatRequest{
		RouteFamily: "openai_chat",
		Model:       "claude-sonnet-4.5",
		Messages:    []provider.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Complete: status=%d message=%q err=%v", status, message, err)
	}
	if outcome.AccountID != "acct-ok" {
		t.Fatalf("expected successful failover to second account, got %#v", outcome.AccountID)
	}
	if outcome.Text != "hello from kiro" {
		t.Fatalf("unexpected outcome text: %#v", outcome.Text)
	}
	failedAccount, ok := store.GetAccount("acct-fail")
	if !ok || failedAccount.ErrorCount == 0 {
		t.Fatalf("expected failed account to record the failed attempt, got %#v", failedAccount)
	}
}

func TestService_PrefersNativeThinkingOverParsedFallback(t *testing.T) {
	store, authManager, log := newTestDeps(t)
	if err := store.UpsertAccount(config.Account{ID: "acct-ok", Provider: "kiro", Email: "ok@example.com", AccessToken: "token-ok", Enabled: true, CreatedAt: 1, UpdatedAt: 1, HealthState: config.AccountHealthReady}); err != nil {
		t.Fatalf("UpsertAccount: %v", err)
	}

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := bytesJoinFrames(t,
			awsEventFrame(t, "assistantResponseEvent", map[string]any{"content": "<thinking>parsed plan</thinking>Visible answer"}),
			awsEventFrame(t, "reasoningContentEvent", map[string]any{"text": "native plan"}),
		)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})

	service := NewService(store, authManager, account.NewPool(store), log, &http.Client{Transport: transport, Timeout: 2 * time.Second})
	outcome, status, message, err := service.Complete(context.Background(), provider.ChatRequest{
		RouteFamily: "anthropic_messages",
		Model:       "claude-sonnet-4.5",
		Thinking:    contract.ThinkingConfig{Requested: true, Mode: contract.ThinkingModeAuto},
		Messages:    []provider.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Complete: status=%d message=%q err=%v", status, message, err)
	}
	if outcome.Thinking != "native plan" {
		t.Fatalf("expected native thinking to win, got %q", outcome.Thinking)
	}
	if outcome.Text != "Visible answer" {
		t.Fatalf("expected parsed fallback tags to be removed from text, got %q", outcome.Text)
	}
}

func TestService_UsesParsedFallbackWhenNativeThinkingMissing(t *testing.T) {
	store, authManager, log := newTestDeps(t)
	if err := store.UpsertAccount(config.Account{ID: "acct-ok", Provider: "kiro", Email: "ok@example.com", AccessToken: "token-ok", Enabled: true, CreatedAt: 1, UpdatedAt: 1, HealthState: config.AccountHealthReady}); err != nil {
		t.Fatalf("UpsertAccount: %v", err)
	}

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := bytesJoinFrames(t,
			awsEventFrame(t, "assistantResponseEvent", map[string]any{"content": "<thinking>parsed plan</thinking>Visible answer"}),
		)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})

	service := NewService(store, authManager, account.NewPool(store), log, &http.Client{Transport: transport, Timeout: 2 * time.Second})
	outcome, status, message, err := service.Complete(context.Background(), provider.ChatRequest{
		RouteFamily: "anthropic_messages",
		Model:       "claude-sonnet-4.5",
		Thinking:    contract.ThinkingConfig{Requested: true, Mode: contract.ThinkingModeAuto},
		Messages:    []provider.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Complete: status=%d message=%q err=%v", status, message, err)
	}
	if outcome.Thinking != "parsed plan" {
		t.Fatalf("expected parsed fallback thinking, got %q", outcome.Thinking)
	}
	if outcome.Text != "Visible answer" {
		t.Fatalf("expected fallback tags to be removed from text, got %q", outcome.Text)
	}
	if outcome.ThinkingSignature == "" {
		t.Fatalf("expected parsed fallback thinking signature")
	}
}

func TestService_DoesNotActivateParsedFallbackWithoutThinkingRequest(t *testing.T) {
	store, authManager, log := newTestDeps(t)
	if err := store.UpsertAccount(config.Account{ID: "acct-ok", Provider: "kiro", Email: "ok@example.com", AccessToken: "token-ok", Enabled: true, CreatedAt: 1, UpdatedAt: 1, HealthState: config.AccountHealthReady}); err != nil {
		t.Fatalf("UpsertAccount: %v", err)
	}

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := bytesJoinFrames(t,
			awsEventFrame(t, "assistantResponseEvent", map[string]any{"content": "<thinking>parsed plan</thinking>Visible answer"}),
		)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})

	service := NewService(store, authManager, account.NewPool(store), log, &http.Client{Transport: transport, Timeout: 2 * time.Second})
	outcome, status, message, err := service.Complete(context.Background(), provider.ChatRequest{
		RouteFamily: "anthropic_messages",
		Model:       "claude-sonnet-4.5",
		Messages:    []provider.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Complete: status=%d message=%q err=%v", status, message, err)
	}
	if outcome.Thinking != "" {
		t.Fatalf("expected no parsed fallback thinking, got %q", outcome.Thinking)
	}
	if outcome.Text != "<thinking>parsed plan</thinking>Visible answer" {
		t.Fatalf("expected original text to remain unchanged, got %q", outcome.Text)
	}
}

func newTestDeps(t *testing.T) (*config.Manager, *auth.Manager, *logger.Logger) {
	t.Helper()
	store, err := config.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	log := logger.New(100)
	return store, auth.NewManager(store, log), log
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type blockingBody struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingBody() *blockingBody {
	return &blockingBody{closed: make(chan struct{})}
}

func (b *blockingBody) Read(_ []byte) (int, error) {
	<-b.closed
	return 0, io.EOF
}

func (b *blockingBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

func bytesJoinFrames(t *testing.T, frames ...[]byte) string {
	t.Helper()
	joined := make([]byte, 0)
	for _, frame := range frames {
		joined = append(joined, frame...)
	}
	return string(joined)
}
