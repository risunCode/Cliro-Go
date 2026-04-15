package proxyhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"cliro/internal/account"
	"cliro/internal/auth"
	"cliro/internal/config"
	"cliro/internal/logger"
	"cliro/internal/platform"
	codexprovider "cliro/internal/provider/codex"
	kiroprovider "cliro/internal/provider/kiro"
	models "cliro/internal/proxy/models"

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
	compatV1Prefix             = "/v1"
)

func nowUnix() int64 {
	return time.Now().Unix()
}

func newSSEID() string {
	return uuid.NewString()
}

type completionExecutor interface {
	ExecuteFromIR(ctx context.Context, request models.Request) (codexprovider.CompletionOutcome, int, string, error)
}

type Server struct {
	store      *config.Manager
	auth       *auth.Manager
	pool       *account.Pool
	log        *logger.Logger
	httpClient *http.Client
	codex      completionExecutor
	kiro       *kiroprovider.Service

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

func (s *Server) newMux() *http.ServeMux {
	mux := http.NewServeMux()
	s.handleRouteWithCompatV1(mux, "/", s.handleRoot)
	s.handleRouteWithCompatV1(mux, RouteHealth, s.handleHealth)
	s.handleRouteWithCompatV1(mux, RouteStats, s.handleStats)
	s.handleRouteWithCompatV1(mux, RouteModels, s.handleModels)
	s.handleRouteWithCompatV1(mux, RouteOpenAIResponses, s.handleResponses)
	s.handleRouteWithCompatV1(mux, RouteOpenAIChatCompletions, s.handleChatCompletions)
	s.handleRouteWithCompatV1(mux, RouteOpenAICompletions, s.handleCompletions)
	s.handleRouteWithCompatV1(mux, RouteAnthropicMessages, s.handleAnthropicMessages)
	s.handleRouteWithCompatV1(mux, RouteAnthropicCountTokens, s.handleAnthropicCountTokens)
	mux.HandleFunc("/api/event_logging/batch", s.handleEventLogging)
	return mux
}

func (s *Server) handleRouteWithCompatV1(mux *http.ServeMux, path string, handler http.HandlerFunc) {
	for _, alias := range routeAliases(path) {
		mux.HandleFunc(alias, handler)
	}
}

func compatV1Path(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if trimmed == "/" {
		return compatV1Prefix
	}
	return compatV1Prefix + trimmed
}

func routeAliases(path string) []string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil
	}
	aliases := []string{trimmed}
	firstCompat := compatV1Path(trimmed)
	if firstCompat != "" {
		aliases = append(aliases, firstCompat)
	}
	secondCompat := compatV1Path(firstCompat)
	if secondCompat != "" {
		aliases = append(aliases, secondCompat)
	}
	unique := make([]string, 0, len(aliases))
	seen := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		if _, ok := seen[alias]; ok {
			continue
		}
		seen[alias] = struct{}{}
		unique = append(unique, alias)
	}
	return unique
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

	bindAddr := ProxyBindAddress(allowLAN, port)
	mux := s.newMux()

	server := &http.Server{Addr: bindAddr, Handler: mux}
	s.server = server
	s.running = true
	s.started = time.Now()
	s.bindAddr = bindAddr

	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			s.log.Error("gateway", "server.stopped_unexpectedly", logger.F("address", bindAddr), logger.Err(err))
		}
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	s.log.Info("gateway", "server.started", logger.F("address", bindAddr))
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
	s.log.Info("gateway", "server.stopping")
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

func (s *Server) applyCommonHeaders(w http.ResponseWriter) {
	ApplyCommonProxyHeaders(w)
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
	eventFields := []logger.Field{
		logger.F("request_id", strings.TrimSpace(requestID)),
	}
	eventFields = append(eventFields, parseRequestFields(fields...)...)
	eventName := "request_" + strings.TrimSpace(phase)
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "warn":
		s.log.Warn("proxy", eventName, eventFields...)
	case "error":
		s.log.Error("proxy", eventName, eventFields...)
	default:
		s.log.Info("proxy", eventName, eventFields...)
	}
}

func parseRequestFields(fields ...string) []logger.Field {
	parsed := make([]logger.Field, 0, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			continue
		}
		key, rawValue, ok := strings.Cut(trimmed, "=")
		if !ok {
			parsed = append(parsed, logger.F("detail", trimmed))
			continue
		}
		key = strings.TrimSpace(key)
		rawValue = strings.TrimSpace(rawValue)
		if key == "" {
			continue
		}
		if len(rawValue) >= 2 && strings.HasPrefix(rawValue, `"`) && strings.HasSuffix(rawValue, `"`) {
			if unquoted, err := strconv.Unquote(rawValue); err == nil {
				parsed = append(parsed, logger.F(key, unquoted))
				continue
			}
		}
		if parsedBool, err := strconv.ParseBool(rawValue); err == nil {
			parsed = append(parsed, logger.F(key, parsedBool))
			continue
		}
		if parsedInt, err := strconv.ParseInt(rawValue, 10, 64); err == nil {
			parsed = append(parsed, logger.F(key, parsedInt))
			continue
		}
		parsed = append(parsed, logger.F(key, rawValue))
	}
	return parsed
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
