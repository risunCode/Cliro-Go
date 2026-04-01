package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	contract "cliro-go/internal/contract"
	"cliro-go/internal/logger"
	"cliro-go/internal/platform"
	provider "cliro-go/internal/provider"
	kiroprovider "cliro-go/internal/provider/kiro"
)

type recordingExecutor struct {
	lastRequest contract.Request
	outcome     provider.CompletionOutcome
	status      int
	message     string
	err         error
}

func (r *recordingExecutor) ExecuteFromIR(_ context.Context, request contract.Request) (provider.CompletionOutcome, int, string, error) {
	r.lastRequest = request
	return r.outcome, r.status, r.message, r.err
}

type recordingLiveExecutor struct {
	recordingExecutor
	lastChatRequest provider.ChatRequest
	callbackEvents  []kiroprovider.StreamEvent
}

func (r *recordingLiveExecutor) CompleteWithCallback(_ context.Context, req provider.ChatRequest, eventCallback func(kiroprovider.StreamEvent)) (provider.CompletionOutcome, int, string, error) {
	r.lastChatRequest = req
	if eventCallback != nil {
		if len(r.callbackEvents) == 0 {
			eventCallback(kiroprovider.StreamEvent{Text: "done"})
		} else {
			for _, event := range r.callbackEvents {
				eventCallback(event)
			}
		}
	}
	return r.outcome, r.status, r.message, r.err
}

func TestExecuteRequest_PreservesThinkingIntentForResolvedModel(t *testing.T) {
	server, _, log := newTestServer(t)
	executor := &recordingExecutor{outcome: provider.CompletionOutcome{Text: "ok"}}
	server.kiro = executor

	ctx := platform.WithRequestID(context.Background(), "req-thinking")
	response, status, message, err := server.executeRequest(ctx, contract.Request{
		Endpoint: contract.EndpointAnthropicMessages,
		Model:    "claude-sonnet-4.5-thinking",
		Messages: []contract.Message{{Role: contract.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("executeRequest error: %v status=%d message=%q", err, status, message)
	}
	if executor.lastRequest.Model != "claude-sonnet-4.5" {
		t.Fatalf("executor model = %q, want %q", executor.lastRequest.Model, "claude-sonnet-4.5")
	}
	if !executor.lastRequest.Thinking.Requested {
		t.Fatalf("executor thinking requested = false, want true")
	}
	if response.Model != "claude-sonnet-4.5" {
		t.Fatalf("response model = %q, want %q", response.Model, "claude-sonnet-4.5")
	}
	entries := log.Entries(20)
	entry, ok := findLogEntry(entries, "thinking.decision", func(entry logger.Entry) bool {
		_, hasResolvedModel := entry.Fields["resolved_model"]
		return hasResolvedModel
	})
	if !ok {
		t.Fatalf("expected routing thinking decision log, got %+v", entries)
	}
	if entry.Fields["requested_model"] != "claude-sonnet-4.5-thinking" {
		t.Fatalf("requested_model = %#v, want claude-sonnet-4.5-thinking", entry.Fields["requested_model"])
	}
	if entry.Fields["resolved_model"] != "claude-sonnet-4.5" {
		t.Fatalf("resolved_model = %#v, want claude-sonnet-4.5", entry.Fields["resolved_model"])
	}
	if entry.Fields["thinking_requested"] != true {
		t.Fatalf("thinking_requested = %#v, want true", entry.Fields["thinking_requested"])
	}
}

func TestExecuteRequest_KeepsNonThinkingRequestsEquivalent(t *testing.T) {
	server, _, _ := newTestServer(t)
	executor := &recordingExecutor{outcome: provider.CompletionOutcome{Text: "ok"}}
	server.kiro = executor

	ctx := platform.WithRequestID(context.Background(), "req-standard")
	_, status, message, err := server.executeRequest(ctx, contract.Request{
		Endpoint: contract.EndpointAnthropicMessages,
		Model:    "claude-sonnet-4.5",
		Messages: []contract.Message{{Role: contract.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("executeRequest error: %v status=%d message=%q", err, status, message)
	}
	if executor.lastRequest.Model != "claude-sonnet-4.5" {
		t.Fatalf("executor model = %q, want %q", executor.lastRequest.Model, "claude-sonnet-4.5")
	}
	if executor.lastRequest.Thinking.Requested {
		t.Fatalf("executor thinking requested = true, want false")
	}
}

func TestHandleAnthropicMessagesLiveStream_UsesResolvedModelForKiroThinkingRequests(t *testing.T) {
	server, _, _ := newTestServer(t)
	executor := &recordingLiveExecutor{recordingExecutor: recordingExecutor{outcome: provider.CompletionOutcome{Model: "claude-sonnet-4.5"}}}
	server.kiro = executor

	req := httptest.NewRequest(http.MethodPost, RouteAnthropicMessages, strings.NewReader(`{"model":"claude-sonnet-4.5-thinking","max_tokens":64,"stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`))
	rr := httptest.NewRecorder()

	server.handleAnthropicMessages(rr, req)

	if executor.lastChatRequest.Model != "claude-sonnet-4.5" {
		t.Fatalf("live request model = %q, want %q", executor.lastChatRequest.Model, "claude-sonnet-4.5")
	}
	if executor.lastChatRequest.RouteFamily != string(contract.EndpointAnthropicMessages) {
		t.Fatalf("live request route = %q, want %q", executor.lastChatRequest.RouteFamily, contract.EndpointAnthropicMessages)
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("expected event stream content type, got %q", contentType)
	}
	if !strings.Contains(rr.Body.String(), `"model":"claude-sonnet-4.5-thinking"`) {
		t.Fatalf("expected anthropic stream to preserve requested model in message_start: %s", rr.Body.String())
	}
}

func TestHandleAnthropicMessagesLiveStream_EmitsThinkingSignatureBeforeText(t *testing.T) {
	server, _, _ := newTestServer(t)
	executor := &recordingLiveExecutor{
		recordingExecutor: recordingExecutor{outcome: provider.CompletionOutcome{
			Model:             "claude-sonnet-4.5",
			Thinking:          "plan first carefully",
			ThinkingSignature: contract.StableThinkingSignature("plan first carefully"),
		}},
		callbackEvents: []kiroprovider.StreamEvent{{Thinking: "plan first "}, {Thinking: "carefully"}, {Text: "done"}},
	}
	server.kiro = executor

	req := httptest.NewRequest(http.MethodPost, RouteAnthropicMessages, strings.NewReader(`{"model":"claude-sonnet-4.5-thinking","max_tokens":64,"stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`))
	rr := httptest.NewRecorder()

	server.handleAnthropicMessages(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `"type":"signature_delta"`) {
		t.Fatalf("missing signature_delta event: %s", body)
	}
	thinkingDeltaIndex := strings.Index(body, `"type":"thinking_delta"`)
	signatureDeltaIndex := strings.Index(body, `"type":"signature_delta"`)
	textStartIndex := strings.Index(body, `"content_block":{"text":"","type":"text"}`)
	if thinkingDeltaIndex == -1 || signatureDeltaIndex == -1 || textStartIndex == -1 {
		t.Fatalf("missing expected live stream events: %s", body)
	}
	if !(thinkingDeltaIndex < signatureDeltaIndex && signatureDeltaIndex < textStartIndex) {
		t.Fatalf("unexpected live thinking event order: %s", body)
	}
	if strings.Count(body, `"type":"signature_delta"`) != 1 {
		t.Fatalf("signature_delta count = %d, want 1 body=%s", strings.Count(body, `"type":"signature_delta"`), body)
	}
}

func TestHandleAnthropicMessagesLiveStream_EmitsToolUseBlocksFromOutcome(t *testing.T) {
	server, _, _ := newTestServer(t)
	executor := &recordingLiveExecutor{
		recordingExecutor: recordingExecutor{outcome: provider.CompletionOutcome{
			Model: "claude-sonnet-4.5",
			ToolUses: []provider.ToolUse{{
				ID:   "toolu_1",
				Name: "Read",
				Input: map[string]any{
					"path": "README.md",
				},
			}},
		}},
		callbackEvents: []kiroprovider.StreamEvent{{Text: "Saya akan membaca file yang relevan."}},
	}
	server.kiro = executor

	req := httptest.NewRequest(http.MethodPost, RouteAnthropicMessages, strings.NewReader(`{"model":"claude-sonnet-4.5","max_tokens":64,"stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"baca struktur repo"}]}]}`))
	rr := httptest.NewRecorder()

	server.handleAnthropicMessages(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `"name":"Read"`) {
		t.Fatalf("missing tool_use block in live stream: %s", body)
	}
	if !strings.Contains(body, `"partial_json":"{\"file_path\":\"README.md\"}"`) {
		t.Fatalf("missing tool input payload in live stream: %s", body)
	}
	if !strings.Contains(body, `"stop_reason":"tool_use"`) {
		t.Fatalf("expected tool_use stop reason, got %s", body)
	}
	textStopIndex := strings.Index(body, `event: content_block_stop`)
	toolDeltaIndex := strings.Index(body, `"partial_json":"{\"file_path\":\"README.md\"}"`)
	if textStopIndex == -1 || toolDeltaIndex == -1 || toolDeltaIndex < textStopIndex {
		t.Fatalf("expected tool block after text block stop, got %s", body)
	}
}
