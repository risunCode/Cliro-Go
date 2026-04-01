package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	contract "cliro-go/internal/contract"
	"cliro-go/internal/contract/rules"
	"cliro-go/internal/logger"
	"cliro-go/internal/platform"
	"cliro-go/internal/provider"
	"cliro-go/internal/route"
	"cliro-go/internal/util"
)

func (s *Server) prepareExecutionRequest(request contract.Request) (contract.Request, route.ModelResolution, int, string, error) {
	aliases := s.store.ModelAliases()
	resolution, err := route.ResolveModel(request.Model, route.DefaultThinkingSuffix, aliases)
	if err != nil {
		return contract.Request{}, route.ModelResolution{}, http.StatusBadRequest, err.Error(), err
	}

	request.Thinking.Requested = request.Thinking.Requested || resolution.ThinkingRequested
	request.Model = resolution.ResolvedModel

	if err := route.ValidateEndpointProvider(string(request.Endpoint), resolution.Provider); err != nil {
		return contract.Request{}, route.ModelResolution{}, http.StatusBadRequest, err.Error(), err
	}
	if err := rules.ValidateRequest(request, string(resolution.Provider)); err != nil {
		return contract.Request{}, route.ModelResolution{}, http.StatusBadRequest, err.Error(), err
	}

	return request, resolution, 0, "", nil
}

func (s *Server) executeRequest(ctx context.Context, request contract.Request) (contract.Response, int, string, error) {
	requestID := platform.RequestIDFromContext(ctx)
	preparedRequest, resolution, status, message, err := s.prepareExecutionRequest(request)
	if err != nil {
		s.recordRequestFailure()
		s.logRequestEvent("warn", requestID, "failed", fmt.Sprintf("route=%q", string(request.Endpoint)), fmt.Sprintf("reason=%q", err.Error()))
		return contract.Response{}, status, message, err
	}
	s.logThinkingDecision("info", requestID, logger.String("requested_model", strings.TrimSpace(resolution.RequestedModel)), logger.String("resolved_model", strings.TrimSpace(resolution.ResolvedModel)), logger.String("provider", string(resolution.Provider)), logger.Bool("thinking_requested", preparedRequest.Thinking.Requested))
	s.logRequestEvent("info", requestID, "routed", fmt.Sprintf("route=%q", string(preparedRequest.Endpoint)), fmt.Sprintf("provider=%q", string(resolution.Provider)), fmt.Sprintf("model=%q", strings.TrimSpace(preparedRequest.Model)))

	switch resolution.Provider {
	case route.ProviderCodex:
		outcome, status, message, execErr := s.codex.ExecuteFromIR(ctx, preparedRequest)
		if execErr != nil {
			return contract.Response{}, status, message, execErr
		}
		s.logRequestEvent("info", requestID, "provider_completed", fmt.Sprintf("route=%q", string(request.Endpoint)), fmt.Sprintf("provider=%q", util.FirstNonEmpty(strings.TrimSpace(outcome.Provider), string(resolution.Provider))), fmt.Sprintf("account=%q", strings.TrimSpace(outcome.AccountLabel)), fmt.Sprintf("model=%q", strings.TrimSpace(outcome.Model)), fmt.Sprintf("prompt_tokens=%d", outcome.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", outcome.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", outcome.Usage.TotalTokens))
		s.logThinkingDecision("info", requestID, logger.String("route", string(request.Endpoint)), logger.String("provider", util.FirstNonEmpty(strings.TrimSpace(outcome.Provider), string(resolution.Provider))), logger.Bool("thinking_requested", preparedRequest.Thinking.Requested), logger.String("thinking_source", thinkingSourceValue(outcome.ThinkingSource, outcome.Thinking)), logger.Bool("thinking_emitted", strings.TrimSpace(outcome.Thinking) != ""))
		return outcomeToIRResponse(outcome, preparedRequest.Model), 0, "", nil
	case route.ProviderKiro:
		outcome, status, message, execErr := s.kiro.ExecuteFromIR(ctx, preparedRequest)
		if execErr != nil {
			return contract.Response{}, status, message, execErr
		}
		s.logRequestEvent("info", requestID, "provider_completed", fmt.Sprintf("route=%q", string(request.Endpoint)), fmt.Sprintf("provider=%q", util.FirstNonEmpty(strings.TrimSpace(outcome.Provider), string(resolution.Provider))), fmt.Sprintf("account=%q", strings.TrimSpace(outcome.AccountLabel)), fmt.Sprintf("model=%q", strings.TrimSpace(outcome.Model)), fmt.Sprintf("prompt_tokens=%d", outcome.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", outcome.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", outcome.Usage.TotalTokens))
		s.logThinkingDecision("info", requestID, logger.String("route", string(request.Endpoint)), logger.String("provider", util.FirstNonEmpty(strings.TrimSpace(outcome.Provider), string(resolution.Provider))), logger.Bool("thinking_requested", preparedRequest.Thinking.Requested), logger.String("thinking_source", thinkingSourceValue(outcome.ThinkingSource, outcome.Thinking)), logger.Bool("thinking_emitted", strings.TrimSpace(outcome.Thinking) != ""))
		return outcomeToIRResponse(outcome, preparedRequest.Model), 0, "", nil
	default:
		s.recordRequestFailure()
		s.logRequestEvent("warn", requestID, "failed", fmt.Sprintf("route=%q", string(request.Endpoint)), fmt.Sprintf("reason=%q", "unsupported provider"))
		return contract.Response{}, http.StatusBadRequest, "unsupported provider", fmt.Errorf("unsupported provider")
	}
}

func outcomeToIRResponse(outcome provider.CompletionOutcome, model string) contract.Response {
	toolCalls := make([]contract.ToolCall, 0, len(outcome.ToolUses))
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
		toolCalls = append(toolCalls, contract.ToolCall{
			ID:        toolUse.ID,
			Name:      toolUse.Name,
			Arguments: arguments,
		})
	}

	stopReason := "stop"
	if len(toolCalls) > 0 {
		stopReason = "tool_calls"
	}

	resolvedModel := util.FirstNonEmpty(strings.TrimSpace(outcome.Model), strings.TrimSpace(model))

	return contract.Response{
		ID:                outcome.ID,
		Model:             resolvedModel,
		Text:              outcome.Text,
		Thinking:          outcome.Thinking,
		ThinkingSignature: outcome.ThinkingSignature,
		ThinkingSource:    thinkingSourceValue(outcome.ThinkingSource, outcome.Thinking),
		ToolCalls:         toolCalls,
		StopReason:        stopReason,
		Usage: contract.Usage{
			PromptTokens:     outcome.Usage.PromptTokens,
			CompletionTokens: outcome.Usage.CompletionTokens,
			TotalTokens:      outcome.Usage.TotalTokens,
			InputTokens:      outcome.Usage.PromptTokens,
			OutputTokens:     outcome.Usage.CompletionTokens,
		},
	}
}

func (s *Server) logThinkingDecision(level string, requestID string, fields ...logger.Field) {
	eventFields := append([]logger.Field{logger.String("request_id", strings.TrimSpace(requestID))}, fields...)
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "warn":
		s.log.WarnEvent("proxy", "thinking.decision", eventFields...)
	case "error":
		s.log.ErrorEvent("proxy", "thinking.decision", eventFields...)
	case "debug":
		s.log.DebugEvent("proxy", "thinking.decision", eventFields...)
	default:
		s.log.InfoEvent("proxy", "thinking.decision", eventFields...)
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
