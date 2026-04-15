package proxyhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"cliro/internal/logger"
	"cliro/internal/platform"
	codexprovider "cliro/internal/provider/codex"
	kiroprovider "cliro/internal/provider/kiro"
	models "cliro/internal/proxy/models"
	sharedproxy "cliro/internal/proxy/shared"
)

func (s *Server) prepareExecutionRequest(request models.Request) (models.Request, models.ModelResolution, int, string, error) {
	aliases := s.store.ModelAliasesSnapshot()
	resolution, err := models.ResolveModel(request.Model, models.DefaultThinkingSuffix, aliases)
	if err != nil {
		return models.Request{}, models.ModelResolution{}, http.StatusBadRequest, err.Error(), err
	}

	request.Thinking.Requested = request.Thinking.Requested || resolution.ThinkingRequested
	if resolution.ThinkingEffort != "" && len(request.Thinking.RawParams) == 0 {
		request.Thinking.RawParams = map[string]any{"effort": resolution.ThinkingEffort}
	}
	request.Model = resolution.ResolvedModel

	if err := models.ValidateEndpointProvider(string(request.Endpoint), resolution.Provider); err != nil {
		return models.Request{}, models.ModelResolution{}, http.StatusBadRequest, err.Error(), err
	}
	if err := models.ValidateRequest(request, string(resolution.Provider)); err != nil {
		return models.Request{}, models.ModelResolution{}, http.StatusBadRequest, err.Error(), err
	}

	return request, resolution, 0, "", nil
}

func (s *Server) executeRequest(ctx context.Context, request models.Request) (models.Response, int, string, error) {
	requestID := platform.RequestIDFromContext(ctx)
	preparedRequest, resolution, status, message, err := s.prepareExecutionRequest(request)
	if err != nil {
		s.recordRequestFailure()
		s.logRequestEvent("warn", requestID, "failed", fmt.Sprintf("route=%q", string(request.Endpoint)), fmt.Sprintf("reason=%q", err.Error()))
		return models.Response{}, status, message, err
	}
	s.logThinkingDecision("info", requestID, logger.F("requested_model", strings.TrimSpace(resolution.RequestedModel)), logger.F("resolved_model", strings.TrimSpace(resolution.ResolvedModel)), logger.F("provider", string(resolution.Provider)), logger.F("thinking_requested", preparedRequest.Thinking.Requested))
	s.logRequestEvent("info", requestID, "routed", fmt.Sprintf("route=%q", string(preparedRequest.Endpoint)), fmt.Sprintf("provider=%q", string(resolution.Provider)), fmt.Sprintf("model=%q", strings.TrimSpace(preparedRequest.Model)))

	logCompletion := func(providerValue string, accountLabel string, modelValue string, usage models.Usage, thinkingSource string, thinking string) {
		s.logRequestEvent(
			"info",
			requestID,
			"provider_completed",
			fmt.Sprintf("route=%q", string(request.Endpoint)),
			fmt.Sprintf("provider=%q", providerValue),
			fmt.Sprintf("account=%q", strings.TrimSpace(accountLabel)),
			fmt.Sprintf("model=%q", sharedproxy.FirstNonEmpty(strings.TrimSpace(modelValue), strings.TrimSpace(preparedRequest.Model))),
			fmt.Sprintf("prompt_tokens=%d", usage.PromptTokens),
			fmt.Sprintf("completion_tokens=%d", usage.CompletionTokens),
			fmt.Sprintf("total_tokens=%d", usage.TotalTokens),
		)
		s.logThinkingDecision(
			"info",
			requestID,
			logger.F("route", string(request.Endpoint)),
			logger.F("provider", providerValue),
			logger.F("thinking_requested", preparedRequest.Thinking.Requested),
			logger.F("thinking_source", thinkingSourceValue(thinkingSource, thinking)),
			logger.F("thinking_emitted", strings.TrimSpace(thinking) != ""),
		)
	}

	switch resolution.Provider {
	case models.ProviderCodex:
		outcome, status, message, execErr := s.codex.ExecuteFromIR(ctx, preparedRequest)
		if execErr != nil {
			return models.Response{}, status, message, execErr
		}
		response := outcomeToIRResponse(outcome, preparedRequest.Model)
		logCompletion(sharedproxy.FirstNonEmpty(strings.TrimSpace(outcome.Provider), string(resolution.Provider)), outcome.AccountLabel, outcome.Model, response.Usage, outcome.ThinkingSource, outcome.Thinking)
		return response, 0, "", nil
	case models.ProviderKiro:
		outcome, status, message, execErr := s.kiro.ExecuteFromIR(ctx, preparedRequest)
		if execErr != nil {
			return models.Response{}, status, message, execErr
		}
		response := kiroOutcomeToResponse(outcome, preparedRequest.Model)
		logCompletion(sharedproxy.FirstNonEmpty(strings.TrimSpace(outcome.Provider), string(resolution.Provider)), outcome.AccountLabel, outcome.Model, response.Usage, outcome.ThinkingSource, outcome.Thinking)
		return response, 0, "", nil
	default:
		s.recordRequestFailure()
		s.logRequestEvent("warn", requestID, "failed", fmt.Sprintf("route=%q", string(request.Endpoint)), fmt.Sprintf("reason=%q", "unsupported provider"))
		return models.Response{}, http.StatusBadRequest, "unsupported provider", fmt.Errorf("unsupported provider")
	}
}

func kiroOutcomeToResponse(outcome kiroprovider.CompletionOutcome, model string) models.Response {
	toolCalls := make([]models.ToolCall, 0, len(outcome.ToolUses))
	for _, toolUse := range outcome.ToolUses {
		if strings.TrimSpace(toolUse.Name) == "" {
			continue
		}
		arguments := "{}"
		if toolUse.Input != nil {
			if encoded, err := json.Marshal(toolUse.Input); err == nil {
				arguments = string(encoded)
			}
		}
		toolCalls = append(toolCalls, models.ToolCall{ID: toolUse.ID, Name: toolUse.Name, Arguments: arguments})
	}
	stopReason := "stop"
	if len(toolCalls) > 0 {
		stopReason = "tool_calls"
	}
	resolvedModel := sharedproxy.FirstNonEmpty(strings.TrimSpace(outcome.Model), strings.TrimSpace(model))
	return models.Response{ID: outcome.ID, Model: resolvedModel, Text: outcome.Text, Thinking: outcome.Thinking, ThinkingSignature: outcome.ThinkingSignature, ThinkingSource: thinkingSourceValue(outcome.ThinkingSource, outcome.Thinking), ToolCalls: toolCalls, StopReason: stopReason, Usage: models.Usage{PromptTokens: outcome.Usage.PromptTokens, CompletionTokens: outcome.Usage.CompletionTokens, TotalTokens: outcome.Usage.TotalTokens, InputTokens: outcome.Usage.PromptTokens, OutputTokens: outcome.Usage.CompletionTokens}}
}

func outcomeToIRResponse(outcome codexprovider.CompletionOutcome, model string) models.Response {
	toolCalls := make([]models.ToolCall, 0, len(outcome.ToolUses))
	for _, toolUse := range outcome.ToolUses {
		if strings.TrimSpace(toolUse.Name) == "" {
			continue
		}
		arguments := "{}"
		if toolUse.Input != nil {
			if encoded, err := json.Marshal(toolUse.Input); err == nil {
				arguments = string(encoded)
			}
		}
		toolCalls = append(toolCalls, models.ToolCall{
			ID:        toolUse.ID,
			Name:      toolUse.Name,
			Arguments: arguments,
		})
	}

	stopReason := "stop"
	if len(toolCalls) > 0 {
		stopReason = "tool_calls"
	}

	resolvedModel := sharedproxy.FirstNonEmpty(strings.TrimSpace(outcome.Model), strings.TrimSpace(model))

	return models.Response{
		ID:                outcome.ID,
		Model:             resolvedModel,
		Text:              outcome.Text,
		Thinking:          outcome.Thinking,
		ThinkingSignature: outcome.ThinkingSignature,
		ThinkingSource:    thinkingSourceValue(outcome.ThinkingSource, outcome.Thinking),
		ToolCalls:         toolCalls,
		StopReason:        stopReason,
		Usage: models.Usage{
			PromptTokens:     outcome.Usage.PromptTokens,
			CompletionTokens: outcome.Usage.CompletionTokens,
			TotalTokens:      outcome.Usage.TotalTokens,
			InputTokens:      outcome.Usage.PromptTokens,
			OutputTokens:     outcome.Usage.CompletionTokens,
		},
	}
}

func (s *Server) logThinkingDecision(level string, requestID string, fields ...logger.Field) {
	eventFields := append([]logger.Field{logger.F("request_id", strings.TrimSpace(requestID))}, fields...)
	switch level {
	case "warn":
		s.log.Warn("proxy", "thinking_decision", eventFields...)
	case "error":
		s.log.Error("proxy", "thinking_decision", eventFields...)
	case "debug":
		s.log.Debug("proxy", "thinking_decision", eventFields...)
	default:
		s.log.Info("proxy", "thinking_decision", eventFields...)
	}
}

func thinkingSourceValue(source string, thinking string) string {
	trimmedSource := strings.TrimSpace(source)
	if trimmedSource != "" {
		return trimmedSource
	}
	if strings.TrimSpace(thinking) != "" {
		return "native"
	}
	return "none"
}
