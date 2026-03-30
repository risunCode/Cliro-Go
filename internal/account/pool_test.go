package account

import (
	"testing"

	"cliro-go/internal/config"
)

func TestPoolSkipsBannedAccounts(t *testing.T) {
	dataDir := t.TempDir()
	manager, err := config.NewManager(dataDir)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if err := manager.UpsertAccount(config.Account{ID: "banned", Provider: "codex", Email: "banned@example.com", Enabled: true, Banned: true, CreatedAt: 2, UpdatedAt: 2}); err != nil {
		t.Fatalf("upsert banned account: %v", err)
	}
	if err := manager.UpsertAccount(config.Account{ID: "healthy", Provider: "codex", Email: "healthy@example.com", Enabled: true, CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatalf("upsert healthy account: %v", err)
	}

	pool := NewPool(manager)
	available := pool.AvailableAccounts()
	if len(available) != 1 {
		t.Fatalf("available accounts = %d, want 1", len(available))
	}
	if available[0].ID != "healthy" {
		t.Fatalf("available account id = %q, want healthy", available[0].ID)
	}
	if pool.AvailableCount() != 1 {
		t.Fatalf("available count = %d, want 1", pool.AvailableCount())
	}
}

func TestAvailabilitySnapshot_SeparatesCooldownAndDurableStates(t *testing.T) {
	dataDir := t.TempDir()
	manager, err := config.NewManager(dataDir)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	now := int64(1_800_000_000)
	accounts := []config.Account{
		{ID: "ready", Provider: "codex", Email: "ready@example.com", Enabled: true, HealthState: config.AccountHealthReady, CreatedAt: 5, UpdatedAt: 5},
		{ID: "quota", Provider: "codex", Email: "quota@example.com", Enabled: true, HealthState: config.AccountHealthCooldownQuota, CooldownUntil: now + 60, CreatedAt: 4, UpdatedAt: 4},
		{ID: "transient", Provider: "codex", Email: "transient@example.com", Enabled: true, HealthState: config.AccountHealthCooldownTransient, CooldownUntil: now + 30, CreatedAt: 3, UpdatedAt: 3},
		{ID: "durable", Provider: "codex", Email: "durable@example.com", Enabled: false, HealthState: config.AccountHealthDisabledDurable, CreatedAt: 2, UpdatedAt: 2},
		{ID: "banned", Provider: "codex", Email: "banned@example.com", Enabled: true, Banned: true, HealthState: config.AccountHealthBanned, CreatedAt: 1, UpdatedAt: 1},
	}
	for _, account := range accounts {
		if err := manager.UpsertAccount(account); err != nil {
			t.Fatalf("upsert account %s: %v", account.ID, err)
		}
	}

	pool := NewPool(manager)
	snapshot := pool.AvailabilitySnapshot("codex")
	if snapshot.ReadyCount != 1 || snapshot.CooldownQuotaCount != 1 || snapshot.CooldownTransientCount != 1 || snapshot.DisabledDurableCount != 1 || snapshot.BannedCount != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if reason := pool.ProviderUnavailableReason("codex"); reason == "" || reason == "no available codex accounts (ready=0 cooldown_quota=0 cooldown_transient=0 durable_disabled=0 banned=0)" {
		t.Fatalf("unexpected provider unavailable reason: %q", reason)
	}
}

func TestAvailableAccounts_BalancePrefersLowerUsage(t *testing.T) {
	dataDir := t.TempDir()
	manager, err := config.NewManager(dataDir)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := manager.SetSchedulingMode(config.SchedulingModeBalance); err != nil {
		t.Fatalf("set scheduling mode: %v", err)
	}

	accounts := []config.Account{
		{ID: "heavy", Provider: "codex", Email: "heavy@example.com", Enabled: true, RequestCount: 20, ErrorCount: 2, LastUsed: 300, CreatedAt: 3, UpdatedAt: 3},
		{ID: "mid", Provider: "codex", Email: "mid@example.com", Enabled: true, RequestCount: 8, ErrorCount: 1, LastUsed: 200, CreatedAt: 2, UpdatedAt: 2},
		{ID: "light", Provider: "codex", Email: "light@example.com", Enabled: true, RequestCount: 2, ErrorCount: 0, LastUsed: 100, CreatedAt: 1, UpdatedAt: 1},
	}
	for _, account := range accounts {
		if err := manager.UpsertAccount(account); err != nil {
			t.Fatalf("upsert account %s: %v", account.ID, err)
		}
	}

	pool := NewPool(manager)
	available := pool.AvailableAccountsForProvider("codex")
	if len(available) != 3 {
		t.Fatalf("available accounts = %d, want 3", len(available))
	}
	if available[0].ID != "light" || available[1].ID != "mid" || available[2].ID != "heavy" {
		t.Fatalf("unexpected balance order: %q, %q, %q", available[0].ID, available[1].ID, available[2].ID)
	}
}

func TestAvailableAccounts_CacheFirstPrefersMostRecentAccount(t *testing.T) {
	dataDir := t.TempDir()
	manager, err := config.NewManager(dataDir)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := manager.SetSchedulingMode(config.SchedulingModeCacheFirst); err != nil {
		t.Fatalf("set scheduling mode: %v", err)
	}

	accounts := []config.Account{
		{ID: "old", Provider: "codex", Email: "old@example.com", Enabled: true, LastUsed: 100, CreatedAt: 3, UpdatedAt: 3},
		{ID: "warm", Provider: "codex", Email: "warm@example.com", Enabled: true, LastUsed: 300, CreatedAt: 2, UpdatedAt: 2},
		{ID: "mid", Provider: "codex", Email: "mid@example.com", Enabled: true, LastUsed: 200, CreatedAt: 1, UpdatedAt: 1},
	}
	for _, account := range accounts {
		if err := manager.UpsertAccount(account); err != nil {
			t.Fatalf("upsert account %s: %v", account.ID, err)
		}
	}

	pool := NewPool(manager)
	available := pool.AvailableAccountsForProvider("codex")
	if len(available) != 3 {
		t.Fatalf("available accounts = %d, want 3", len(available))
	}
	if available[0].ID != "warm" || available[1].ID != "mid" || available[2].ID != "old" {
		t.Fatalf("unexpected cache-first order: %q, %q, %q", available[0].ID, available[1].ID, available[2].ID)
	}
}
