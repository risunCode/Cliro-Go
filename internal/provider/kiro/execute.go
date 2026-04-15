package kiro

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"cliro/internal/config"
	"cliro/internal/logger"
	"cliro/internal/platform"
	"cliro/internal/provider"
	models "cliro/internal/proxy/models"

	"github.com/google/uuid"
)

func (s *Service) executeWithAccount(ctx context.Context, account config.Account, request models.Request) (CompletionOutcome, int, string, error) {
	payload, err := BuildPayload(request, account)
	if err != nil {
		return CompletionOutcome{}, http.StatusBadRequest, err.Error(), err
	}
	body, err := MarshalPayload(payload)
	if err != nil {
		return CompletionOutcome{}, http.StatusBadRequest, err.Error(), err
	}
	var lastStatus int
	var lastMessage string
	var lastErr error
	requestID := platform.RequestIDFromContext(ctx)
	for _, runtimeURL := range kiroRuntimeURLs {
		s.log.Info("proxy", "kiro.runtime_attempt", logger.F("request_id", requestID), logger.F("runtime_url", runtimeURL), logger.F("account", strings.TrimSpace(account.Email)), logger.F("model", strings.TrimSpace(request.Model)))
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, runtimeURL, bytes.NewReader(body))
		if err != nil {
			return CompletionOutcome{}, 0, err.Error(), err
		}
		applyKiroRuntimeHeaders(httpReq, account)
		resp, err := s.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			lastMessage = err.Error()
			s.log.Warn("proxy", "kiro.runtime_attempt_failed", logger.F("request_id", requestID), logger.F("runtime_url", runtimeURL), logger.F("error", err.Error()))
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			data, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			lastStatus = resp.StatusCode
			lastMessage = provider.CompactHTTPBody(data)
			if lastMessage == "" {
				lastMessage = fmt.Sprintf("kiro upstream returned %d", resp.StatusCode)
			}
			lastErr = fmt.Errorf(lastMessage)
			s.log.Warn("proxy", "kiro.runtime_attempt_failed", logger.F("request_id", requestID), logger.F("runtime_url", runtimeURL), logger.F("status", resp.StatusCode), logger.F("error", lastMessage))
			continue
		}
		outcome, err := collectCompletion(resp.Body, request.Model)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			lastMessage = err.Error()
			s.log.Warn("proxy", "kiro.runtime_attempt_failed", logger.F("request_id", requestID), logger.F("runtime_url", runtimeURL), logger.F("error", err.Error()))
			continue
		}
		s.log.Info("proxy", "kiro.runtime_attempt_succeeded", logger.F("request_id", requestID), logger.F("runtime_url", runtimeURL))
		if strings.TrimSpace(outcome.ID) == "" {
			outcome.ID = "msg_" + uuid.NewString()
		}
		return outcome, 0, "", nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("kiro runtime request failed")
	}
	return CompletionOutcome{}, lastStatus, firstNonEmpty(lastMessage, lastErr.Error()), lastErr
}

func applyKiroRuntimeHeaders(req *http.Request, account config.Account) {
	applyKiroQuotaHeaders(req, account.AccessToken)
	req.Header.Set("Accept", "application/vnd.amazon.eventstream")
	req.Header.Set("x-amzn-kiro-agent-mode", "spec")
	req.Header.Set("x-amz-sso-bearer", strings.TrimSpace(account.AccessToken))
	req.Header.Set("x-amzn-codewhisperer-machine-id", kiroMachineIDHeaderVal)
	if strings.TrimSpace(account.AccountID) != "" {
		req.Header.Set("x-amzn-codewhisperer-profile-arn", strings.TrimSpace(account.AccountID))
	}
}
