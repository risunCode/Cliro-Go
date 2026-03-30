package kiro

import "cliro-go/internal/config"

type TokenRefresher interface {
	EnsureFreshAccount(accountID string) (config.Account, error)
	RefreshAccount(accountID string) (config.Account, error)
}
