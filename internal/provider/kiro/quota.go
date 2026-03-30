package kiro

type QuotaRefresher interface {
	RefreshQuota(accountID string) error
}
