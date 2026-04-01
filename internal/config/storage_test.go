package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func corruptBackups(t *testing.T, targetPath string) []string {
	t.Helper()
	matches, err := filepath.Glob(targetPath + ".corrupt-*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	return matches
}

func TestLoadSettings_RecoversCorruptedJSON(t *testing.T) {
	dataDir := t.TempDir()
	storage, err := NewStorage(dataDir)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}

	configPath := filepath.Join(dataDir, "config.json")
	writeTestFile(t, configPath, []byte(`{"proxyPort":`))

	settings, warnings, err := storage.LoadSettings()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if settings.ProxyPort != defaultProxyPort {
		t.Fatalf("proxy port = %d, want %d", settings.ProxyPort, defaultProxyPort)
	}
	if len(warnings) != 1 || warnings[0].Code != "settings_corrupted" {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
	if warnings[0].BackupPath == "" {
		t.Fatalf("expected backup path in warning")
	}
	if len(corruptBackups(t, configPath)) != 1 {
		t.Fatalf("expected one corrupt backup file")
	}
	migrated, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read recovered settings: %v", readErr)
	}
	if !bytes.Contains(migrated, []byte(`"proxyPort": 8095`)) {
		t.Fatalf("expected recovered settings file, got %s", string(migrated))
	}
}

func TestLoadSettings_AppliesThinkingDefaultsToLegacyConfig(t *testing.T) {
	dataDir := t.TempDir()
	storage, err := NewStorage(dataDir)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}

	configPath := filepath.Join(dataDir, "config.json")
	writeTestFile(t, configPath, []byte(`{"proxyPort":9001,"allowLan":true}`))

	settings, warnings, err := storage.LoadSettings()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
	if settings.ProxyPort != 9001 || !settings.AllowLAN {
		t.Fatalf("unexpected legacy settings: %+v", settings)
	}
	if settings.Thinking.Suffix != defaultThinkingSuffix || settings.Thinking.Mode != ThinkingModeAuto {
		t.Fatalf("unexpected thinking defaults: %+v", settings.Thinking)
	}
	if !settings.Thinking.RequireAnthropicSignature || settings.Thinking.ForceForAnthropic || settings.Thinking.MaxForcedThinkingTokens != defaultMaxForcedThinkingTokens {
		t.Fatalf("unexpected thinking controls: %+v", settings.Thinking)
	}
	if len(settings.Thinking.FallbackTags) != 2 || settings.Thinking.FallbackTags[0] != "<thinking>" || settings.Thinking.FallbackTags[1] != "<think>" {
		t.Fatalf("unexpected fallback tags: %+v", settings.Thinking.FallbackTags)
	}
}

func TestLoadSettings_PreservesExplicitThinkingSettings(t *testing.T) {
	dataDir := t.TempDir()
	storage, err := NewStorage(dataDir)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}

	want := AppSettings{
		ProxyPort:      9123,
		AllowLAN:       true,
		AutoStartProxy: false,
		SchedulingMode: string(SchedulingModePerformance),
		Cloudflared: CloudflaredSettings{
			Enabled:  true,
			Mode:     CloudflaredModeAuth,
			Token:    " token ",
			UseHTTP2: false,
		},
		Thinking: ThinkingSettings{
			Suffix:                    "  -ponder  ",
			Mode:                      ThinkingModeForce,
			FallbackTags:              []string{"  <ponder>  ", "<thinking>", "<ponder>"},
			RequireAnthropicSignature: false,
			ForceForAnthropic:         true,
			MaxForcedThinkingTokens:   2048,
		},
	}
	if err := storage.SaveSettings(want); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	settings, warnings, err := storage.LoadSettings()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
	if settings.Thinking.Suffix != "-ponder" || settings.Thinking.Mode != ThinkingModeForce {
		t.Fatalf("unexpected thinking settings: %+v", settings.Thinking)
	}
	if settings.Thinking.RequireAnthropicSignature || !settings.Thinking.ForceForAnthropic || settings.Thinking.MaxForcedThinkingTokens != 2048 {
		t.Fatalf("unexpected thinking controls: %+v", settings.Thinking)
	}
	if len(settings.Thinking.FallbackTags) != 2 || settings.Thinking.FallbackTags[0] != "<ponder>" || settings.Thinking.FallbackTags[1] != "<thinking>" {
		t.Fatalf("unexpected normalized fallback tags: %+v", settings.Thinking.FallbackTags)
	}
	if settings.Cloudflared.Token != "token" {
		t.Fatalf("expected cloudflared token trim, got %q", settings.Cloudflared.Token)
	}
}

func TestManager_PersistsThinkingDefaultsWhenSavingLegacyConfig(t *testing.T) {
	dataDir := t.TempDir()
	configPath := filepath.Join(dataDir, "config.json")
	writeTestFile(t, configPath, []byte(`{"proxyPort":9001,"autoStartProxy":false}`))

	manager, err := NewManager(dataDir)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := manager.SetAllowLAN(true); err != nil {
		t.Fatalf("set allow LAN: %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}

	var persisted map[string]any
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("unmarshal persisted config: %v", err)
	}
	thinkingValue, ok := persisted["thinking"]
	if !ok {
		t.Fatalf("expected thinking settings to be persisted, got %s", string(raw))
	}
	thinking, ok := thinkingValue.(map[string]any)
	if !ok {
		t.Fatalf("unexpected thinking payload type: %T", thinkingValue)
	}
	if thinking["suffix"] != defaultThinkingSuffix {
		t.Fatalf("suffix = %v", thinking["suffix"])
	}
	if thinking["mode"] != string(ThinkingModeAuto) {
		t.Fatalf("mode = %v", thinking["mode"])
	}
	if thinking["requireAnthropicSignature"] != true {
		t.Fatalf("requireAnthropicSignature = %v", thinking["requireAnthropicSignature"])
	}
	if thinking["maxForcedThinkingTokens"] != float64(defaultMaxForcedThinkingTokens) {
		t.Fatalf("maxForcedThinkingTokens = %v", thinking["maxForcedThinkingTokens"])
	}
	tags, ok := thinking["fallbackTags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "<thinking>" || tags[1] != "<think>" {
		t.Fatalf("unexpected fallbackTags = %#v", thinking["fallbackTags"])
	}
}

func TestLoadAccounts_RecoversMalformedJSON(t *testing.T) {
	dataDir := t.TempDir()
	storage, err := NewStorage(dataDir)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}

	accountsPath := filepath.Join(dataDir, "accounts.json")
	writeTestFile(t, accountsPath, []byte(`[{"id":`))

	accounts, warnings, err := storage.LoadAccounts()
	if err != nil {
		t.Fatalf("load accounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected empty recovered accounts, got %d", len(accounts))
	}
	if len(warnings) != 1 || warnings[0].Code != "accounts_corrupted" {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
	if len(corruptBackups(t, accountsPath)) != 1 {
		t.Fatalf("expected one corrupt backup file")
	}
	rewritten, readErr := os.ReadFile(accountsPath)
	if readErr != nil {
		t.Fatalf("read recovered accounts: %v", readErr)
	}
	if string(bytes.TrimSpace(rewritten)) != "[]" {
		t.Fatalf("expected recovered accounts array, got %s", string(rewritten))
	}
}

func TestLoadAccounts_RecoversLegacyEnvelopeWithoutBackwardParsing(t *testing.T) {
	dataDir := t.TempDir()
	storage, err := NewStorage(dataDir)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}

	accountsPath := filepath.Join(dataDir, "accounts.json")
	legacyPayload := []byte(`{"accounts":[{"id":"a1","provider":"codex","accessToken":"x","refreshToken":"y","enabled":true,"createdAt":1,"updatedAt":1}]}`)
	writeTestFile(t, accountsPath, legacyPayload)

	accounts, warnings, err := storage.LoadAccounts()
	if err != nil {
		t.Fatalf("load accounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected strict recovery to empty array, got %d accounts", len(accounts))
	}
	if len(warnings) != 1 || warnings[0].Code != "accounts_corrupted" {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
	backupPaths := corruptBackups(t, accountsPath)
	if len(backupPaths) != 1 {
		t.Fatalf("expected one corrupt backup file, got %d", len(backupPaths))
	}
	backupContent, readErr := os.ReadFile(backupPaths[0])
	if readErr != nil {
		t.Fatalf("read backup: %v", readErr)
	}
	if !bytes.Equal(bytes.TrimSpace(backupContent), bytes.TrimSpace(legacyPayload)) {
		t.Fatalf("backup content mismatch: %s", string(backupContent))
	}
}

func TestLoadStats_RecoversCorruptedJSON(t *testing.T) {
	dataDir := t.TempDir()
	storage, err := NewStorage(dataDir)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}

	statsPath := filepath.Join(dataDir, "stats.json")
	writeTestFile(t, statsPath, []byte(`{"totalRequests":`))

	stats, warnings, err := storage.LoadStats()
	if err != nil {
		t.Fatalf("load stats: %v", err)
	}
	if stats.TotalRequests != 0 || stats.TotalTokens != 0 {
		t.Fatalf("expected zero-value recovered stats, got %+v", stats)
	}
	if len(warnings) != 1 || warnings[0].Code != "stats_corrupted" {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
	if len(corruptBackups(t, statsPath)) != 1 {
		t.Fatalf("expected one corrupt backup file")
	}
}

func TestNewManager_CollectsStartupWarningsAndContinues(t *testing.T) {
	dataDir := t.TempDir()
	writeTestFile(t, filepath.Join(dataDir, "config.json"), []byte(`{"proxyPort":`))
	writeTestFile(t, filepath.Join(dataDir, "accounts.json"), []byte(`{"accounts":[{"id":"legacy"}]}`))
	writeTestFile(t, filepath.Join(dataDir, "stats.json"), []byte(`{"totalRequests":`))

	manager, err := NewManager(dataDir)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if manager.ProxyPort() != defaultProxyPort {
		t.Fatalf("proxy port = %d, want %d", manager.ProxyPort(), defaultProxyPort)
	}
	if len(manager.Accounts()) != 0 {
		t.Fatalf("expected recovered empty accounts")
	}
	warnings := manager.StartupWarnings()
	if len(warnings) != 3 {
		t.Fatalf("expected 3 startup warnings, got %d", len(warnings))
	}
	snapshot := manager.Snapshot()
	if len(snapshot.StartupWarnings) != 3 {
		t.Fatalf("snapshot startup warnings = %d, want 3", len(snapshot.StartupWarnings))
	}
}
