package codex

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	accountstate "cliro/internal/account"
	"cliro/internal/config"
	"cliro/internal/logger"
)

const (
	// defaultRefreshInterval is how often the loop wakes up to scan all accounts.
	defaultRefreshInterval = 45 * time.Minute

	// refreshDebounce prevents re-refreshing an account that was just refreshed.
	refreshDebounce = 60 * time.Second

	// tokenExpiryMargin triggers a proactive refresh when the token
	// expires within this window, even if the interval hasn't elapsed.
	tokenExpiryMargin = 5 * time.Minute

	// perRefreshTimeout caps each individual OAuth token refresh call.
	// Deliberately uses context.Background() so a long SSE conversation
	// doesn't cancel background refreshes.
	perRefreshTimeout = 30 * time.Second

	// refreshConcurrency is the max number of concurrent refresh goroutines.
	refreshConcurrency = 8
)

// refreshLoop holds the state for the Codex background token refresh goroutine.
type refreshLoop struct {
	store        *config.Manager
	log          *logger.Logger
	refreshFn    func(config.Account) error
	interval     time.Duration
	stopCh       chan struct{}
	stopped      atomic.Bool
	refreshingMu sync.Mutex
	refreshing   map[string]bool // accountID → currently refreshing
}

func newRefreshLoop(
	store *config.Manager,
	log *logger.Logger,
	interval time.Duration,
	refreshFn func(config.Account) error,
) *refreshLoop {
	if interval <= 0 {
		interval = defaultRefreshInterval
	}
	return &refreshLoop{
		store:      store,
		log:        log,
		refreshFn:  refreshFn,
		interval:   interval,
		stopCh:     make(chan struct{}),
		refreshing: make(map[string]bool),
	}
}

// Start launches the background loop. It runs once immediately on startup,
// then on every interval tick. Call Stop to shut it down.
func (r *refreshLoop) Start(ctx context.Context) {
	r.runOnce(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.runOnce(ctx)
		}
	}
}

// Stop signals the loop goroutine to exit.
func (r *refreshLoop) Stop() {
	if r.stopped.CompareAndSwap(false, true) {
		close(r.stopCh)
	}
}

// runOnce iterates all codex accounts and refreshes those that need it,
// bounded by the refreshConcurrency semaphore.
func (r *refreshLoop) runOnce(ctx context.Context) {
	accounts := r.store.Accounts()
	sem := make(chan struct{}, refreshConcurrency)
	var wg sync.WaitGroup

	for _, acc := range accounts {
		if !r.needsRefresh(acc) {
			continue
		}
		acc := acc // capture
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r.refreshAccount(ctx, acc)
		}()
	}

	wg.Wait()
}

// needsRefresh returns true if the account should be refreshed now.
func (r *refreshLoop) needsRefresh(acc config.Account) bool {
	// Only codex accounts
	if !strings.EqualFold(strings.TrimSpace(acc.Provider), "codex") {
		return false
	}
	// Must have a refresh token
	if strings.TrimSpace(acc.RefreshToken) == "" {
		return false
	}
	// Skip disabled/banned accounts
	if !acc.Enabled || acc.Banned {
		return false
	}
	// Skip permanently disabled accounts
	if acc.HealthState == config.AccountHealthDisabledDurable {
		return false
	}

	now := time.Now()

	// Debounce: skip if refreshed within the last 60 seconds
	if acc.LastRefresh > 0 {
		lastRefresh := time.Unix(acc.LastRefresh, 0)
		if now.Sub(lastRefresh) < refreshDebounce {
			return false
		}
	}

	// Proactive refresh: token expiring within margin
	if acc.ExpiresAt > 0 {
		expiresAt := time.Unix(acc.ExpiresAt, 0)
		if time.Until(expiresAt) < tokenExpiryMargin {
			return true
		}
	}

	// Interval-based refresh: hasn't been refreshed since last interval
	if acc.LastRefresh > 0 {
		lastRefresh := time.Unix(acc.LastRefresh, 0)
		if now.Sub(lastRefresh) < r.interval {
			return false
		}
	}

	return true
}

// refreshAccount claims a per-account lock (CAS via map) to prevent
// double-refresh, then calls the actual OAuth token refresh.
func (r *refreshLoop) refreshAccount(ctx context.Context, acc config.Account) {
	// CAS-style guard using a mutex-protected map
	r.refreshingMu.Lock()
	if r.refreshing[acc.ID] {
		r.refreshingMu.Unlock()
		return
	}
	r.refreshing[acc.ID] = true
	r.refreshingMu.Unlock()

	defer func() {
		r.refreshingMu.Lock()
		delete(r.refreshing, acc.ID)
		r.refreshingMu.Unlock()
	}()

	// Independent timeout — does not inherit caller/SSE context
	rctx, cancel := context.WithTimeout(context.Background(), perRefreshTimeout)
	defer cancel()

	// Respect outer stop signal without blocking the independent timeout
	done := make(chan struct{})
	var refreshErr error
	go func() {
		defer close(done)
		refreshErr = r.refreshFn(acc)
	}()

	select {
	case <-rctx.Done():
		r.log.Warn("auth", "codex.autorefresh.timeout",
			logger.F("account", accountstate.Label(acc)))
		return
	case <-ctx.Done():
		return
	case <-done:
	}

	if refreshErr != nil {
		r.log.Warn("auth", "codex.autorefresh.failed",
			logger.F("account", accountstate.Label(acc)),
			logger.F("error", refreshErr.Error()))
		return
	}

	r.log.Info("auth", "codex.autorefresh.ok",
		logger.F("account", accountstate.Label(acc)))
}
