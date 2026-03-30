package gateway

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"cliro-go/internal/account"
	"cliro-go/internal/adapter/ir"
	"cliro-go/internal/adapter/rules"
	"cliro-go/internal/auth"
	"cliro-go/internal/config"
	"cliro-go/internal/logger"
	"cliro-go/internal/platform"
	"cliro-go/internal/provider"
	codexprovider "cliro-go/internal/provider/codex"
	kiroprovider "cliro-go/internal/provider/kiro"
	"cliro-go/internal/route"

	"github.com/google/uuid"
)

const (
	requestTimeout = 5 * time.Minute

	RouteOpenAIResponses       = "/v1/responses"
	RouteOpenAIChatCompletions = "/v1/chat/completions"
	RouteOpenAICompletions     = "/v1/completions"
	RouteAnthropicMessages     = "/v1/messages"
	RouteAnthropicCountTokens  = "/v1/messages/count_tokens"
	RouteHealth                = "/health"
	RouteStats                 = "/v1/stats"
	RouteModels                = "/v1/models"
)

func nowUnix() int64 {
	return time.Now().Unix()
}

func newSSEID() string {
	return uuid.NewString()
}

type Error struct {
	Status  int
	Type    string
	Message string
}

func (e Error) Error() string {
	return e.Message
}

func InvalidRequest(message string) Error {
	return Error{Status: http.StatusBadRequest, Type: "invalid_request_error", Message: message}
}

func ServerError(message string) Error {
	return Error{Status: http.StatusInternalServerError, Type: "server_error", Message: message}
}

func Unauthorized(message string) Error {
	return Error{Status: http.StatusUnauthorized, Type: "authentication_error", Message: message}
}

func Forbidden(message string) Error {
	return Error{Status: http.StatusForbidden, Type: "permission_error", Message: message}
}

type completionExecutor interface {
	ExecuteFromIR(ctx context.Context, request ir.Request) (provider.CompletionOutcome, int, string, error)
}

type Server struct {
	store      *config.Manager
	auth       *auth.Manager
	pool       *account.Pool
	log        *logger.Logger
	httpClient *http.Client
	codex      completionExecutor
	kiro       completionExecutor

	mu       sync.Mutex
	server   *http.Server
	running  bool
	started  time.Time
	bindAddr string
}

func NewServer(store *config.Manager, authManager *auth.Manager, accountPool *account.Pool, log *logger.Logger) *Server {
	s := &Server{
		store: store,
		auth:  authManager,
		pool:  accountPool,
		log:   log,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
	}
	s.codex = codexprovider.NewService(store, authManager, accountPool, log, s.httpClient)
	s.kiro = kiroprovider.NewService(store, authManager, accountPool, log, s.httpClient)
	return s
}

func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Server) BindAddress() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bindAddr
}

func (s *Server) Start(port int, allowLAN bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}

	bindAddr := platform.ProxyBindAddress(allowLAN, port)
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc(RouteHealth, s.handleHealth)
	mux.HandleFunc(RouteStats, s.handleStats)
	mux.HandleFunc(RouteModels, s.handleModels)
	mux.HandleFunc(RouteOpenAIResponses, s.handleResponses)
	mux.HandleFunc(RouteOpenAIChatCompletions, s.handleChatCompletions)
	mux.HandleFunc(RouteOpenAICompletions, s.handleCompletions)
	mux.HandleFunc(RouteAnthropicMessages, s.handleAnthropicMessages)
	mux.HandleFunc(RouteAnthropicCountTokens, s.handleAnthropicCountTokens)
	mux.HandleFunc("/api/event_logging/batch", s.handleEventLogging)

	server := &http.Server{Addr: bindAddr, Handler: mux}
	s.server = server
	s.running = true
	s.started = time.Now()
	s.bindAddr = bindAddr

	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			s.log.Error("gateway", "gateway server stopped unexpectedly: "+err.Error())
		}
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	s.log.Info("gateway", fmt.Sprintf("gateway listening on http://%s", bindAddr))
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.running = false
	s.mu.Unlock()
	if server == nil {
		return nil
	}
	s.log.Info("gateway", "stopping gateway server")
	return server.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "ok",
		"running":    s.Running(),
		"started_at": s.started.Unix(),
	})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if secErr := s.validateSecurityHeaders(r); secErr.Message != "" {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "root"), fmt.Sprintf("reason=%q", secErr.Message))
		s.writeGenericError(w, secErr.Status, secErr.Type, secErr.Message)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":    "CLIro-Go Gateway",
		"status":  "ok",
		"running": s.Running(),
		"routes": []string{
			"GET /health",
			"GET /v1/models",
			"GET /v1/stats",
			"POST /v1/responses",
			"POST /v1/chat/completions",
			"POST /v1/completions",
			"POST /v1/messages",
			"POST /v1/messages/count_tokens",
		},
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if secErr := s.validateSecurityHeaders(r); secErr.Message != "" {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "stats"), fmt.Sprintf("reason=%q", secErr.Message))
		s.writeGenericError(w, secErr.Status, secErr.Type, secErr.Message)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	snapshot := s.store.Snapshot()
	accounts := s.store.Accounts()
	enabled := 0
	availableSnapshot := s.pool.AvailabilitySnapshot("")
	for _, account := range accounts {
		if account.Enabled {
			enabled++
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":          "ok",
		"accounts":        len(accounts),
		"enabledAccounts": enabled,
		"available":       availableSnapshot.ReadyCount,
		"availability":    availableSnapshot,
		"stats":           snapshot.Stats,
	})
}

func (s *Server) handleEventLogging(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if secErr := s.validateSecurityHeaders(r); secErr.Message != "" {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "event_logging"), fmt.Sprintf("reason=%q", secErr.Message))
		s.writeGenericError(w, secErr.Status, secErr.Type, secErr.Message)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if secErr := s.validateSecurityHeaders(r); secErr.Message != "" {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "models"), fmt.Sprintf("reason=%q", secErr.Message))
		s.writeGenericError(w, secErr.Status, secErr.Type, secErr.Message)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	models := route.CatalogModels(route.DefaultThinkingSuffix)
	data := make([]map[string]any, 0, len(models))
	for _, model := range models {
		data = append(data, map[string]any{"id": model.ID, "object": "model", "owned_by": model.OwnedBy})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   data,
	})
}

func (s *Server) executeRequest(ctx context.Context, request ir.Request) (ir.Response, int, string, error) {
	requestID := platform.RequestIDFromContext(ctx)
	resolution, err := route.ResolveModel(request.Model, route.DefaultThinkingSuffix)
	if err != nil {
		s.recordRequestFailure()
		s.logRequestEvent("warn", requestID, "failed", fmt.Sprintf("route=%q", string(request.Endpoint)), fmt.Sprintf("reason=%q", err.Error()))
		return ir.Response{}, http.StatusBadRequest, err.Error(), err
	}

	if resolution.Provider == route.ProviderCodex {
		request.Model = resolution.ResolvedModel
	} else {
		request.Model = resolution.RequestedModel
	}

	if err := route.ValidateEndpointProvider(string(request.Endpoint), resolution.Provider); err != nil {
		s.recordRequestFailure()
		s.logRequestEvent("warn", requestID, "failed", fmt.Sprintf("route=%q", string(request.Endpoint)), fmt.Sprintf("provider=%q", string(resolution.Provider)), fmt.Sprintf("reason=%q", err.Error()))
		return ir.Response{}, http.StatusBadRequest, err.Error(), err
	}
	if err := rules.ValidateRequest(request, string(resolution.Provider)); err != nil {
		s.recordRequestFailure()
		s.logRequestEvent("warn", requestID, "failed", fmt.Sprintf("route=%q", string(request.Endpoint)), fmt.Sprintf("provider=%q", string(resolution.Provider)), fmt.Sprintf("reason=%q", err.Error()))
		return ir.Response{}, http.StatusBadRequest, err.Error(), err
	}
	s.logRequestEvent("info", requestID, "routed", fmt.Sprintf("route=%q", string(request.Endpoint)), fmt.Sprintf("provider=%q", string(resolution.Provider)), fmt.Sprintf("model=%q", strings.TrimSpace(request.Model)))

	switch resolution.Provider {
	case route.ProviderCodex:
		outcome, status, message, execErr := s.codex.ExecuteFromIR(ctx, request)
		if execErr != nil {
			return ir.Response{}, status, message, execErr
		}
		s.logRequestEvent("info", requestID, "provider_completed", fmt.Sprintf("route=%q", string(request.Endpoint)), fmt.Sprintf("provider=%q", firstNonEmpty(strings.TrimSpace(outcome.Provider), string(resolution.Provider))), fmt.Sprintf("account=%q", strings.TrimSpace(outcome.AccountLabel)), fmt.Sprintf("model=%q", strings.TrimSpace(outcome.Model)), fmt.Sprintf("prompt_tokens=%d", outcome.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", outcome.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", outcome.Usage.TotalTokens))
		return outcomeToIRResponse(outcome, request.Model), 0, "", nil
	case route.ProviderKiro:
		outcome, status, message, execErr := s.kiro.ExecuteFromIR(ctx, request)
		if execErr != nil {
			return ir.Response{}, status, message, execErr
		}
		s.logRequestEvent("info", requestID, "provider_completed", fmt.Sprintf("route=%q", string(request.Endpoint)), fmt.Sprintf("provider=%q", firstNonEmpty(strings.TrimSpace(outcome.Provider), string(resolution.Provider))), fmt.Sprintf("account=%q", strings.TrimSpace(outcome.AccountLabel)), fmt.Sprintf("model=%q", strings.TrimSpace(outcome.Model)), fmt.Sprintf("prompt_tokens=%d", outcome.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", outcome.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", outcome.Usage.TotalTokens))
		return outcomeToIRResponse(outcome, request.Model), 0, "", nil
	default:
		s.recordRequestFailure()
		s.logRequestEvent("warn", requestID, "failed", fmt.Sprintf("route=%q", string(request.Endpoint)), fmt.Sprintf("reason=%q", "unsupported provider"))
		return ir.Response{}, http.StatusBadRequest, "unsupported provider", fmt.Errorf("unsupported provider")
	}
}

func outcomeToIRResponse(outcome provider.CompletionOutcome, model string) ir.Response {
	toolCalls := make([]ir.ToolCall, 0, len(outcome.ToolUses))
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
		toolCalls = append(toolCalls, ir.ToolCall{
			ID:        toolUse.ID,
			Name:      toolUse.Name,
			Arguments: arguments,
		})
	}

	stopReason := "stop"
	if len(toolCalls) > 0 {
		stopReason = "tool_calls"
	}

	resolvedModel := firstNonEmpty(strings.TrimSpace(outcome.Model), strings.TrimSpace(model))

	return ir.Response{
		ID:         outcome.ID,
		Model:      resolvedModel,
		Text:       outcome.Text,
		Thinking:   outcome.Thinking,
		ToolCalls:  toolCalls,
		StopReason: stopReason,
		Usage: ir.Usage{
			PromptTokens:     outcome.Usage.PromptTokens,
			CompletionTokens: outcome.Usage.CompletionTokens,
			TotalTokens:      outcome.Usage.TotalTokens,
			InputTokens:      outcome.Usage.PromptTokens,
			OutputTokens:     outcome.Usage.CompletionTokens,
		},
	}
}

func (s *Server) applyCommonHeaders(w http.ResponseWriter) {
	platform.ApplyCommonProxyHeaders(w)
}

func (s *Server) prepareRequestContext(r *http.Request) (*http.Request, string) {
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" {
		requestID = uuid.NewString()
	}
	ctx := platform.WithRequestID(r.Context(), requestID)
	return r.WithContext(ctx), requestID
}

func (s *Server) logRequestEvent(level string, requestID string, phase string, fields ...string) {
	parts := []string{
		fmt.Sprintf("request_id=%q", strings.TrimSpace(requestID)),
		fmt.Sprintf("phase=%q", strings.TrimSpace(phase)),
	}
	parts = append(parts, fields...)
	message := strings.Join(parts, " ")
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "warn":
		s.log.Warn("proxy", message)
	case "error":
		s.log.Error("proxy", message)
	default:
		s.log.Info("proxy", message)
	}
}

func (s *Server) validateSecurityHeaders(r *http.Request) Error {
	if r == nil {
		return InvalidRequest("request is required")
	}
	if s.store == nil {
		return ServerError("store unavailable")
	}

	configuredKey := strings.TrimSpace(s.store.ProxyAPIKey())
	providedKey, err := resolveProxyCredential(r)
	if err != nil {
		return InvalidRequest(err.Error())
	}

	if !s.store.AuthorizationMode() {
		return Error{}
	}
	if configuredKey == "" {
		return Forbidden("authorization mode enabled but proxy API key is not configured")
	}
	if providedKey == "" {
		return Unauthorized("missing proxy API key")
	}
	if subtle.ConstantTimeCompare([]byte(providedKey), []byte(configuredKey)) != 1 {
		return Unauthorized("invalid proxy API key")
	}
	return Error{}
}

func resolveProxyCredential(r *http.Request) (string, error) {
	if r == nil {
		return "", nil
	}
	authorizationHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	xAPIKey := strings.TrimSpace(r.Header.Get("X-API-Key"))

	resolvedBearer := ""
	if authorizationHeader != "" {
		parts := strings.Fields(authorizationHeader)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			return "", fmt.Errorf("malformed Authorization header")
		}
		resolvedBearer = strings.TrimSpace(parts[1])
	}

	if resolvedBearer != "" && xAPIKey != "" && subtle.ConstantTimeCompare([]byte(resolvedBearer), []byte(xAPIKey)) != 1 {
		return "", fmt.Errorf("conflicting Authorization and X-API-Key headers")
	}
	if resolvedBearer != "" {
		return resolvedBearer, nil
	}
	return xAPIKey, nil
}

func (s *Server) writeGenericError(w http.ResponseWriter, status int, errType string, message string) {
	s.applyCommonHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	})
}

func (s *Server) resolveStreamFlag(model string, endpoint string, requested bool) bool {
	_ = s
	_ = model
	_ = endpoint
	_ = requested
	return true
}

func (s *Server) writeOpenAIError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	})
}

func (s *Server) writeAnthropicError(w http.ResponseWriter, status int, errType, message string) {
	s.applyCommonHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	})
}

func (s *Server) recordRequestFailure() {
	now := time.Now().Unix()
	_ = s.store.UpdateStats(func(stats *config.ProxyStats) {
		stats.TotalRequests++
		stats.FailedRequests++
		stats.LastRequestAt = now
	})
}

func chunkText(text string, chunkSize int) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = 128
	}

	runes := []rune(trimmed)
	chunks := make([]string, 0, (len(runes)/chunkSize)+1)
	for start := 0; start < len(runes); start += chunkSize {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
