package account

import (
	"strings"

	"cliro/internal/config"
)

func Label(account config.Account) string {
	if email := strings.TrimSpace(account.Email); email != "" {
		return email
	}
	if accountID := strings.TrimSpace(account.AccountID); accountID != "" {
		return accountID
	}
	return strings.TrimSpace(account.ID)
}

func QuotaResetAt(quota config.QuotaInfo) int64 {
	var latest int64
	for _, bucket := range quota.Buckets {
		if bucket.ResetAt > latest {
			latest = bucket.ResetAt
		}
	}
	return latest
}
