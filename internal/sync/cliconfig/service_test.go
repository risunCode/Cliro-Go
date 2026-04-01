package cliconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestService(home string) *Service {
	service := NewService(nil)
	service.homeDirFn = func() (string, error) { return home, nil }
	service.lookPathFn = func(string) (string, error) { return "", os.ErrNotExist }
	service.nowFn = time.Now
	return service
}

func TestSyncClaudeCodeWritesSettingsAndOnboarding(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home)

	result, err := service.Sync(AppClaudeCode, "http://127.0.0.1:8095/v1", "sk-cliro_test", "gpt-5.3-codex")
	if err != nil {
		t.Fatalf("sync claude code: %v", err)
	}
	if result.CurrentBaseURL != "http://127.0.0.1:8095/v1" {
		t.Fatalf("base url = %q", result.CurrentBaseURL)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	text := string(settingsData)
	if !strings.Contains(text, `"ANTHROPIC_BASE_URL": "http://127.0.0.1:8095/v1"`) {
		t.Fatalf("expected anthropic base url in settings: %s", text)
	}
	if !strings.Contains(text, `"ANTHROPIC_API_KEY": "sk-cliro_test"`) {
		t.Fatalf("expected api key in settings: %s", text)
	}
	if !strings.Contains(text, `"model": "gpt-5.3-codex"`) {
		t.Fatalf("expected model in settings: %s", text)
	}

	onboardingPath := filepath.Join(home, ".claude.json")
	onboardingData, err := os.ReadFile(onboardingPath)
	if err != nil {
		t.Fatalf("read onboarding file: %v", err)
	}
	if !strings.Contains(string(onboardingData), `"hasCompletedOnboarding": true`) {
		t.Fatalf("expected onboarding flag in file: %s", string(onboardingData))
	}
}

func TestSyncCodexAIWritesAuthAndConfig(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home)

	result, err := service.Sync(AppCodexAI, "http://127.0.0.1:8095", "sk-cliro_test", "gpt-5.3-codex")
	if err != nil {
		t.Fatalf("sync codex ai: %v", err)
	}
	if result.CurrentBaseURL != "http://127.0.0.1:8095/v1" {
		t.Fatalf("base url = %q", result.CurrentBaseURL)
	}

	authPath := filepath.Join(home, ".codex", "auth.json")
	authData, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth file: %v", err)
	}
	authText := string(authData)
	if !strings.Contains(authText, `"OPENAI_API_KEY": "sk-cliro_test"`) {
		t.Fatalf("expected api key in auth file: %s", authText)
	}
	if !strings.Contains(authText, `"OPENAI_BASE_URL": "http://127.0.0.1:8095/v1"`) {
		t.Fatalf("expected base url in auth file: %s", authText)
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	configText := string(configData)
	if !strings.Contains(configText, `model_provider = "custom"`) {
		t.Fatalf("expected custom model provider in config: %s", configText)
	}
	if !strings.Contains(configText, `base_url = "http://127.0.0.1:8095/v1"`) {
		t.Fatalf("expected base url in config: %s", configText)
	}
	if !strings.Contains(configText, `model = "gpt-5.3-codex"`) {
		t.Fatalf("expected model in config: %s", configText)
	}
}

func TestSyncOpenCodeWritesConfigFile(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home)

	result, err := service.Sync(AppOpenCode, "http://127.0.0.1:8095", "sk-cliro_test", "gpt-5.3-codex")
	if err != nil {
		t.Fatalf("sync opencode: %v", err)
	}
	if result.CurrentBaseURL != "http://127.0.0.1:8095/v1" {
		t.Fatalf("base url = %q", result.CurrentBaseURL)
	}

	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read opencode config: %v", err)
	}
	configText := string(configData)
	if !strings.Contains(configText, `"CLIRO": {`) {
		t.Fatalf("expected CLIRO provider in opencode config: %s", configText)
	}
	if !strings.Contains(configText, `"baseURL": "http://127.0.0.1:8095/v1"`) {
		t.Fatalf("expected base url in opencode config: %s", configText)
	}
	if !strings.Contains(configText, `"apiKey": "sk-cliro_test"`) {
		t.Fatalf("expected api key in opencode config: %s", configText)
	}
	if !strings.Contains(configText, `"model": "gpt-5.3-codex"`) {
		t.Fatalf("expected selected model in opencode config: %s", configText)
	}
	if !strings.Contains(configText, `"bash": "allow"`) {
		t.Fatalf("expected bash permission in opencode config: %s", configText)
	}
	if !strings.Contains(configText, `"gpt-5.3-codex": {`) {
		t.Fatalf("expected model entry in opencode config: %s", configText)
	}
	if !strings.Contains(configText, `"reasoning": true`) {
		t.Fatalf("expected reasoning flag in opencode model config: %s", configText)
	}
}

func TestSyncKiloCLIWritesConfigFile(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home)

	result, err := service.Sync(AppKiloCLI, "http://127.0.0.1:8095", "sk-cliro_test", "claude-sonnet-4.5")
	if err != nil {
		t.Fatalf("sync kilo cli: %v", err)
	}
	if result.CurrentBaseURL != "http://127.0.0.1:8095/v1" {
		t.Fatalf("base url = %q", result.CurrentBaseURL)
	}

	configPath := filepath.Join(home, ".config", "kilo", "opencode.json")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read kilo config: %v", err)
	}
	configText := string(configData)
	if !strings.Contains(configText, `"CLIRO": {`) {
		t.Fatalf("expected CLIRO provider in kilo config: %s", configText)
	}
	if !strings.Contains(configText, `"baseURL": "http://127.0.0.1:8095/v1"`) {
		t.Fatalf("expected base url in kilo config: %s", configText)
	}
	if !strings.Contains(configText, `"model": "claude-sonnet-4.5"`) {
		t.Fatalf("expected selected model in kilo config: %s", configText)
	}
	if !strings.Contains(configText, `"claude-sonnet-4.5": {`) {
		t.Fatalf("expected selected model in kilo config: %s", configText)
	}
}

func TestStatusesReflectSyncedBaseURLs(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home)

	if _, err := service.Sync(AppClaudeCode, "http://127.0.0.1:8095/v1", "sk-cliro_test", "gpt-5.3-codex"); err != nil {
		t.Fatalf("sync claude code: %v", err)
	}
	if _, err := service.Sync(AppCodexAI, "http://127.0.0.1:8095", "sk-cliro_test", "gpt-5.3-codex"); err != nil {
		t.Fatalf("sync codex ai: %v", err)
	}

	statuses, err := service.Statuses("http://127.0.0.1:8095/v1")
	if err != nil {
		t.Fatalf("statuses: %v", err)
	}
	if len(statuses) != 4 {
		t.Fatalf("status count = %d", len(statuses))
	}
	claude := statusByID(statuses, string(AppClaudeCode))
	codex := statusByID(statuses, string(AppCodexAI))
	if claude == nil || codex == nil {
		t.Fatalf("expected claude and codex status entries: %+v", statuses)
	}
	if !claude.Synced || !codex.Synced {
		t.Fatalf("expected claude and codex synced: %+v", statuses)
	}
}

func TestStatusesMarkInstalledWhenConfigFilesExist(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home)

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:8095/v1"},"model":"gpt-5.3-codex"}`), 0o600); err != nil {
		t.Fatalf("write settings file: %v", err)
	}

	statuses, err := service.Statuses("http://127.0.0.1:8095/v1")
	if err != nil {
		t.Fatalf("statuses: %v", err)
	}

	claude := statusByID(statuses, string(AppClaudeCode))
	if claude == nil {
		t.Fatalf("expected claude status entry: %+v", statuses)
	}
	if !claude.Installed {
		t.Fatalf("expected existing claude config to mark target installed: %+v", *claude)
	}
}

func TestStatusesMarkKiloInstalledWhenConfigDirExists(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home)

	kiloDir := filepath.Join(home, ".config", "kilo")
	if err := os.MkdirAll(kiloDir, 0o700); err != nil {
		t.Fatalf("mkdir kilo dir: %v", err)
	}

	statuses, err := service.Statuses("http://127.0.0.1:8095/v1")
	if err != nil {
		t.Fatalf("statuses: %v", err)
	}

	kilo := statusByID(statuses, string(AppKiloCLI))
	if kilo == nil {
		t.Fatalf("expected kilo status entry: %+v", statuses)
	}
	if !kilo.Installed {
		t.Fatalf("expected kilo to be marked installed when config dir exists: %+v", *kilo)
	}
	if kilo.InstallPath != kiloDir {
		t.Fatalf("expected kilo install path = %q, got %q", kiloDir, kilo.InstallPath)
	}
}

func TestStatusesMarkInstalledWhenKnownConfigDirExists(t *testing.T) {
	testCases := []struct {
		name string
		app  App
		dir  string
	}{
		{name: "claude", app: AppClaudeCode, dir: filepath.Join(".claude")},
		{name: "opencode", app: AppOpenCode, dir: filepath.Join(".config", "opencode")},
		{name: "codex", app: AppCodexAI, dir: filepath.Join(".codex")},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			home := t.TempDir()
			service := newTestService(home)

			expectedPath := filepath.Join(home, testCase.dir)
			if err := os.MkdirAll(expectedPath, 0o700); err != nil {
				t.Fatalf("mkdir config dir: %v", err)
			}

			statuses, err := service.Statuses("http://127.0.0.1:8095/v1")
			if err != nil {
				t.Fatalf("statuses: %v", err)
			}

			status := statusByID(statuses, string(testCase.app))
			if status == nil {
				t.Fatalf("expected status entry for %s: %+v", testCase.app, statuses)
			}
			if !status.Installed {
				t.Fatalf("expected app marked installed when config dir exists: %+v", *status)
			}
			if status.InstallPath != expectedPath {
				t.Fatalf("expected install path = %q, got %q", expectedPath, status.InstallPath)
			}
		})
	}
}

func TestGetInstallStatus_UsesCacheUntilExpired(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home)
	lookups := 0
	service.lookPathFn = func(string) (string, error) {
		lookups++
		return filepath.Join(home, "claude.exe"), nil
	}
	now := time.Unix(1000, 0)
	service.nowFn = func() time.Time { return now }

	installed, _, _ := service.getInstallStatus("claude", false)
	if !installed {
		t.Fatalf("expected installed result")
	}
	installed, _, _ = service.getInstallStatus("claude", false)
	if !installed {
		t.Fatalf("expected cached installed result")
	}
	if lookups != 1 {
		t.Fatalf("expected one lookup before cache expiry, got %d", lookups)
	}

	now = now.Add(installProbeCacheTTL + time.Second)
	installed, _, _ = service.getInstallStatus("claude", false)
	if !installed {
		t.Fatalf("expected installed result after cache expiry")
	}
	if lookups != 2 {
		t.Fatalf("expected second lookup after cache expiry, got %d", lookups)
	}

	installed, _, _ = service.getInstallStatus("claude", true)
	if !installed {
		t.Fatalf("expected installed result with force refresh")
	}
	if lookups != 3 {
		t.Fatalf("expected forced lookup to bypass cache, got %d", lookups)
	}
}

func statusByID(statuses []Status, id string) *Status {
	for index := range statuses {
		if statuses[index].ID == id {
			return &statuses[index]
		}
	}
	return nil
}
