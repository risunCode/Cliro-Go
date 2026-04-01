package account

import (
	"fmt"
	"strings"

	"cliro-go/internal/config"
)

type Account = config.Account
type QuotaInfo = config.QuotaInfo
type QuotaBucket = config.QuotaBucket
type ProxyStats = config.ProxyStats

type Store struct {
	manager *config.Manager
}

func NewStore(manager *config.Manager) *Store {
	return &Store{manager: manager}
}

func (s *Store) Manager() *config.Manager {
	return s.manager
}

func (s *Store) Accounts() []Account {
	if s == nil || s.manager == nil {
		return nil
	}
	return s.manager.Accounts()
}

func ValidateProvider(provider string) error {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized == "" {
		return fmt.Errorf("account provider is required")
	}
	if normalized != "codex" && normalized != "kiro" {
		return fmt.Errorf("unsupported account provider: %s", provider)
	}
	return nil
}

func ValidateAccount(account Account) error {
	if strings.TrimSpace(account.ID) == "" {
		return fmt.Errorf("account id is required")
	}
	if err := ValidateProvider(account.Provider); err != nil {
		return err
	}
	return nil
}
