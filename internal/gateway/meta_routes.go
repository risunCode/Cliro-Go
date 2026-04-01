package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"cliro-go/internal/route"
)

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if !isRootPathAlias(r.URL.Path) {
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
			"GET /v1/health",
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

func isRootPathAlias(path string) bool {
	trimmed := strings.TrimSpace(path)
	return trimmed == "/" || trimmed == compatV1Prefix || trimmed == compatV1Path(compatV1Prefix)
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
	models := route.CatalogModels()
	data := make([]map[string]any, 0, len(models))
	for _, model := range models {
		data = append(data, map[string]any{"id": model.ID, "object": "model", "owned_by": model.OwnedBy})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   data,
	})
}
