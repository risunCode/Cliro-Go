package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	accountstate "cliro/internal/account"
	"cliro/internal/config"
	"cliro/internal/logger"
	"cliro/internal/provider"
	"cliro/internal/proxy/models"

	"github.com/google/uuid"
)

type responseEvent struct {
	Type     string       `json:"type"`
	Delta    string       `json:"delta"`
	Text     string       `json:"text"`
	Item     responseItem `json:"item"`
	Response struct {
		ID    string `json:"id"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
		Output []responseItem `json:"output"`
	} `json:"response"`
	Error struct {
		Message         string `json:"message"`
		Type            string `json:"type"`
		ResetsInSeconds int64  `json:"resets_in_seconds"`
		ResetsAt        int64  `json:"resets_at"`
	} `json:"error"`
}

type responseItem struct {
	ID               string            `json:"id"`
	Type             string            `json:"type"`
	Role             string            `json:"role"`
	Status           string            `json:"status"`
	CallID           string            `json:"call_id"`
	Name             string            `json:"name"`
	Arguments        string            `json:"arguments"`
	EncryptedContent string            `json:"encrypted_content"`
	Content          []responseContent `json:"content"`
	Summary          []responseContent `json:"summary"`
}

type responseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (s *Service) Complete(ctx context.Context, req ChatRequest) (CompletionOutcome, int, string, error) {
	requestID := requestIDFromContext(ctx)
	if strings.TrimSpace(req.Model) == "" {
		s.recordRequestFailure()
		s.logProxyEvent("warn", "request.rejected", requestID, logger.F("route", strings.TrimSpace(req.RouteFamily)), logger.F("reason", "model is required"))
		return CompletionOutcome{}, http.StatusBadRequest, "model is required", fmt.Errorf("model is required")
	}

	if s.pool.AvailabilitySnapshot("codex").ReadyCount == 0 {
		s.recordRequestFailure()
		reason := s.pool.ProviderUnavailableReason("codex")
		s.logProxyEvent("warn", "request.rejected", requestID, logger.F("route", strings.TrimSpace(req.RouteFamily)), logger.F("reason", reason))
		return CompletionOutcome{}, http.StatusServiceUnavailable, reason, fmt.Errorf(reason)
	}

	var lastStatus int
	var lastMessage string
	excluded := make(map[string]bool)
	attempt := 0
	attemptCtx := AttemptContext{RequestID: requestID, Provider: "codex", Model: req.Model, Stream: req.Stream}
	toolNames := BuildToolNameMapping(req.Tools, req.Messages, DefaultToolNameLimit)

	for {
		candidate, ok := s.retryPlan.NextAccount(excluded)
		if !ok {
			break
		}
		attempt++
		accountLabel := accountstate.Label(candidate)
		s.logProxyEvent("info", "request.attempt", requestID, logger.F("route", strings.TrimSpace(req.RouteFamily)), logger.F("account", accountLabel), logger.F("model", strings.TrimSpace(req.Model)))
		account, err := s.auth.EnsureFreshAccount(candidate.ID)
		if err != nil {
			decision := provider.ClassifyHTTPFailure(http.StatusUnauthorized, err.Error())
			result := AttemptResult{Attempt: attempt, Status: decision.Status, Message: decision.Message, Err: err, Failure: decision, RetryCause: "ensure_fresh_account", Final: true}
			retryDecision := s.retryPlan.Decide(result)
			result.RetryCause = retryDecision.Cause
			result.Final = !retryDecision.Retry && !retryDecision.RefreshAuth
			LogAttemptDiagnostic(s.log, NewAttemptDiagnostic(attemptCtx, candidate.ID, accountLabel, result))
			s.applyFailureDecision(requestID, candidate.ID, accountLabel, decision)
			lastStatus = decision.Status
			lastMessage = decision.Message
			excluded[candidate.ID] = true
			continue
		}
		accountLabel = accountstate.Label(account)
		recoveredAuth := false

		for {
			upstreamReq, err := s.buildRequest(ctx, account, req, toolNames)
			if err != nil {
				s.recordRequestFailure()
				return CompletionOutcome{}, http.StatusBadRequest, err.Error(), err
			}

			openStarted := time.Now()
			resp, err := s.httpClient.Do(upstreamReq)
			openDuration := time.Since(openStarted)
			if err != nil {
				decision := provider.ClassifyTransportFailure(err)
				result := AttemptResult{Attempt: attempt, Status: decision.Status, Message: decision.Message, Err: err, Failure: decision, UpstreamOpen: openDuration, RetryCause: "transport_error"}
				retryDecision := s.retryPlan.Decide(result)
				result.RetryCause = retryDecision.Cause
				result.Final = !retryDecision.Retry && !retryDecision.RefreshAuth
				LogAttemptDiagnostic(s.log, NewAttemptDiagnostic(attemptCtx, account.ID, accountLabel, result))
				s.applyFailureDecision(requestID, account.ID, accountLabel, decision)
				lastStatus = decision.Status
				lastMessage = decision.Message
				if retryDecision.ExcludeAccount {
					excluded[account.ID] = true
				}
				break
			}

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				data, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				decision := s.handleUpstreamFailure(account, resp.StatusCode, data)
				result := AttemptResult{Attempt: attempt, Status: resp.StatusCode, Message: decision.Message, Failure: decision, UpstreamOpen: openDuration, RecoveredAuth: recoveredAuth, RetryCause: "upstream_http_error"}
				retryDecision := s.retryPlan.Decide(result)
				if retryDecision.RefreshAuth {
					refreshedAccount, recoveryStatus, refreshErr := s.recovery.Recover(ctx, "codex", account.ID)
					if refreshErr == nil {
						account = refreshedAccount
						accountLabel = accountstate.Label(account)
						recoveredAuth = true
						s.logAuthEvent("info", "auth.token_refreshed_retry", requestID, logger.F("account", accountLabel), logger.F("recovery_status", string(recoveryStatus)))
						continue
					}
					decision = provider.ClassifyHTTPFailure(http.StatusUnauthorized, refreshErr.Error())
					result = AttemptResult{Attempt: attempt, Status: decision.Status, Message: decision.Message, Err: refreshErr, Failure: decision, UpstreamOpen: openDuration, RecoveredAuth: true, RetryCause: "auth_refresh_rejected"}
					retryDecision = s.retryPlan.Decide(result)
				}

				result.RetryCause = retryDecision.Cause
				result.Final = !retryDecision.Retry && !retryDecision.RefreshAuth
				LogAttemptDiagnostic(s.log, NewAttemptDiagnostic(attemptCtx, account.ID, accountLabel, result))
				s.applyFailureDecision(requestID, account.ID, accountLabel, decision)
				lastStatus = decision.Status
				lastMessage = decision.Message
				if decision.Class == provider.FailureRequestShape {
					s.recordRequestFailure()
					return CompletionOutcome{}, decision.Status, decision.Message, fmt.Errorf(decision.Message)
				}
				if retryDecision.ExcludeAccount {
					excluded[account.ID] = true
				}
				break
			}

			outcome, err := s.collectCompletion(resp.Body, req.Model, toolNames)
			_ = resp.Body.Close()
			if err != nil {
				decision := provider.ClassifyTransportFailure(err)
				result := AttemptResult{Attempt: attempt, Status: decision.Status, Message: decision.Message, Err: err, Failure: decision, UpstreamOpen: openDuration, UpstreamReadable: true, RetryCause: "stream_parse_error"}
				retryDecision := s.retryPlan.Decide(result)
				result.RetryCause = retryDecision.Cause
				result.Final = !retryDecision.Retry && !retryDecision.RefreshAuth
				LogAttemptDiagnostic(s.log, NewAttemptDiagnostic(attemptCtx, account.ID, accountLabel, result))
				s.applyFailureDecision(requestID, account.ID, accountLabel, decision)
				lastStatus = decision.Status
				lastMessage = decision.Message
				if retryDecision.ExcludeAccount {
					excluded[account.ID] = true
				}
				break
			}
			if !CompletionHasVisibleOutput(outcome) {
				decision := provider.FailureDecision{Class: provider.FailureEmptyStream, Message: "empty stream", RetryAllowed: true, Status: http.StatusBadGateway}
				result := AttemptResult{Attempt: attempt, Status: decision.Status, Message: decision.Message, Failure: decision, UpstreamOpen: openDuration, UpstreamReadable: true, EmptyStream: true, RetryCause: "empty_stream"}
				retryDecision := s.retryPlan.Decide(result)
				result.RetryCause = retryDecision.Cause
				result.Final = !retryDecision.Retry && !retryDecision.RefreshAuth
				LogAttemptDiagnostic(s.log, NewAttemptDiagnostic(attemptCtx, account.ID, accountLabel, result))
				s.applyFailureDecision(requestID, account.ID, accountLabel, decision)
				lastStatus = decision.Status
				lastMessage = decision.Message
				if retryDecision.ExcludeAccount {
					excluded[account.ID] = true
				}
				break
			}
			outcome.Provider = "codex"
			outcome.AccountID = account.ID
			outcome.AccountLabel = accountLabel

			s.markSuccess(requestID, account.ID, accountLabel, outcome.Usage)
			LogAttemptDiagnostic(s.log, NewAttemptDiagnostic(attemptCtx, account.ID, accountLabel, AttemptResult{Attempt: attempt, Success: true, Final: true, UpstreamOpen: openDuration, UpstreamReadable: true, CompletionHasOutput: true}))
			return outcome, 0, "", nil
		}
	}

	snapshot := s.pool.AvailabilitySnapshot("codex")
	if snapshot.ReadyCount == 0 {
		lastStatus = http.StatusServiceUnavailable
		lastMessage = s.pool.ProviderUnavailableReason("codex")
	}
	if lastStatus == 0 {
		lastStatus = http.StatusServiceUnavailable
	}
	if strings.TrimSpace(lastMessage) == "" {
		lastMessage = "all codex accounts failed"
	}
	s.recordRequestFailure()
	s.logProxyEvent("warn", "request.failed", requestID, logger.F("route", strings.TrimSpace(req.RouteFamily)), logger.F("reason", lastMessage))
	return CompletionOutcome{}, lastStatus, lastMessage, fmt.Errorf(lastMessage)
}

func (s *Service) buildRequest(ctx context.Context, account config.Account, req ChatRequest, toolNames ToolNameMapping) (*http.Request, error) {
	payload, _, err := s.buildRequestPayloadWithToolNames(RemapChatRequestToolNames(req, toolNames))
	if err != nil {
		return nil, fmt.Errorf("messages are empty")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, codexBaseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+account.AccessToken)
	httpReq.Header.Set("Session_id", uuid.NewString())
	httpReq.Header.Set("User-Agent", codexUserAgent)
	httpReq.Header.Set("Version", codexVersion)
	httpReq.Header.Set("Origin", "https://chatgpt.com")
	httpReq.Header.Set("Referer", "https://chatgpt.com/")
	httpReq.Header.Set("Connection", "Keep-Alive")
	httpReq.Header.Set("Originator", "codex-tui")
	if strings.TrimSpace(account.AccountID) != "" {
		httpReq.Header.Set("Chatgpt-Account-Id", account.AccountID)
	}

	return httpReq, nil
}

func (s *Service) collectCompletion(body io.Reader, model string, toolNames ToolNameMapping) (CompletionOutcome, error) {
	var out CompletionOutcome
	out.ID = "chatcmpl-" + uuid.NewString()
	out.Model = model

	scanner := bufio.NewScanner(body)
	bufPtr := scannerBufPool.Get().(*[]byte)
	defer func() {
		*bufPtr = (*bufPtr)[:0]
		scannerBufPool.Put(bufPtr)
	}()
	scanner.Buffer(*bufPtr, 10*1024*1024)
	var textBuilder strings.Builder
	var thinkingBuilder strings.Builder
	toolUses := make([]ToolUse, 0)
	seenToolUseIDs := make(map[string]struct{})
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var event responseEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		switch event.Type {
		case "response.output_text.delta":
			if event.Delta != "" {
				textBuilder.WriteString(event.Delta)
			} else if event.Text != "" {
				textBuilder.WriteString(event.Text)
			}
		case "response.reasoning.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.output_reasoning.delta":
			if event.Delta != "" {
				thinkingBuilder.WriteString(event.Delta)
			} else if event.Text != "" {
				thinkingBuilder.WriteString(event.Text)
			}
		case "response.output_item.done":
			collectResponseItem(&textBuilder, &thinkingBuilder, &toolUses, seenToolUseIDs, event.Item, toolNames)
		case "response.completed":
			out.ID = firstNonEmpty(event.Response.ID, out.ID)
			out.Usage.PromptTokens = event.Response.Usage.InputTokens
			out.Usage.CompletionTokens = event.Response.Usage.OutputTokens
			out.Usage.TotalTokens = event.Response.Usage.TotalTokens
			for _, item := range event.Response.Output {
				collectResponseItem(&textBuilder, &thinkingBuilder, &toolUses, seenToolUseIDs, item, toolNames)
			}
		case "error":
			return out, fmt.Errorf(firstNonEmpty(event.Error.Message, "upstream error"))
		}
	}
	if err := scanner.Err(); err != nil {
		return out, err
	}

	out.Text = textBuilder.String()
	out.Thinking = thinkingBuilder.String()
	out.ThinkingSignature = models.StableThinkingSignature(out.Thinking)
	if strings.TrimSpace(out.Thinking) != "" {
		out.ThinkingSource = "native"
	} else {
		out.ThinkingSource = "none"
	}
	out.ToolUses = RestoreToolUseNames(toolUses, toolNames)
	if out.Usage.TotalTokens == 0 {
		out.Usage.TotalTokens = out.Usage.PromptTokens + out.Usage.CompletionTokens
	}
	return out, nil
}

func collectResponseItem(textBuilder *strings.Builder, thinkingBuilder *strings.Builder, toolUses *[]ToolUse, seenToolUseIDs map[string]struct{}, item responseItem, toolNames ToolNameMapping) {
	switch strings.ToLower(strings.TrimSpace(item.Type)) {
	case "function_call":
		toolUseID := firstNonEmpty(strings.TrimSpace(item.CallID), strings.TrimSpace(item.ID))
		toolName := toolNames.Restore(item.Name)
		if toolUseID == "" || toolName == "" {
			return
		}
		if _, exists := seenToolUseIDs[toolUseID]; exists {
			return
		}
		seenToolUseIDs[toolUseID] = struct{}{}
		*toolUses = append(*toolUses, ToolUse{ID: toolUseID, Name: toolName, Input: remappedCodexToolArgs(toolName, item.Arguments)})
	case "message":
		if textBuilder.Len() == 0 {
			if text := responseItemText(item.Content); text != "" {
				textBuilder.WriteString(text)
			}
		}
	case "reasoning":
		if thinkingBuilder.Len() == 0 {
			if text := firstNonEmpty(responseItemText(item.Summary), responseItemText(item.Content)); text != "" {
				thinkingBuilder.WriteString(text)
			}
		}
	}
}

func responseItemText(content []responseContent) string {
	if len(content) == 0 {
		return ""
	}
	parts := make([]string, 0, len(content))
	for _, part := range content {
		if strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func remappedCodexToolArgs(name string, arguments string) map[string]any {
	input := map[string]any{}
	if strings.TrimSpace(arguments) != "" {
		_ = json.Unmarshal([]byte(arguments), &input)
	}
	return models.RemapToolCallArgs(name, input)
}

func (s *Service) handleUpstreamFailure(account config.Account, statusCode int, body []byte) provider.FailureDecision {
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = fmt.Sprintf("upstream returned %d", statusCode)
	}

	var event responseEvent
	if err := json.Unmarshal(body, &event); err == nil && event.Error.Message != "" {
		message = event.Error.Message
	}
	decision := provider.ClassifyHTTPFailure(statusCode, message)
	if decision.Class == provider.FailureQuotaCooldown {
		cooldownUntil := time.Now().Add(decision.Cooldown).Unix()
		if err := json.Unmarshal(body, &event); err == nil {
			if event.Error.ResetsAt > time.Now().Unix() {
				cooldownUntil = event.Error.ResetsAt
			} else if event.Error.ResetsInSeconds > 0 {
				cooldownUntil = time.Now().Add(time.Duration(event.Error.ResetsInSeconds) * time.Second).Unix()
			}
		}
		decision.Cooldown = time.Until(time.Unix(cooldownUntil, 0))
		if decision.Cooldown < 0 {
			decision.Cooldown = quotaCooldown
		}
	}
	return decision
}

func maxDuration(current time.Duration, fallback time.Duration) time.Duration {
	if current > 0 {
		return current
	}
	if fallback > 0 {
		return fallback
	}
	return 0
}
