package cliconfig

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cliro-go/internal/logger"
	"cliro-go/internal/route"
)

type Service struct {
	log          *logger.Logger
	homeDirFn    func() (string, error)
	lookPathFn   func(string) (string, error)
	nowFn        func() time.Time
	installMu    sync.Mutex
	installCache map[string]installProbeCache
}

func NewService(log *logger.Logger) *Service {
	return &Service{
		log:          log,
		homeDirFn:    os.UserHomeDir,
		lookPathFn:   exec.LookPath,
		nowFn:        time.Now,
		installCache: make(map[string]installProbeCache),
	}
}

func (s *Service) ModelCatalog() []CatalogModel {
	catalog := route.CatalogModels()
	out := make([]CatalogModel, 0, len(catalog))
	for _, model := range catalog {
		out = append(out, CatalogModel{ID: model.ID, OwnedBy: model.OwnedBy})
	}
	return out
}

func (s *Service) Statuses(baseURL string) ([]Status, error) {
	home, err := s.homeDirFn()
	if err != nil {
		return nil, fmt.Errorf("resolve user home directory: %w", err)
	}
	out := make([]Status, 0, len(appDefinitions))
	for _, app := range appDefinitions {
		status, err := s.statusForApp(app, home, baseURL)
		if err != nil {
			return nil, err
		}
		out = append(out, status)
	}
	return out, nil
}

func (s *Service) Sync(app App, baseURL string, apiKey string, model string) (SyncResult, error) {
	home, err := s.homeDirFn()
	if err != nil {
		return SyncResult{}, fmt.Errorf("resolve user home directory: %w", err)
	}

	def, ok := appDefinitionByID(app)
	if !ok {
		return SyncResult{}, fmt.Errorf("unsupported cli sync target: %s", app)
	}

	model = strings.TrimSpace(model)
	if model != "" && !s.hasModel(model) {
		return SyncResult{}, fmt.Errorf("unsupported model: %s", model)
	}

	files := def.files(home)
	expectedBaseURL := expectedBaseURLForApp(app, baseURL)
	for _, file := range files {
		if err := ensureFileParent(file.Path); err != nil {
			return SyncResult{}, err
		}
		if err := createBackupIfNeeded(file); err != nil {
			return SyncResult{}, err
		}

		current, err := os.ReadFile(file.Path)
		if err != nil && !os.IsNotExist(err) {
			return SyncResult{}, fmt.Errorf("read %s: %w", file.Path, err)
		}

		updated, err := patchFile(app, file.Name, string(current), expectedBaseURL, strings.TrimSpace(apiKey), model)
		if err != nil {
			return SyncResult{}, err
		}
		if err := writeFileAtomic(file.Path, []byte(updated)); err != nil {
			return SyncResult{}, err
		}
	}

	if s.log != nil {
		s.log.Info("cli-sync", fmt.Sprintf("synced %s config to %s model=%q", def.label, expectedBaseURL, model))
	}

	return SyncResult{
		ID:             string(def.id),
		Label:          def.label,
		Model:          model,
		CurrentBaseURL: expectedBaseURL,
		Files:          files,
	}, nil
}

func (s *Service) ReadConfigFile(app App, path string) (string, error) {
	file, err := s.resolveConfigFile(app, path)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(file.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", file.Path, err)
	}
	return string(data), nil
}

func (s *Service) WriteConfigFile(app App, path string, content string) error {
	file, err := s.resolveConfigFile(app, path)
	if err != nil {
		return err
	}
	if err := ensureFileParent(file.Path); err != nil {
		return err
	}
	if err := createBackupIfNeeded(file); err != nil {
		return err
	}
	if err := writeFileAtomic(file.Path, []byte(content)); err != nil {
		return err
	}
	if s.log != nil {
		s.log.Info("cli-sync", fmt.Sprintf("saved manual edits for %s -> %s", app, file.Path))
	}
	return nil
}

func (s *Service) statusForApp(app appDefinition, home string, baseURL string) (Status, error) {
	files := app.files(home)
	expectedBaseURL := expectedBaseURLForApp(app.id, baseURL)
	installed, version, installPath := s.getInstallStatus(app.command, true)
	configInstalled, configPath := detectInstalledFromConfig(app.id, home, files)
	if configInstalled {
		installed = true
		if strings.TrimSpace(configPath) != "" {
			installPath = configPath
		}
	}
	status := Status{ID: string(app.id), Label: app.label, Installed: installed, InstallPath: installPath, Version: version, Files: files}

	switch app.id {
	case AppClaudeCode:
		currentBaseURL, currentModel, onboardingDone, err := readClaudeStatus(files)
		if err != nil {
			return Status{}, err
		}
		status.CurrentBaseURL = currentBaseURL
		status.CurrentModel = currentModel
		status.Synced = onboardingDone && sameURL(currentBaseURL, expectedBaseURL)
	case AppOpenCode, AppKiloCLI:
		currentBaseURL, currentModel, err := readOpenCodeStatus(files)
		if err != nil {
			return Status{}, err
		}
		status.CurrentBaseURL = currentBaseURL
		status.CurrentModel = currentModel
		status.Synced = sameURL(currentBaseURL, expectedBaseURL)
	case AppCodexAI:
		currentBaseURL, currentModel, err := readCodexStatus(files)
		if err != nil {
			return Status{}, err
		}
		status.CurrentBaseURL = currentBaseURL
		status.CurrentModel = currentModel
		status.Synced = sameURL(currentBaseURL, expectedBaseURL)
	}

	return status, nil
}

func (s *Service) hasModel(model string) bool {
	for _, item := range s.ModelCatalog() {
		if item.ID == model {
			return true
		}
	}
	return false
}

func (s *Service) resolveConfigFile(app App, path string) (FileInfo, error) {
	home, err := s.homeDirFn()
	if err != nil {
		return FileInfo{}, fmt.Errorf("resolve user home directory: %w", err)
	}
	def, ok := appDefinitionByID(app)
	if !ok {
		return FileInfo{}, fmt.Errorf("unsupported cli sync target: %s", app)
	}
	normalizedPath := filepath.Clean(strings.TrimSpace(path))
	for _, file := range def.files(home) {
		if filepath.Clean(file.Path) == normalizedPath {
			return file, nil
		}
	}
	return FileInfo{}, fmt.Errorf("unsupported cli sync file: %s", path)
}
