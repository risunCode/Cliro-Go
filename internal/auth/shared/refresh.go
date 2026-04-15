package shared

import (
	"net/http"
	"strings"
	"time"

	"cliro/internal/config"
	"cliro/internal/logger"
)

func HandleRefreshFailure(store *config.Manager, log *logger.Logger, account *config.Account, err error) {
	if store == nil || log == nil || account == nil || err == nil {
		return
	}
	if blockedMsg, blocked := BlockedAccountReason(err.Error()); blocked {
		if markErr := store.MarkAccountBanned(account.ID, blockedMsg); markErr != nil {
			log.Warn("auth", "failed to mark account banned: "+markErr.Error())
		}
		return
	}
	if reloginMessage, refreshable := RefreshableAuthReason(err.Error()); refreshable {
		if markErr := store.MarkAccountReloginRequired(account.ID, reloginMessage); markErr != nil {
			log.Warn("auth", "failed to mark account relogin required: "+markErr.Error())
		}
		if updated, ok := store.GetAccount(account.ID); ok {
			*account = updated
		}
		return
	}
}

func FetchUpdatedAccount(store *config.Manager, log *logger.Logger, fallback config.Account) (config.Account, bool) {
	if store == nil {
		return fallback, false
	}
	refreshed, ok := store.GetAccount(fallback.ID)
	if !ok {
		if log != nil {
			log.Warn("auth", "account not found after refresh update: "+strings.TrimSpace(fallback.ID))
		}
		return fallback, false
	}
	return refreshed, true
}

// TokenExpired reports whether the account's access token has expired relative to now.
func TokenExpired(account config.Account, now time.Time) bool {
	if account.ExpiresAt <= 0 {
		return false
	}
	return now.Unix() >= account.ExpiresAt
}

// DefaultHTTPClient returns a new fallback HTTP client with a 60-second timeout.
// Auth services call this when their httpClient factory returns nil.
func DefaultHTTPClient(factory func() *http.Client) *http.Client {
	if factory != nil {
		if client := factory(); client != nil {
			return client
		}
	}
	return &http.Client{Timeout: 60 * time.Second}
}
