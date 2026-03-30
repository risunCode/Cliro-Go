package kiro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cliro-go/internal/account"
	"cliro-go/internal/config"
	"cliro-go/internal/logger"
	"cliro-go/internal/platform"
	"cliro-go/internal/provider"

	"github.com/google/uuid"
)

const (
	kiroVersion                   = "0.10.32"
	requestTimeout                = 5 * time.Minute
	kiroConversationOrigin        = "AI_EDITOR"
	kiroFallbackUserContent       = "."
	kiroThinkingModePrompt        = "<thinking_mode>enabled</thinking_mode>\n<max_thinking_length>200000</max_thinking_length>"
	maxToolDescriptionRuneLength  = 10237
	kiroDefaultAssistantContent   = "Continue, then provide a succinct summary of your work."
	kiroToolAssistantContent      = "Tool call prepared."
	defaultKiroThinkingMaxTokens  = 4000
	defaultKiroThinkingModel      = "claude-sonnet-4.5"
	defaultKiroThinkingSuffix     = "-thinking"
	kiroToolContinuationMaxRunes  = 4000
	kiroChunkNormalizationMaxLine = 160
)

type endpointConfig struct {
	URL         string
	Origin      string
	AmzTarget   string
	Name        string
	UseFallback bool
}

var endpoints = []endpointConfig{
	{
		URL:         "https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse",
		Origin:      "AI_EDITOR",
		AmzTarget:   "AmazonCodeWhispererStreamingService.GenerateAssistantResponse",
		Name:        "CodeWhisperer",
		UseFallback: false,
	},
	{
		URL:         "https://q.us-east-1.amazonaws.com/generateAssistantResponse",
		Origin:      "AI_EDITOR",
		AmzTarget:   "",
		Name:        "AmazonQ",
		UseFallback: true,
	},
}

type Service struct {
	store      *config.Manager
	auth       TokenRefresher
	pool       *account.Pool
	log        *logger.Logger
	httpClient *http.Client
}

type payload struct {
	ConversationState conversationState `json:"conversationState"`
	ProfileARN        string            `json:"profileArn,omitempty"`
	InferenceConfig   *inferenceConfig  `json:"inferenceConfig,omitempty"`
}

type conversationState struct {
	AgentContinuationID string           `json:"agentContinuationId,omitempty"`
	AgentTaskType       string           `json:"agentTaskType,omitempty"`
	ChatTriggerType     string           `json:"chatTriggerType"`
	ConversationID      string           `json:"conversationId"`
	CurrentMessage      currentMessage   `json:"currentMessage"`
	History             []historyMessage `json:"history,omitempty"`
}

type currentMessage struct {
	UserInputMessage userInputMessage `json:"userInputMessage"`
}

type userInputMessage struct {
	Content                 string                   `json:"content"`
	ModelID                 string                   `json:"modelId,omitempty"`
	Origin                  string                   `json:"origin"`
	Images                  []image                  `json:"images,omitempty"`
	UserInputMessageContext *userInputMessageContext `json:"userInputMessageContext,omitempty"`
}

type image struct {
	Format string `json:"format"`
	Source struct {
		Bytes string `json:"bytes"`
	} `json:"source"`
}

type userInputMessageContext struct {
	Tools       []toolWrapper `json:"tools,omitempty"`
	ToolResults []toolResult  `json:"toolResults,omitempty"`
}

type toolWrapper struct {
	ToolSpecification toolSpecification `json:"toolSpecification"`
}

type toolSpecification struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema inputSchema `json:"inputSchema"`
}

type inputSchema struct {
	JSON any `json:"json"`
}

type toolResult struct {
	ToolUseID string          `json:"toolUseId"`
	Content   []resultContent `json:"content"`
	Status    string          `json:"status"`
}

type resultContent struct {
	Text string `json:"text"`
}

type historyMessage struct {
	UserInputMessage         *userInputMessage         `json:"userInputMessage,omitempty"`
	AssistantResponseMessage *assistantResponseMessage `json:"assistantResponseMessage,omitempty"`
}

type assistantResponseMessage struct {
	Content  string             `json:"content"`
	ToolUses []provider.ToolUse `json:"toolUses,omitempty"`
}

type inferenceConfig struct {
	MaxTokens   int      `json:"maxTokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"topP,omitempty"`
}

type thinkingStreamSource int

const (
	thinkingSourceUnknown thinkingStreamSource = iota
	thinkingSourceReasoningEvent
	thinkingSourceTagBlock
)

type toolUseState struct {
	ToolUseID   string
	Name        string
	InputBuffer strings.Builder
}

func NewService(store *config.Manager, auth TokenRefresher, accountPool *account.Pool, log *logger.Logger, httpClient *http.Client) *Service {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	return &Service{
		store:      store,
		auth:       auth,
		pool:       accountPool,
		log:        log,
		httpClient: client,
	}
}

func (s *Service) Complete(ctx context.Context, req provider.ChatRequest) (provider.CompletionOutcome, int, string, error) {
	requestID := platform.RequestIDFromContext(ctx)
	if strings.TrimSpace(req.Model) == "" {
		s.recordRequestFailure()
		s.log.Warn("proxy", fmt.Sprintf("request_id=%q provider=%q route=%q phase=%q reason=%q", requestID, "kiro", strings.TrimSpace(req.RouteFamily), "rejected", "model is required"))
		return provider.CompletionOutcome{}, http.StatusBadRequest, "model is required", fmt.Errorf("model is required")
	}

	upstreamCandidates := s.pool.AvailableAccountsForProvider("kiro")
	if len(upstreamCandidates) == 0 {
		s.recordRequestFailure()
		reason := s.pool.ProviderUnavailableReason("kiro")
		s.log.Warn("proxy", fmt.Sprintf("request_id=%q provider=%q route=%q phase=%q reason=%q", requestID, "kiro", strings.TrimSpace(req.RouteFamily), "rejected", reason))
		return provider.CompletionOutcome{}, http.StatusServiceUnavailable, reason, fmt.Errorf(reason)
	}

	resolvedModel, thinkingRequested := parseModelAndThinking(req.Model, defaultKiroThinkingSuffix)
	normalizedModel := normalizeModel(resolvedModel)
	if strings.TrimSpace(normalizedModel) == "" {
		normalizedModel = defaultKiroThinkingModel
	}

	prompt := buildPrompt(req)
	preparedPayload, err := buildPayload(req, normalizedModel, thinkingRequested)
	if err != nil {
		s.recordRequestFailure()
		return provider.CompletionOutcome{}, http.StatusBadRequest, err.Error(), err
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = strings.TrimSpace(preparedPayload.ConversationState.CurrentMessage.UserInputMessage.Content)
	}
	if strings.TrimSpace(prompt) == "" {
		s.recordRequestFailure()
		return provider.CompletionOutcome{}, http.StatusBadRequest, "messages are empty", fmt.Errorf("messages are empty")
	}

	useFakeReasoning := shouldUseFakeReasoning(thinkingRequested)
	if useFakeReasoning {
		injectFakeReasoningPrompt(preparedPayload, defaultKiroThinkingMaxTokens)
	}

	var lastStatus int
	var lastMessage string

accountLoop:
	for _, account := range upstreamCandidates {
		accountLabel := config.AccountLabel(account)
		if s.auth != nil {
			freshAccount, err := s.auth.EnsureFreshAccount(account.ID)
			if err != nil {
				decision := provider.ClassifyHTTPFailure(http.StatusUnauthorized, err.Error())
				s.applyFailureDecision(requestID, account.ID, accountLabel, decision)
				lastStatus = decision.Status
				lastMessage = decision.Message
				continue
			}
			account = freshAccount
			accountLabel = config.AccountLabel(account)
		}
		s.log.Info("proxy", fmt.Sprintf("request_id=%q provider=%q route=%q phase=%q account=%q model=%q resolved_model=%q stream=%t fake_reasoning=%t", requestID, "kiro", strings.TrimSpace(req.RouteFamily), "attempt", accountLabel, strings.TrimSpace(req.Model), normalizedModel, req.Stream, useFakeReasoning))

		for _, endpoint := range endpoints {
			retriedAfterRefresh := false
			for {
				upstreamReq, err := s.buildRequest(ctx, account, endpoint, preparedPayload, req)
				if err != nil {
					s.recordRequestFailure()
					return provider.CompletionOutcome{}, http.StatusBadRequest, err.Error(), err
				}

				resp, err := s.httpClient.Do(upstreamReq)
				if err != nil {
					decision := provider.ClassifyTransportFailure(err)
					if !endpoint.UseFallback {
						lastStatus = decision.Status
						lastMessage = decision.Message
						break
					}
					s.applyFailureDecision(requestID, account.ID, accountLabel, decision)
					lastStatus = decision.Status
					lastMessage = decision.Message
					break
				}

				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					data, _ := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					message := compactBody(data)
					if message == "" {
						message = fmt.Sprintf("kiro upstream returned %d", resp.StatusCode)
					}
					decision := provider.ClassifyHTTPFailure(resp.StatusCode, message)

					if decision.Class == provider.FailureAuthRefreshable && !retriedAfterRefresh && s.auth != nil && strings.TrimSpace(account.RefreshToken) != "" {
						refreshedAccount, refreshErr := s.auth.RefreshAccount(account.ID)
						if refreshErr == nil {
							account = refreshedAccount
							accountLabel = config.AccountLabel(account)
							retriedAfterRefresh = true
							s.log.Info("auth", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q endpoint=%q", requestID, "kiro", "token_refreshed_retry", accountLabel, endpoint.Name))
							continue
						}
						s.log.Warn("auth", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q reason=%q", requestID, "kiro", "token_refresh_failed", accountLabel, refreshErr.Error()))
						decision = provider.ClassifyHTTPFailure(http.StatusUnauthorized, refreshErr.Error())
					}

					if decision.Class == provider.FailureRequestShape {
						s.recordRequestFailure()
						return provider.CompletionOutcome{}, decision.Status, decision.Message, fmt.Errorf(decision.Message)
					}
					if decision.Class == provider.FailureQuotaCooldown && !endpoint.UseFallback {
						lastStatus = decision.Status
						lastMessage = decision.Message
						break
					}
					if decision.Class == provider.FailureRetryableTransport && !endpoint.UseFallback {
						lastStatus = decision.Status
						lastMessage = decision.Message
						break
					}

					s.applyFailureDecision(requestID, account.ID, accountLabel, decision)
					lastStatus = decision.Status
					lastMessage = decision.Message
					if decision.Class == provider.FailureDurableDisabled || decision.Class == provider.FailureAuthRefreshable {
						continue accountLoop
					}
					break
				}

				text, thinking, toolUses, err := parseEventStream(resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					decision := provider.ClassifyTransportFailure(err)
					s.applyFailureDecision(requestID, account.ID, accountLabel, decision)
					lastStatus = decision.Status
					lastMessage = decision.Message
					break
				}

				var thinkingSource thinkingStreamSource
				if strings.TrimSpace(thinking) != "" {
					_ = allowReasoningSource(&thinkingSource)
				}

				regularContent, taggedThinking := ExtractTaggedThinking(text)
				if strings.TrimSpace(taggedThinking) != "" {
					if allowTagSource(&thinkingSource) {
						text = regularContent
						if strings.TrimSpace(thinking) == "" {
							thinking = taggedThinking
						}
					} else {
						text = regularContent
					}
				}

				if len(toolUses) == 0 {
					cleaned, parsedToolUses := ParseBracketToolCalls(text)
					if len(parsedToolUses) > 0 {
						text = cleaned
						toolUses = parsedToolUses
					}
				}
				if len(toolUses) > 0 {
					toolUses = DeduplicateToolCalls(toolUses)
				}

				if strings.TrimSpace(thinking) == "" && useFakeReasoning {
					thinking = buildFallbackFakeReasoning(prompt, text, defaultKiroThinkingMaxTokens)
				}

				usage := config.ProxyStats{}
				usage.PromptTokens = estimateTokens(prompt)
				completionInput := text
				if strings.TrimSpace(thinking) != "" {
					completionInput = thinking + "\n" + text
				}
				for _, toolUse := range toolUses {
					completionInput += "\n" + toolUse.Name
					if encoded, err := json.Marshal(toolUse.Input); err == nil {
						completionInput += string(encoded)
					}
				}
				usage.CompletionTokens = estimateTokens(completionInput)
				usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

				outcome := provider.CompletionOutcome{
					Text:         text,
					Thinking:     thinking,
					ToolUses:     toolUses,
					Usage:        usage,
					ID:           "chatcmpl-" + uuid.NewString(),
					Model:        req.Model,
					Provider:     "kiro",
					AccountID:    account.ID,
					AccountLabel: accountLabel,
				}

				s.markSuccess(requestID, account.ID, accountLabel, usage)
				return outcome, 0, "", nil
			}
		}
	}

	snapshot := s.pool.AvailabilitySnapshot("kiro")
	if snapshot.ReadyCount == 0 {
		lastStatus = http.StatusServiceUnavailable
		lastMessage = s.pool.ProviderUnavailableReason("kiro")
	}
	if lastStatus == 0 {
		lastStatus = http.StatusServiceUnavailable
	}
	if strings.TrimSpace(lastMessage) == "" {
		lastMessage = "all kiro accounts failed"
	}
	s.recordRequestFailure()
	s.log.Warn("proxy", fmt.Sprintf("request_id=%q provider=%q route=%q phase=%q reason=%q", requestID, "kiro", strings.TrimSpace(req.RouteFamily), "failed", lastMessage))
	return provider.CompletionOutcome{}, lastStatus, lastMessage, fmt.Errorf(lastMessage)
}

func (s *Service) buildRequest(ctx context.Context, account config.Account, endpoint endpointConfig, payload *payload, req provider.ChatRequest) (*http.Request, error) {
	if payload == nil {
		return nil, fmt.Errorf("messages are empty")
	}
	applyPayloadOrigin(payload, endpoint.Origin)
	payload.ProfileARN = effectiveKiroProfileARN(account, req)
	if strings.TrimSpace(payload.ConversationState.CurrentMessage.UserInputMessage.Content) == "" {
		return nil, fmt.Errorf("messages are empty")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/vnd.amazon.eventstream")
	if strings.TrimSpace(endpoint.AmzTarget) != "" {
		httpReq.Header.Set("X-Amz-Target", endpoint.AmzTarget)
	}
	httpReq.Header.Set("User-Agent", fmt.Sprintf("aws-sdk-js/1.0.27 ua/2.1 os/linux lang/js md/nodejs#22.21.1 api/codewhispererstreaming#1.0.27 m/E KiroIDE-%s", kiroVersion))
	httpReq.Header.Set("X-Amz-User-Agent", fmt.Sprintf("aws-sdk-js/1.0.27 KiroIDE %s", kiroVersion))
	httpReq.Header.Set("Amz-Sdk-Request", "attempt=1; max=3")
	httpReq.Header.Set("Amz-Sdk-Invocation-Id", uuid.NewString())
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(account.AccessToken))

	return httpReq, nil
}

func parseEventStream(body io.Reader) (string, string, []provider.ToolUse, error) {
	var textBuilder strings.Builder
	var thinkingBuilder strings.Builder
	toolUses := make([]provider.ToolUse, 0)
	var currentToolUse *toolUseState
	lastText := ""
	lastThinking := ""

	for {
		prelude := make([]byte, 12)
		_, err := io.ReadFull(body, prelude)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return "", "", nil, err
		}

		totalLength := int(prelude[0])<<24 | int(prelude[1])<<16 | int(prelude[2])<<8 | int(prelude[3])
		headersLength := int(prelude[4])<<24 | int(prelude[5])<<16 | int(prelude[6])<<8 | int(prelude[7])
		if totalLength < 16 {
			continue
		}

		remaining := totalLength - 12
		if remaining <= 0 {
			continue
		}
		message := make([]byte, remaining)
		if _, err := io.ReadFull(body, message); err != nil {
			return "", "", nil, err
		}

		if headersLength < 0 || headersLength > len(message)-4 {
			continue
		}

		eventType := extractEventType(message[:headersLength])
		payloadBytes := message[headersLength : len(message)-4]
		if len(payloadBytes) == 0 {
			continue
		}

		var event map[string]any
		if err := json.Unmarshal(payloadBytes, &event); err != nil {
			continue
		}

		switch eventType {
		case "assistantResponseEvent":
			if content, ok := event["content"].(string); ok && strings.TrimSpace(content) != "" {
				delta := normalizeChunk(content, &lastText)
				if delta != "" {
					textBuilder.WriteString(delta)
				}
			}
		case "reasoningContentEvent":
			thought := ""
			if text, ok := event["text"].(string); ok {
				thought = text
			}
			if thought == "" {
				if text, ok := event["reasoningContent"].(string); ok {
					thought = text
				}
			}
			if strings.TrimSpace(thought) != "" {
				delta := normalizeChunk(thought, &lastThinking)
				if delta != "" {
					thinkingBuilder.WriteString(delta)
				}
			}
		case "toolUseEvent":
			currentToolUse = handleToolUseEvent(event, currentToolUse, &toolUses)
		case "error", "exception":
			if msg := extractEventErrorMessage(event); msg != "" {
				return "", "", nil, fmt.Errorf(msg)
			}
		}

		if msg := extractEventErrorMessage(event); msg != "" && (eventType == "error" || eventType == "exception") {
			return "", "", nil, fmt.Errorf(msg)
		}
	}

	if currentToolUse != nil {
		flushToolUseState(currentToolUse, &toolUses)
	}
	toolUses = DeduplicateToolCalls(toolUses)

	return textBuilder.String(), thinkingBuilder.String(), toolUses, nil
}

func handleToolUseEvent(event map[string]any, current *toolUseState, out *[]provider.ToolUse) *toolUseState {
	toolUseID, _ := event["toolUseId"].(string)
	name, _ := event["name"].(string)
	stop, _ := event["stop"].(bool)

	trimmedID := strings.TrimSpace(toolUseID)
	trimmedName := strings.TrimSpace(name)
	if trimmedID != "" && trimmedName != "" {
		if current == nil {
			current = &toolUseState{ToolUseID: trimmedID, Name: trimmedName}
		} else if current.ToolUseID != trimmedID {
			flushToolUseState(current, out)
			current = &toolUseState{ToolUseID: trimmedID, Name: trimmedName}
		} else if current.Name == "" {
			current.Name = trimmedName
		}
	}

	if current != nil {
		switch input := event["input"].(type) {
		case string:
			current.InputBuffer.WriteString(input)
		case map[string]any:
			encoded, _ := json.Marshal(input)
			current.InputBuffer.Reset()
			_, _ = current.InputBuffer.Write(encoded)
		}
	}

	if stop && current != nil {
		flushToolUseState(current, out)
		return nil
	}

	return current
}

func flushToolUseState(state *toolUseState, out *[]provider.ToolUse) {
	if state == nil || out == nil {
		return
	}

	id := strings.TrimSpace(state.ToolUseID)
	name := strings.TrimSpace(state.Name)
	if id == "" || name == "" {
		return
	}

	input := map[string]any{}
	if state.InputBuffer.Len() > 0 {
		_ = json.Unmarshal([]byte(state.InputBuffer.String()), &input)
	}

	*out = append(*out, provider.ToolUse{ID: id, Name: name, Input: input})
}

func extractEventErrorMessage(event map[string]any) string {
	if event == nil {
		return ""
	}
	if msg, ok := event["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}
	if msg, ok := event["errorMessage"].(string); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}
	if rawErr, exists := event["error"]; exists {
		switch typed := rawErr.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case map[string]any:
			if msg, ok := typed["message"].(string); ok && strings.TrimSpace(msg) != "" {
				return strings.TrimSpace(msg)
			}
		}
	}
	return ""
}

func extractEventType(headers []byte) string {
	offset := 0
	for offset < len(headers) {
		nameLen := int(headers[offset])
		offset++
		if offset+nameLen > len(headers) {
			break
		}
		name := string(headers[offset : offset+nameLen])
		offset += nameLen
		if offset >= len(headers) {
			break
		}

		valueType := headers[offset]
		offset++
		if valueType == 7 {
			if offset+2 > len(headers) {
				break
			}
			valueLen := int(headers[offset])<<8 | int(headers[offset+1])
			offset += 2
			if offset+valueLen > len(headers) {
				break
			}
			value := string(headers[offset : offset+valueLen])
			offset += valueLen
			if name == ":event-type" {
				return value
			}
			continue
		}

		skipSizes := map[byte]int{0: 0, 1: 0, 2: 1, 3: 2, 4: 4, 5: 8, 8: 8, 9: 16}
		if valueType == 6 {
			if offset+2 > len(headers) {
				break
			}
			l := int(headers[offset])<<8 | int(headers[offset+1])
			offset += 2 + l
		} else if skip, ok := skipSizes[valueType]; ok {
			offset += skip
		} else {
			break
		}
	}
	return ""
}

func normalizeChunk(chunk string, previous *string) string {
	if chunk == "" {
		return ""
	}
	if previous == nil {
		return chunk
	}

	prev := *previous
	if prev == "" {
		*previous = chunk
		return chunk
	}
	if chunk == prev {
		return ""
	}
	if strings.HasPrefix(chunk, prev) {
		delta := chunk[len(prev):]
		*previous = chunk
		return delta
	}
	if strings.HasPrefix(prev, chunk) {
		return ""
	}

	maxOverlap := 0
	maxLen := len(prev)
	if len(chunk) < maxLen {
		maxLen = len(chunk)
	}
	for i := maxLen; i > 0; i-- {
		if strings.HasSuffix(prev, chunk[:i]) {
			maxOverlap = i
			break
		}
	}

	*previous = chunk
	if maxOverlap > 0 {
		return chunk[maxOverlap:]
	}
	return chunk
}

func allowReasoningSource(source *thinkingStreamSource) bool {
	if source == nil {
		return false
	}
	if *source == thinkingSourceTagBlock {
		return false
	}
	*source = thinkingSourceReasoningEvent
	return true
}

func allowTagSource(source *thinkingStreamSource) bool {
	if source == nil {
		return false
	}
	if *source == thinkingSourceReasoningEvent {
		return false
	}
	if *source == thinkingSourceUnknown {
		*source = thinkingSourceTagBlock
	}
	return *source == thinkingSourceTagBlock
}

func parseModelAndThinking(model string, suffix string) (string, bool) {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return "", false
	}

	resolvedSuffix := strings.TrimSpace(suffix)
	if resolvedSuffix == "" {
		resolvedSuffix = defaultKiroThinkingSuffix
	}

	lowerModel := strings.ToLower(trimmed)
	lowerSuffix := strings.ToLower(resolvedSuffix)
	if strings.HasSuffix(lowerModel, lowerSuffix) {
		base := strings.TrimSpace(trimmed[:len(trimmed)-len(lowerSuffix)])
		if base != "" {
			return base, true
		}
	}

	return trimmed, false
}

func normalizeModel(model string) string {
	resolved, _ := parseModelAndThinking(model, defaultKiroThinkingSuffix)
	normalized := strings.ToLower(strings.TrimSpace(resolved))

	switch {
	case strings.HasPrefix(normalized, "claude-sonnet-4-20250514"):
		return "claude-sonnet-4"
	case strings.HasPrefix(normalized, "claude-3-5-sonnet") || strings.HasPrefix(normalized, "claude-sonnet-4.5") || strings.HasPrefix(normalized, "claude-sonnet-4-5"):
		return "claude-sonnet-4.5"
	case strings.HasPrefix(normalized, "claude-3-sonnet") || strings.HasPrefix(normalized, "claude-sonnet-4"):
		return "claude-sonnet-4"
	case strings.HasPrefix(normalized, "claude-3-haiku") || strings.HasPrefix(normalized, "claude-haiku-4.5") || strings.HasPrefix(normalized, "claude-haiku-4-5"):
		return "claude-haiku-4.5"
	case strings.HasPrefix(normalized, "claude-3-opus") || strings.HasPrefix(normalized, "claude-opus-4.5") || strings.HasPrefix(normalized, "claude-opus-4-5"):
		return "claude-opus-4.5"
	default:
		if strings.TrimSpace(resolved) != "" {
			return strings.TrimSpace(resolved)
		}
		return strings.TrimSpace(model)
	}
}

func buildPrompt(req provider.ChatRequest) string {
	sections := make([]string, 0, len(req.Messages))
	for _, message := range req.Messages {
		text := strings.TrimSpace(messageToText(message.Content))
		if text == "" && len(message.ToolCalls) > 0 {
			parts := make([]string, 0, len(message.ToolCalls))
			for _, toolCall := range message.ToolCalls {
				name := strings.TrimSpace(toolCall.Function.Name)
				if name == "" {
					continue
				}
				arguments := strings.TrimSpace(toolCall.Function.Arguments)
				if arguments != "" {
					parts = append(parts, name+arguments)
				} else {
					parts = append(parts, name)
				}
			}
			text = strings.TrimSpace(strings.Join(parts, "\n"))
		}
		if text == "" {
			continue
		}

		role := strings.ToLower(strings.TrimSpace(message.Role))
		label := "User"
		switch role {
		case "system", "developer":
			label = "System"
		case "assistant":
			label = "Assistant"
		case "tool":
			label = "Tool"
		}
		sections = append(sections, label+":\n"+text)
	}

	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func buildPayload(req provider.ChatRequest, modelID string, thinking bool) (*payload, error) {
	if strings.TrimSpace(modelID) == "" {
		modelID = defaultKiroThinkingModel
	}
	history, current := convertMessagesToKiro(req.Messages, req.Tools, modelID)
	if current == nil {
		current = &historyMessage{UserInputMessage: &userInputMessage{Content: "continue", ModelID: modelID}}
	}
	if current.UserInputMessage == nil {
		return nil, fmt.Errorf("messages are empty")
	}

	if strings.TrimSpace(current.UserInputMessage.Content) == "" {
		current.UserInputMessage.Content = "continue"
	}
	current.UserInputMessage.ModelID = modelID
	current.UserInputMessage.Origin = kiroConversationOrigin

	out := &payload{}
	out.ConversationState.ChatTriggerType = "MANUAL"
	out.ConversationState.ConversationID = extractConversationID(req.Metadata, modelID, "", "")
	out.ConversationState.AgentContinuationID = extractContinuationID(req.Metadata)
	out.ConversationState.CurrentMessage.UserInputMessage = *current.UserInputMessage
	if len(history) > 0 {
		out.ConversationState.History = history
	}

	kiroTools := convertTools(req.Tools)
	currentToolResults := []toolResult{}
	if current.UserInputMessage.UserInputMessageContext != nil {
		currentToolResults = deduplicateToolResults(current.UserInputMessage.UserInputMessageContext.ToolResults)
	}
	if len(kiroTools) > 0 || len(currentToolResults) > 0 {
		out.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext = &userInputMessageContext{}
		if len(kiroTools) > 0 {
			out.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools = kiroTools
		}
		if len(currentToolResults) > 0 {
			out.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.ToolResults = currentToolResults
		}
	}
	if out.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext != nil && len(out.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools) == 0 && len(out.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.ToolResults) == 0 {
		out.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext = nil
	}

	if req.MaxTokens != nil || req.Temperature != nil || req.TopP != nil {
		out.InferenceConfig = &inferenceConfig{}
		if req.MaxTokens != nil && *req.MaxTokens > 0 {
			out.InferenceConfig.MaxTokens = *req.MaxTokens
		}
		if req.Temperature != nil {
			out.InferenceConfig.Temperature = req.Temperature
		}
		if req.TopP != nil {
			out.InferenceConfig.TopP = req.TopP
		}
	}

	if strings.TrimSpace(out.ConversationState.CurrentMessage.UserInputMessage.Content) == "" {
		return nil, fmt.Errorf("messages are empty")
	}

	return out, nil
}

func convertMessagesToKiro(messages []provider.Message, tools []provider.Tool, modelID string) ([]historyMessage, *historyMessage) {
	type pendingState struct {
		role          string
		userContent   []string
		assistantText []string
		toolResults   []toolResult
		assistantUses []provider.ToolUse
	}

	state := pendingState{}
	history := make([]historyMessage, 0, len(messages))

	flush := func() {
		switch state.role {
		case "user":
			content := strings.TrimSpace(strings.Join(state.userContent, "\n\n"))
			if content == "" {
				content = "continue"
			}
			entry := historyMessage{UserInputMessage: &userInputMessage{Content: content, ModelID: modelID}}
			toolResults := deduplicateToolResults(state.toolResults)
			if len(toolResults) > 0 {
				entry.UserInputMessage.UserInputMessageContext = &userInputMessageContext{ToolResults: toolResults}
			}
			history = append(history, entry)
		case "assistant":
			content := strings.TrimSpace(strings.Join(state.assistantText, "\n\n"))
			if content == "" {
				content = "..."
			}
			entry := historyMessage{AssistantResponseMessage: &assistantResponseMessage{Content: content}}
			if len(state.assistantUses) > 0 {
				entry.AssistantResponseMessage.ToolUses = DeduplicateToolCalls(state.assistantUses)
			}
			history = append(history, entry)
		}
		state = pendingState{}
	}

	for _, msg := range messages {
		role := normalizeKiroMessageRole(msg.Role)
		if role == "" {
			continue
		}
		if state.role != "" && state.role != role {
			flush()
		}
		state.role = role

		switch role {
		case "assistant":
			if text := strings.TrimSpace(extractMessageText(msg.Content)); text != "" {
				state.assistantText = append(state.assistantText, text)
			}
			if toolUses := convertToolCalls(msg.ToolCalls); len(toolUses) > 0 {
				state.assistantUses = append(state.assistantUses, toolUses...)
			}
		case "user":
			if strings.EqualFold(strings.TrimSpace(msg.Role), "tool") {
				toolID := strings.TrimSpace(msg.ToolCallID)
				if toolID == "" {
					toolID = "toolu_" + uuid.NewString()[:8]
				}
				state.toolResults = append(state.toolResults, toolResult{ToolUseID: toolID, Status: "success", Content: []resultContent{{Text: firstNonEmpty(strings.TrimSpace(messageToText(msg.Content)), "continue")}}})
				continue
			}
			if text := strings.TrimSpace(extractMessageText(msg.Content)); text != "" {
				state.userContent = append(state.userContent, text)
			}
		}
	}
	if state.role != "" {
		flush()
	}

	var current *historyMessage
	for idx := len(history) - 1; idx >= 0; idx-- {
		if history[idx].UserInputMessage != nil {
			current = &history[idx]
			history = append(history[:idx], history[idx+1:]...)
			break
		}
	}

	for idx := range history {
		if history[idx].UserInputMessage == nil {
			continue
		}
		history[idx].UserInputMessage.Origin = ""
		if history[idx].UserInputMessage.UserInputMessageContext != nil {
			history[idx].UserInputMessage.UserInputMessageContext.Tools = nil
			if len(history[idx].UserInputMessage.UserInputMessageContext.ToolResults) == 0 {
				history[idx].UserInputMessage.UserInputMessageContext = nil
			}
		}
	}

	if current != nil && current.UserInputMessage != nil {
		if current.UserInputMessage.UserInputMessageContext == nil {
			current.UserInputMessage.UserInputMessageContext = &userInputMessageContext{}
		}
		if len(tools) > 0 {
			current.UserInputMessage.UserInputMessageContext.Tools = convertTools(tools)
		}
		if len(current.UserInputMessage.UserInputMessageContext.Tools) == 0 && len(current.UserInputMessage.UserInputMessageContext.ToolResults) == 0 {
			current.UserInputMessage.UserInputMessageContext = nil
		}
	}

	return history, current
}

func normalizeKiroMessageRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return "assistant"
	case "system", "developer", "user", "tool", "":
		return "user"
	default:
		return "user"
	}
}

func convertToolCalls(toolCalls []provider.ToolCall) []provider.ToolUse {
	if len(toolCalls) == 0 {
		return nil
	}

	result := make([]provider.ToolUse, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		name := strings.TrimSpace(toolCall.Function.Name)
		if name == "" {
			continue
		}
		if strings.EqualFold(name, "web_search") {
			name = "remote_web_search"
		}

		id := strings.TrimSpace(toolCall.ID)
		if id == "" {
			id = "toolu_" + uuid.NewString()[:8]
		}

		input := map[string]any{}
		arguments := strings.TrimSpace(toolCall.Function.Arguments)
		if arguments != "" {
			if err := json.Unmarshal([]byte(arguments), &input); err != nil {
				input = map[string]any{}
			}
		}

		result = append(result, provider.ToolUse{ID: id, Name: name, Input: input})
	}

	return result
}

func convertTools(tools []provider.Tool) []toolWrapper {
	if len(tools) == 0 {
		return nil
	}

	result := make([]toolWrapper, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "" && !strings.EqualFold(tool.Type, "function") {
			continue
		}

		name := strings.TrimSpace(tool.Function.Name)
		if name == "" {
			continue
		}
		if strings.EqualFold(name, "web_search") {
			name = "remote_web_search"
		}

		description := strings.TrimSpace(tool.Function.Description)
		if description == "" {
			description = fmt.Sprintf("Tool: %s", name)
		}
		descRunes := []rune(description)
		if len(descRunes) > maxToolDescriptionRuneLength {
			description = string(descRunes[:maxToolDescriptionRuneLength]) + "..."
		}

		result = append(result, toolWrapper{
			ToolSpecification: toolSpecification{
				Name:        shortenToolName(name),
				Description: description,
				InputSchema: inputSchema{JSON: sanitizeSchema(tool.Function.Parameters)},
			},
		})
	}

	return result
}

func sanitizeSchema(schema any) any {
	if schema == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}}
	}
	object, ok := schema.(map[string]any)
	if !ok {
		return schema
	}

	result := make(map[string]any, len(object))
	for key, value := range object {
		if key == "additionalProperties" {
			continue
		}

		switch typed := value.(type) {
		case map[string]any:
			result[key] = sanitizeSchema(typed)
		case []any:
			sanitized := make([]any, len(typed))
			for idx, item := range typed {
				sanitized[idx] = sanitizeSchema(item)
			}
			result[key] = sanitized
		default:
			result[key] = value
		}
	}
	if strings.TrimSpace(fmt.Sprint(result["type"])) == "" {
		result["type"] = "object"
	}
	if _, ok := result["properties"]; !ok {
		result["properties"] = map[string]any{}
	}
	if _, ok := result["required"]; !ok {
		result["required"] = []any{}
	}

	return result
}

func shortenToolName(name string) string {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) <= 64 {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "mcp__") {
		last := strings.LastIndex(trimmed, "__")
		if last > 5 {
			shortened := "mcp__" + trimmed[last+2:]
			if len(shortened) <= 64 {
				return shortened
			}
		}
	}
	return trimmed[:64]
}

func ensureAlternatingHistory(history []historyMessage) []historyMessage {
	if len(history) < 2 {
		return history
	}

	result := make([]historyMessage, 0, len(history)+4)
	result = append(result, history[0])
	for idx := 1; idx < len(history); idx++ {
		prev := result[len(result)-1]
		current := history[idx]

		if prev.UserInputMessage != nil && current.UserInputMessage != nil {
			result = append(result, historyMessage{AssistantResponseMessage: &assistantResponseMessage{Content: kiroDefaultAssistantContent}})
		}
		if prev.AssistantResponseMessage != nil && current.AssistantResponseMessage != nil {
			result = append(result, historyMessage{UserInputMessage: &userInputMessage{Content: kiroFallbackUserContent, Origin: kiroConversationOrigin}})
		}

		result = append(result, current)
	}

	return result
}

func buildConversationID(modelID string, systemPrompt string, anchor string) string {
	trimmedAnchor := strings.TrimSpace(anchor)
	if trimmedAnchor == "" {
		return uuid.NewString()
	}
	seed := strings.Join([]string{strings.TrimSpace(modelID), strings.TrimSpace(systemPrompt), trimmedAnchor}, "\n")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed)).String()
}

func firstConversationAnchor(messages []provider.Message) string {
	for _, msg := range messages {
		if !strings.EqualFold(strings.TrimSpace(msg.Role), "user") {
			continue
		}
		if text := strings.TrimSpace(extractMessageText(msg.Content)); text != "" {
			return text
		}
	}

	for _, msg := range messages {
		if text := strings.TrimSpace(extractMessageText(msg.Content)); text != "" {
			return text
		}
	}

	return ""
}

func buildToolContinuation(toolResults []toolResult) string {
	if len(toolResults) == 0 {
		return kiroFallbackUserContent
	}

	parts := make([]string, 0, len(toolResults))
	for _, result := range toolResults {
		for _, item := range result.Content {
			text := strings.TrimSpace(item.Text)
			if text != "" {
				parts = append(parts, text)
			}
		}
	}

	if len(parts) == 0 {
		return kiroFallbackUserContent
	}

	joined := strings.Join(parts, "\n\n")
	runes := []rune(joined)
	if len(runes) > kiroToolContinuationMaxRunes {
		return string(runes[:kiroToolContinuationMaxRunes])
	}
	return joined
}

func extractMessageText(content any) string {
	switch typed := content.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		if nested, ok := typed["content"]; ok {
			if text := extractMessageText(nested); text != "" {
				return text
			}
		}
		if text, ok := typed["text"].(string); ok {
			return strings.TrimSpace(text)
		}
		if value, ok := typed["value"].(string); ok {
			return strings.TrimSpace(value)
		}
		raw, _ := json.Marshal(typed)
		return strings.TrimSpace(string(raw))
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(extractMessageText(item))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		raw, _ := json.Marshal(typed)
		return strings.TrimSpace(string(raw))
	}
}

func filterOrphanedToolResults(history []historyMessage, currentToolResults *[]toolResult) {
	validToolUseIDs := make(map[string]struct{})
	for _, entry := range history {
		if entry.AssistantResponseMessage == nil {
			continue
		}
		for _, toolUse := range entry.AssistantResponseMessage.ToolUses {
			id := strings.TrimSpace(toolUse.ID)
			if id == "" {
				continue
			}
			validToolUseIDs[id] = struct{}{}
		}
	}

	if len(validToolUseIDs) == 0 {
		for idx := range history {
			if history[idx].UserInputMessage != nil && history[idx].UserInputMessage.UserInputMessageContext != nil {
				history[idx].UserInputMessage.UserInputMessageContext.ToolResults = nil
				if len(history[idx].UserInputMessage.UserInputMessageContext.Tools) == 0 {
					history[idx].UserInputMessage.UserInputMessageContext = nil
				}
			}
		}
		if currentToolResults != nil {
			*currentToolResults = nil
		}
		return
	}

	for idx := range history {
		if history[idx].UserInputMessage == nil || history[idx].UserInputMessage.UserInputMessageContext == nil {
			continue
		}

		filtered := make([]toolResult, 0, len(history[idx].UserInputMessage.UserInputMessageContext.ToolResults))
		for _, result := range history[idx].UserInputMessage.UserInputMessageContext.ToolResults {
			if _, ok := validToolUseIDs[strings.TrimSpace(result.ToolUseID)]; ok {
				filtered = append(filtered, result)
			}
		}
		history[idx].UserInputMessage.UserInputMessageContext.ToolResults = deduplicateToolResults(filtered)
		if len(history[idx].UserInputMessage.UserInputMessageContext.ToolResults) == 0 && len(history[idx].UserInputMessage.UserInputMessageContext.Tools) == 0 {
			history[idx].UserInputMessage.UserInputMessageContext = nil
		}
	}

	if currentToolResults != nil {
		filtered := make([]toolResult, 0, len(*currentToolResults))
		for _, result := range *currentToolResults {
			if _, ok := validToolUseIDs[strings.TrimSpace(result.ToolUseID)]; ok {
				filtered = append(filtered, result)
			}
		}
		*currentToolResults = deduplicateToolResults(filtered)
	}
}

func deduplicateToolResults(toolResults []toolResult) []toolResult {
	if len(toolResults) <= 1 {
		return toolResults
	}

	seen := make(map[string]struct{}, len(toolResults))
	result := make([]toolResult, 0, len(toolResults))
	for _, toolResult := range toolResults {
		id := strings.TrimSpace(toolResult.ToolUseID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, toolResult)
	}
	return result
}

func injectFakeReasoningPrompt(payload *payload, maxTokens int) {
	if payload == nil {
		return
	}
	if maxTokens <= 0 {
		maxTokens = defaultKiroThinkingMaxTokens
	}
	injection := fmt.Sprintf("<thinking_mode>enabled</thinking_mode>\n<max_thinking_length>%d</max_thinking_length>\n<thinking_instruction>Think step by step inside thinking tags before responding.</thinking_instruction>", maxTokens)
	content := strings.TrimSpace(payload.ConversationState.CurrentMessage.UserInputMessage.Content)
	if content == "" {
		payload.ConversationState.CurrentMessage.UserInputMessage.Content = injection
		return
	}
	payload.ConversationState.CurrentMessage.UserInputMessage.Content = injection + "\n\n" + content
}

func applyPayloadOrigin(payload *payload, origin string) {
	if payload == nil {
		return
	}
	resolvedOrigin := strings.TrimSpace(origin)
	if resolvedOrigin == "" {
		resolvedOrigin = kiroConversationOrigin
	}

	payload.ConversationState.CurrentMessage.UserInputMessage.Origin = resolvedOrigin
}

func shouldUseFakeReasoning(thinkingRequested bool) bool {
	return thinkingRequested
}

func buildFallbackFakeReasoning(prompt string, answer string, maxTokens int) string {
	if maxTokens <= 0 {
		maxTokens = defaultKiroThinkingMaxTokens
	}
	promptSummary := strings.TrimSpace(prompt)
	answerSummary := strings.TrimSpace(answer)
	if len([]rune(promptSummary)) > 220 {
		promptSummary = string([]rune(promptSummary)[:220]) + "..."
	}
	if len([]rune(answerSummary)) > 220 {
		answerSummary = string([]rune(answerSummary)[:220]) + "..."
	}
	if promptSummary == "" {
		promptSummary = "user request"
	}
	if answerSummary == "" {
		answerSummary = "assistant response"
	}
	result := "Analyze user intent and constraints from the request.\n" +
		"Plan a concise response that addresses the core ask directly.\n" +
		"Synthesize output clearly: " + answerSummary
	if len(result) > maxTokens {
		return result[:maxTokens]
	}
	return result
}

func estimateTokens(text string) int {
	runeCount := len([]rune(strings.TrimSpace(text)))
	if runeCount <= 0 {
		return 1
	}
	estimated := runeCount / 4
	if estimated <= 0 {
		estimated = 1
	}
	return estimated
}

func messageToText(content any) string {
	switch typed := content.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				text, _ := object["text"].(string)
				if strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		data, _ := json.Marshal(typed)
		return strings.TrimSpace(string(data))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func compactBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		if msg, ok := payload["message"].(string); ok && strings.TrimSpace(msg) != "" {
			trimmed = strings.TrimSpace(msg)
		} else if nested, ok := payload["error"].(map[string]any); ok {
			if msg, ok := nested["message"].(string); ok && strings.TrimSpace(msg) != "" {
				trimmed = strings.TrimSpace(msg)
			}
		}
	}
	if len(trimmed) > 240 {
		return trimmed[:240] + "..."
	}
	return trimmed
}

func extractConversationID(metadata map[string]any, modelID string, systemPrompt string, anchor string) string {
	if metadata != nil {
		if conversationID, ok := metadata["conversationId"].(string); ok && strings.TrimSpace(conversationID) != "" {
			return strings.TrimSpace(conversationID)
		}
	}
	return uuid.NewString()
}

func extractContinuationID(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	if continuationID, ok := metadata["continuationId"].(string); ok && strings.TrimSpace(continuationID) != "" {
		return strings.TrimSpace(continuationID)
	}
	return ""
}

func effectiveKiroProfileARN(account config.Account, req provider.ChatRequest) string {
	if req.Metadata != nil {
		if profileARN, ok := req.Metadata["profileArn"].(string); ok && strings.TrimSpace(profileARN) != "" {
			trimmed := strings.TrimSpace(profileARN)
			if strings.HasPrefix(trimmed, "arn:") {
				return trimmed
			}
			return ""
		}
	}
	trimmedAccountID := strings.TrimSpace(account.AccountID)
	if strings.HasPrefix(trimmedAccountID, "arn:") {
		return trimmedAccountID
	}
	return ""
}

func (s *Service) markSuccess(requestID string, accountID string, accountLabel string, usage config.ProxyStats) {
	now := time.Now().Unix()
	_ = s.store.UpdateAccount(accountID, func(a *config.Account) {
		a.RequestCount++
		a.PromptTokens += usage.PromptTokens
		a.CompletionTokens += usage.CompletionTokens
		a.TotalTokens += usage.TotalTokens
		a.LastUsed = now
		a.CooldownUntil = 0
		a.ConsecutiveFailures = 0
		a.Banned = false
		a.BannedReason = ""
		a.HealthState = config.AccountHealthReady
		a.HealthReason = ""
		a.LastError = ""
		if a.Quota.Status == "exhausted" || a.Quota.Status == "unknown" || a.Quota.Status == "degraded" {
			a.Quota.Status = "healthy"
			a.Quota.Summary = "Recent request succeeded."
			a.Quota.Source = firstNonEmpty(a.Quota.Source, "runtime")
			a.Quota.Error = ""
			a.Quota.LastCheckedAt = now
			for i := range a.Quota.Buckets {
				if a.Quota.Buckets[i].Status == "exhausted" || a.Quota.Buckets[i].Status == "unknown" {
					a.Quota.Buckets[i].Status = "healthy"
				}
			}
		}
	})

	_ = s.store.UpdateStats(func(stats *config.ProxyStats) {
		stats.TotalRequests++
		stats.SuccessRequests++
		stats.PromptTokens += usage.PromptTokens
		stats.CompletionTokens += usage.CompletionTokens
		stats.TotalTokens += usage.TotalTokens
		stats.LastRequestAt = now
	})
	s.log.Info("proxy", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q prompt_tokens=%d completion_tokens=%d total_tokens=%d", requestID, "kiro", "success", accountLabel, usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens))
}

func (s *Service) markTransientFailure(requestID string, accountID string, accountLabel string, err error) {
	breakerEnabled := s.store.CircuitBreaker()
	steps := s.store.CircuitSteps()
	now := time.Now().Unix()
	appliedCooldown := time.Duration(0)
	appliedStep := 0
	_ = s.store.UpdateAccount(accountID, func(a *config.Account) {
		a.ErrorCount++
		a.LastError = err.Error()
		a.LastFailureAt = now
		a.Quota.Status = firstNonEmpty(a.Quota.Status, "degraded")
		a.Quota.Summary = err.Error()
		a.Quota.Source = firstNonEmpty(a.Quota.Source, "runtime")
		a.Quota.Error = err.Error()
		a.Quota.LastCheckedAt = now
		if !breakerEnabled {
			a.ConsecutiveFailures = 0
			a.CooldownUntil = 0
			a.HealthState = config.AccountHealthReady
			a.HealthReason = ""
			return
		}
		nextFailures := a.ConsecutiveFailures + 1
		appliedCooldown = provider.CircuitCooldown(steps, nextFailures)
		appliedStep = nextFailures
		a.ConsecutiveFailures = nextFailures
		a.CooldownUntil = now + int64(appliedCooldown/time.Second)
		a.HealthState = config.AccountHealthCooldownTransient
		a.HealthReason = err.Error()
	})
	if breakerEnabled && appliedCooldown > 0 {
		cappedStep := appliedStep
		if cappedStep > len(steps) {
			cappedStep = len(steps)
		}
		s.log.Warn("proxy", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q reason=%q circuit_step=%d cooldown_seconds=%d", requestID, "kiro", "attempt_failed", accountLabel, err.Error(), cappedStep, int(appliedCooldown/time.Second)))
		return
	}
	s.log.Warn("proxy", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q reason=%q circuit_breaker=%t", requestID, "kiro", "attempt_failed", accountLabel, err.Error(), breakerEnabled))
}

func (s *Service) markBanned(requestID string, accountID string, accountLabel string, reason string) {
	_ = s.store.MarkAccountBanned(accountID, reason)
	s.log.Warn("auth", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q reason=%q", requestID, "kiro", "banned", accountLabel, reason))
}

func (s *Service) applyFailureDecision(requestID string, accountID string, accountLabel string, decision provider.FailureDecision) {
	switch decision.Class {
	case provider.FailureRequestShape:
		s.log.Warn("proxy", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q reason=%q", requestID, "kiro", "request_shape", accountLabel, decision.Message))
	case provider.FailureDurableDisabled:
		if decision.BanAccount {
			s.markBanned(requestID, accountID, accountLabel, decision.Message)
			return
		}
		_ = s.store.MarkAccountDurablyDisabled(accountID, decision.Message)
		s.log.Warn("auth", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q reason=%q", requestID, "kiro", "durable_disabled", accountLabel, decision.Message))
	case provider.FailureQuotaCooldown:
		cooldownUntil := time.Now().Add(decision.Cooldown).Unix()
		_ = s.store.UpdateAccount(accountID, func(a *config.Account) {
			a.ErrorCount++
			a.CooldownUntil = cooldownUntil
			a.HealthState = config.AccountHealthCooldownQuota
			a.HealthReason = decision.Message
			a.LastFailureAt = time.Now().Unix()
			a.LastError = decision.Message
			a.Quota = config.QuotaInfo{
				Status:        "exhausted",
				Summary:       decision.Message,
				Source:        "runtime",
				Error:         decision.Message,
				LastCheckedAt: time.Now().Unix(),
				Buckets:       []config.QuotaBucket{{Name: "credits", ResetAt: cooldownUntil, Status: "exhausted"}},
			}
		})
		s.log.Warn("quota", fmt.Sprintf("request_id=%q provider=%q account=%q phase=%q reason=%q cooldown_until=%d", requestID, "kiro", accountLabel, "cooldown", decision.Message, cooldownUntil))
	default:
		s.markTransientFailure(requestID, accountID, accountLabel, fmt.Errorf(decision.Message))
	}
}

func (s *Service) recordRequestFailure() {
	now := time.Now().Unix()
	_ = s.store.UpdateStats(func(stats *config.ProxyStats) {
		stats.TotalRequests++
		stats.FailedRequests++
		stats.LastRequestAt = now
	})
}
