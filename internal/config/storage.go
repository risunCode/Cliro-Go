package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Storage handles multi-file persistence for app data
type Storage struct {
	mu      sync.RWMutex
	dataDir string
	
	// File paths
	configPath   string
	accountsPath string
	statsPath    string
}

// AppSettings contains general application settings
type AppSettings struct {
	ProxyPort      int  `json:"proxyPort"`
	AllowLAN       bool `json:"allowLan"`
	AutoStartProxy bool `json:"autoStartProxy"`
}

// NewStorage creates a new multi-file storage manager
func NewStorage(dataDir string) (*Storage, error) {
	s := &Storage{
		dataDir:      dataDir,
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
func (s *Storage) LoadSettings() (AppSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AppSettings{
				ProxyPort:      defaultProxyPort,
				AllowLAN:       false,
				AutoStartProxy: true,
			}, nil
		}
		return AppSettings{}, err
	}
	
	var settings AppSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return AppSettings{}, err
	}
	
	// Apply defaults
	if settings.ProxyPort == 0 {
		settings.ProxyPort = defaultProxyPort
	}
	
	return settings, nil
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
func (s *Storage) LoadAccounts() ([]Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	data, err := os.ReadFile(s.accountsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Account{}, nil
		}
		return nil, err
	}
	
	var accounts []Account
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, err
	}
	
	if accounts == nil {
		return []Account{}, nil
	}
	
	return accounts, nil
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
func (s *Storage) LoadStats() (ProxyStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	data, err := os.ReadFile(s.statsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProxyStats{}, nil
		}
		return ProxyStats{}, err
	}
	
	var stats ProxyStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return ProxyStats{}, err
	}
	
	return stats, nil
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
