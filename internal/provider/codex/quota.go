package codex

type QuotaRefresher interface {
	RefreshQuota(accountID string) error
}
