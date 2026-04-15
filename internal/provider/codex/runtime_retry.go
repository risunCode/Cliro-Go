package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"cliro/internal/account"
	"cliro/internal/config"
	"cliro/internal/logger"
	baseprovider "cliro/internal/provider"
)

type FailureClass = baseprovider.FailureClass
type FailureDecision = baseprovider.FailureDecision

const (
	FailureRetryableTransport = baseprovider.FailureRetryableTransport
	FailureAuthRefreshable    = baseprovider.FailureAuthRefreshable
	FailureQuotaCooldown      = baseprovider.FailureQuotaCooldown
	FailureDurableDisabled    = baseprovider.FailureDurableDisabled
	FailureRequestShape       = baseprovider.FailureRequestShape
	FailureEmptyStream        = baseprovider.FailureEmptyStream
	FailureProviderFatal      = baseprovider.FailureProviderFatal
)

type AttemptContext struct {
	RequestID string
	Provider  string
	Model     string
	Stream    bool
}

type AttemptResult struct {
	Attempt             int
	Status              int
	Message             string
	Err                 error
	Failure             FailureDecision
	ClientBytesSent     bool
	UpstreamReadable    bool
	EmptyStream         bool
	UpstreamOpen        time.Duration
	FirstClientChunk    time.Duration
	RecoveredAuth       bool
	RetryCause          string
	Final               bool
	Success             bool
	CompletionHasOutput bool
}

type AttemptDiagnostic struct {
	RequestID          string
	Provider           string
	AccountID          string
	AccountLabel       string
	Model              string
	Attempt            int
	Stream             bool
	UpstreamOpenMillis int64
	FirstClientChunkMs int64
	ClientBytesSent    bool
	UpstreamReadable   bool
	EmptyStream        bool
	RetryCause         string
	FailureClass       string
	Success            bool
	RecoveredAuth      bool
	Final              bool
}

func NewAttemptDiagnostic(ctx AttemptContext, accountID string, accountLabel string, result AttemptResult) AttemptDiagnostic {
	return AttemptDiagnostic{
		RequestID:          strings.TrimSpace(ctx.RequestID),
		Provider:           strings.TrimSpace(ctx.Provider),
		AccountID:          strings.TrimSpace(accountID),
		AccountLabel:       strings.TrimSpace(accountLabel),
		Model:              strings.TrimSpace(ctx.Model),
		Attempt:            result.Attempt,
		Stream:             ctx.Stream,
		UpstreamOpenMillis: durationMillis(result.UpstreamOpen),
		FirstClientChunkMs: durationMillis(result.FirstClientChunk),
		ClientBytesSent:    result.ClientBytesSent,
		UpstreamReadable:   result.UpstreamReadable,
		EmptyStream:        result.EmptyStream,
		RetryCause:         strings.TrimSpace(result.RetryCause),
		FailureClass:       string(result.Failure.Class),
		Success:            result.Success,
		RecoveredAuth:      result.RecoveredAuth,
		Final:              result.Final,
	}
}

func LogAttemptDiagnostic(log *logger.Logger, diag AttemptDiagnostic) {
	if log == nil {
		return
	}
	fields := []logger.Field{
		logger.F("request_id", strings.TrimSpace(diag.RequestID)),
		logger.F("provider", strings.TrimSpace(diag.Provider)),
		logger.F("account_id", strings.TrimSpace(diag.AccountID)),
		logger.F("account", strings.TrimSpace(diag.AccountLabel)),
		logger.F("model", strings.TrimSpace(diag.Model)),
		logger.F("attempt", diag.Attempt),
		logger.F("stream", diag.Stream),
		logger.F("upstream_open_ms", diag.UpstreamOpenMillis),
		logger.F("first_client_chunk_ms", diag.FirstClientChunkMs),
		logger.F("client_bytes_sent", diag.ClientBytesSent),
		logger.F("upstream_readable", diag.UpstreamReadable),
		logger.F("empty_stream", diag.EmptyStream),
		logger.F("retry_cause", strings.TrimSpace(diag.RetryCause)),
		logger.F("failure_class", strings.TrimSpace(diag.FailureClass)),
		logger.F("recovered_auth", diag.RecoveredAuth),
		logger.F("success", diag.Success),
		logger.F("final", diag.Final),
	}
	if diag.Success {
		log.Info("proxy", "request.attempt_result", fields...)
		return
	}
	log.Warn("proxy", "request.attempt_result", fields...)
}

func durationMillis(value time.Duration) int64 {
	if value <= 0 {
		return 0
	}
	return value.Milliseconds()
}

func CompletionHasVisibleOutput(outcome CompletionOutcome) bool {
	if strings.TrimSpace(outcome.Text) != "" {
		return true
	}
	if strings.TrimSpace(outcome.Thinking) != "" {
		return true
	}
	return len(outcome.ToolUses) > 0
}

type StatusPhase string

const (
	StatusPhaseNone              StatusPhase = "none"
	StatusPhaseRefreshOnce       StatusPhase = "refresh_once"
	StatusPhaseCooldownThenRetry StatusPhase = "cooldown_then_retry"
)

type StatusPolicy struct {
	Phase StatusPhase
	Final FailureClass
}

type RetryDecision struct {
	Retry          bool
	RefreshAuth    bool
	Cooldown       time.Duration
	Disable        bool
	Ban            bool
	Cause          string
	ExcludeAccount bool
	FinalStatus    int
	FinalMessage   string
	FailureClass   FailureClass
}

type RetryPlanner struct {
	pool     *account.Pool
	provider string
	policies map[int]StatusPolicy
}

func DefaultStatusPolicies() map[int]StatusPolicy {
	return map[int]StatusPolicy{
		401: {Phase: StatusPhaseRefreshOnce, Final: FailureAuthRefreshable},
		403: {Phase: StatusPhaseRefreshOnce, Final: FailureAuthRefreshable},
		429: {Phase: StatusPhaseCooldownThenRetry, Final: FailureQuotaCooldown},
	}
}

func NewRetryPlanner(pool *account.Pool, provider string, policies map[int]StatusPolicy) RetryPlanner {
	resolved := make(map[int]StatusPolicy)
	for status, policy := range DefaultStatusPolicies() {
		resolved[status] = policy
	}
	for status, policy := range policies {
		resolved[status] = policy
	}
	return RetryPlanner{pool: pool, provider: strings.ToLower(strings.TrimSpace(provider)), policies: resolved}
}

func (p RetryPlanner) NextAccount(excluded map[string]bool) (config.Account, bool) {
	if p.pool == nil {
		return config.Account{}, false
	}
	for _, candidate := range p.pool.AvailableAccountsForProvider(p.provider) {
		if excluded != nil && excluded[candidate.ID] {
			continue
		}
		return candidate, true
	}
	return config.Account{}, false
}

func (p RetryPlanner) Decide(result AttemptResult) RetryDecision {
	decision := result.Failure
	statusPolicy, hasPolicy := p.policies[result.Status]
	if hasPolicy && statusPolicy.Final != "" && decision.Class != FailureEmptyStream {
		decision.Class = statusPolicy.Final
	}
	if decision.Class == FailureEmptyStream {
		if !result.ClientBytesSent {
			return RetryDecision{
				Retry:          true,
				Cause:          firstNonEmpty(result.RetryCause, "empty_stream"),
				ExcludeAccount: true,
				FinalStatus:    decision.Status,
				FinalMessage:   firstNonEmpty(result.Message, decision.Message),
				FailureClass:   decision.Class,
			}
		}
		return RetryDecision{FinalStatus: decision.Status, FinalMessage: firstNonEmpty(result.Message, decision.Message), FailureClass: decision.Class}
	}

	if hasPolicy {
		switch statusPolicy.Phase {
		case StatusPhaseRefreshOnce:
			if !result.RecoveredAuth && decision.Class == FailureAuthRefreshable {
				return RetryDecision{
					Retry:        true,
					RefreshAuth:  true,
					Cause:        "status_phase_refresh_once",
					FinalStatus:  decision.Status,
					FinalMessage: firstNonEmpty(result.Message, decision.Message),
					FailureClass: decision.Class,
				}
			}
		case StatusPhaseCooldownThenRetry:
			if !result.ClientBytesSent {
				return RetryDecision{
					Retry:          true,
					Cooldown:       decision.Cooldown,
					Cause:          "status_phase_cooldown_then_retry",
					ExcludeAccount: true,
					FinalStatus:    decision.Status,
					FinalMessage:   firstNonEmpty(result.Message, decision.Message),
					FailureClass:   decision.Class,
				}
			}
		}
	}

	if decision.Class == FailureAuthRefreshable && !result.RecoveredAuth {
		return RetryDecision{
			Retry:        true,
			RefreshAuth:  true,
			Cause:        firstNonEmpty(result.RetryCause, "auth_refresh"),
			FinalStatus:  decision.Status,
			FinalMessage: firstNonEmpty(result.Message, decision.Message),
			FailureClass: decision.Class,
		}
	}

	if decision.RetryAllowed && !result.ClientBytesSent {
		return RetryDecision{
			Retry:          true,
			Cooldown:       decision.Cooldown,
			Cause:          firstNonEmpty(result.RetryCause, string(decision.Class)),
			ExcludeAccount: true,
			FinalStatus:    decision.Status,
			FinalMessage:   firstNonEmpty(result.Message, decision.Message),
			FailureClass:   decision.Class,
		}
	}

	return RetryDecision{
		Cooldown:     decision.Cooldown,
		Disable:      decision.Disable,
		Ban:          decision.BanAccount,
		FinalStatus:  decision.Status,
		FinalMessage: firstNonEmpty(result.Message, decision.Message),
		FailureClass: decision.Class,
	}
}

type RecoveryStatus string

const (
	RecoveryStatusRefreshed RecoveryStatus = "refreshed"
	RecoveryStatusWaiting   RecoveryStatus = "waiting_for_peer"
	RecoveryStatusRejected  RecoveryStatus = "rejected"
)

type recoveryCall struct {
	done   chan struct{}
	value  config.Account
	err    error
	status RecoveryStatus
	ready  bool
}

type AuthRecoveryCoordinator struct {
	refresh func(accountID string) (config.Account, error)
	mu      sync.Mutex
	active  map[string]*recoveryCall
	sem     chan struct{}
}

func NewAuthRecoveryCoordinator(refresh func(accountID string) (config.Account, error), maxConcurrent int) *AuthRecoveryCoordinator {
	if maxConcurrent <= 0 {
		maxConcurrent = 2
	}
	return &AuthRecoveryCoordinator{
		refresh: refresh,
		active:  make(map[string]*recoveryCall),
		sem:     make(chan struct{}, maxConcurrent),
	}
}

func (c *AuthRecoveryCoordinator) Recover(ctx context.Context, provider string, accountID string) (config.Account, RecoveryStatus, error) {
	if c == nil || c.refresh == nil {
		return config.Account{}, RecoveryStatusRejected, fmt.Errorf("auth recovery is unavailable")
	}
	trimmedID := strings.TrimSpace(accountID)
	if trimmedID == "" {
		return config.Account{}, RecoveryStatusRejected, fmt.Errorf("account id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	c.mu.Lock()
	if existing, ok := c.active[trimmedID]; ok {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return config.Account{}, RecoveryStatusRejected, ctx.Err()
		case <-existing.done:
			return existing.value, RecoveryStatusWaiting, existing.err
		}
	}

	call := &recoveryCall{done: make(chan struct{})}
	c.active[trimmedID] = call
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.active, trimmedID)
		c.mu.Unlock()
		close(call.done)
	}()

	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		call.err = ctx.Err()
		call.status = RecoveryStatusRejected
		call.ready = true
		return config.Account{}, call.status, call.err
	}

	account, err := c.refresh(trimmedID)
	call.value = account
	call.err = err
	call.ready = true
	if err != nil {
		call.status = RecoveryStatusRejected
		return account, call.status, err
	}
	if !account.Enabled || account.Banned {
		call.status = RecoveryStatusRejected
		return account, call.status, fmt.Errorf("%s account is not ready after refresh", firstNonEmpty(provider, "provider"))
	}
	call.status = RecoveryStatusRefreshed
	return account, call.status, nil
}

var ErrEmptyStream = errors.New("empty upstream stream")
var ErrStreamProbeTimeout = errors.New("stream probe timeout")

type StreamProbeResult struct {
	Reader           io.ReadCloser
	UpstreamReadable bool
	EmptyStream      bool
	OpenDuration     time.Duration
}

type StreamBridge struct {
	ProbeSize int
}

func (b StreamBridge) OpenVerified(body io.ReadCloser, timeout time.Duration) (StreamProbeResult, error) {
	if body == nil {
		return StreamProbeResult{EmptyStream: true}, ErrEmptyStream
	}
	probeSize := b.ProbeSize
	if probeSize <= 0 {
		probeSize = 4096
	}

	started := time.Now()
	type readResult struct {
		data []byte
		err  error
	}
	resultCh := make(chan readResult, 1)
	go func() {
		buf := make([]byte, probeSize)
		n, err := body.Read(buf)
		resultCh <- readResult{data: append([]byte(nil), buf[:n]...), err: err}
	}()

	var result readResult
	if timeout > 0 {
		select {
		case result = <-resultCh:
		case <-time.After(timeout):
			_ = body.Close()
			return StreamProbeResult{OpenDuration: time.Since(started)}, ErrStreamProbeTimeout
		}
	} else {
		result = <-resultCh
	}

	probe := StreamProbeResult{OpenDuration: time.Since(started)}
	if len(result.data) > 0 {
		probe.UpstreamReadable = true
		probe.Reader = readCloser{Reader: io.MultiReader(bytes.NewReader(result.data), body), Closer: body}
		return probe, nil
	}
	if errors.Is(result.err, io.EOF) || result.err == nil {
		_ = body.Close()
		probe.EmptyStream = true
		return probe, ErrEmptyStream
	}
	_ = body.Close()
	return probe, result.err
}

func CanRetryStreamFailure(clientBytesSent bool) bool {
	return !clientBytesSent
}

type readCloser struct {
	io.Reader
	io.Closer
}
