package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Storage handles multi-file persistence for app data
type Storage struct {
	mu sync.RWMutex

	// File paths
	configPath   string
	accountsPath string
	statsPath    string
}

// AppSettings contains general application settings
type AppSettings struct {
	ProxyPort         int                 `json:"proxyPort"`
	AllowLAN          bool                `json:"allowLan"`
	AutoStartProxy    bool                `json:"autoStartProxy"`
	ProxyAPIKey       string              `json:"proxyApiKey,omitempty"`
	AuthorizationMode bool                `json:"authorizationMode,omitempty"`
	SchedulingMode    string              `json:"schedulingMode,omitempty"`
	Cloudflared       CloudflaredSettings `json:"cloudflared,omitempty"`
	Thinking          ThinkingSettings    `json:"thinking,omitempty"`
	ModelAliases      map[string]string   `json:"modelAliases,omitempty"`
}

// NewStorage creates a new multi-file storage manager
func NewStorage(dataDir string) (*Storage, error) {
	s := &Storage{
		configPath:   filepath.Join(dataDir, "config.json"),
		accountsPath: filepath.Join(dataDir, "accounts.json"),
		statsPath:    filepath.Join(dataDir, "stats.json"),
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}

	return s, nil
}

// LoadSettings loads application settings
func defaultAppSettings() AppSettings {
	return AppSettings{
		ProxyPort:      defaultProxyPort,
		AllowLAN:       false,
		AutoStartProxy: true,
		SchedulingMode: string(SchedulingModeBalance),
		Cloudflared:    defaultCloudflaredSettings(),
		Thinking:       defaultThinkingSettings(),
	}
}

func (s *Storage) recoverCorruptedFile(path string, code string, raw []byte, fallback any, parseErr error) StartupWarning {
	messageParts := []string{fmt.Sprintf("Recovered corrupted configuration from %s", filepath.Base(path))}
	if parseErr != nil {
		messageParts = append(messageParts, parseErr.Error())
	}

	warning := StartupWarning{
		Code:     code,
		FilePath: path,
	}

	if len(raw) > 0 {
		backupPath := path + ".corrupt-" + time.Now().Format("20060102-150405000000000")
		if err := os.WriteFile(backupPath, raw, 0o600); err != nil {
			messageParts = append(messageParts, "backup failed: "+err.Error())
		} else {
			warning.BackupPath = backupPath
			messageParts = append(messageParts, "backup saved to "+backupPath)
		}
	}

	normalized, err := json.MarshalIndent(fallback, "", "  ")
	if err != nil {
		messageParts = append(messageParts, "rewrite encoding failed: "+err.Error())
	} else if err := os.WriteFile(path, normalized, 0o600); err != nil {
		messageParts = append(messageParts, "rewrite failed: "+err.Error())
	}

	warning.Message = strings.Join(messageParts, "; ")
	return warning
}

// LoadSettings loads application settings
func (s *Storage) LoadSettings() (AppSettings, []StartupWarning, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultAppSettings(), nil, nil
		}
		return AppSettings{}, nil, err
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return defaultAppSettings(), nil, nil
	}

	settings := defaultAppSettings()
	if err := json.Unmarshal(trimmed, &settings); err != nil {
		fallback := defaultAppSettings()
		warning := s.recoverCorruptedFile(s.configPath, "settings_corrupted", trimmed, fallback, err)
		return fallback, []StartupWarning{warning}, nil
	}

	// Apply defaults
	if settings.ProxyPort == 0 {
		settings.ProxyPort = defaultProxyPort
	}
	settings.SchedulingMode = string(normalizeSchedulingMode(SchedulingMode(settings.SchedulingMode)))
	settings.Cloudflared = normalizeCloudflaredSettings(settings.Cloudflared)
	settings.Thinking = normalizeThinkingSettings(settings.Thinking)

	return settings, nil, nil
}

// SaveSettings saves application settings
func (s *Storage) SaveSettings(settings AppSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.configPath, data, 0o600)
}

// LoadAccounts loads all accounts
func (s *Storage) LoadAccounts() ([]Account, []StartupWarning, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.accountsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Account{}, nil, nil
		}
		return nil, nil, err
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return []Account{}, nil, nil
	}

	var accounts []Account
	if err := json.Unmarshal(trimmed, &accounts); err != nil {
		fallback := []Account{}
		warning := s.recoverCorruptedFile(s.accountsPath, "accounts_corrupted", trimmed, fallback, err)
		return fallback, []StartupWarning{warning}, nil
	}

	if accounts == nil {
		return []Account{}, nil, nil
	}

	return accounts, nil, nil
}

// SaveAccounts saves all accounts
func (s *Storage) SaveAccounts(accounts []Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.accountsPath, data, 0o600)
}

// LoadStats loads proxy statistics
func (s *Storage) LoadStats() (ProxyStats, []StartupWarning, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.statsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProxyStats{}, nil, nil
		}
		return ProxyStats{}, nil, err
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return ProxyStats{}, nil, nil
	}

	var stats ProxyStats
	if err := json.Unmarshal(trimmed, &stats); err != nil {
		fallback := ProxyStats{}
		warning := s.recoverCorruptedFile(s.statsPath, "stats_corrupted", trimmed, fallback, err)
		return fallback, []StartupWarning{warning}, nil
	}

	return stats, nil, nil
}

// SaveStats saves proxy statistics
func (s *Storage) SaveStats(stats ProxyStats) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.statsPath, data, 0o600)
}
