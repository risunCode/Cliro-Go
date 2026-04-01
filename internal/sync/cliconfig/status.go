package cliconfig

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const installProbeCacheTTL = 60 * time.Second

func (s *Service) checkInstalled(command string) (bool, string, string) {
	path := s.findInstalledCommandPath(command)
	if strings.TrimSpace(path) == "" {
		return false, "", ""
	}

	cmd := exec.Command(path, "--version")
	configureCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return true, "", path
	}

	return true, extractVersion(string(output)), path
}

func (s *Service) getInstallStatus(command string, force bool) (bool, string, string) {
	if s == nil {
		return false, "", ""
	}

	now := time.Now()
	if s.nowFn != nil {
		now = s.nowFn()
	}

	if !force {
		s.installMu.Lock()
		if cached, ok := s.installCache[command]; ok && now.Sub(cached.CheckedAt) < installProbeCacheTTL {
			s.installMu.Unlock()
			return cached.Installed, cached.Version, cached.Path
		}
		s.installMu.Unlock()
	}

	installed, version, installPath := s.checkInstalled(command)

	s.installMu.Lock()
	s.installCache[command] = installProbeCache{Installed: installed, Path: installPath, Version: version, CheckedAt: now}
	s.installMu.Unlock()

	return installed, version, installPath
}

func (s *Service) findInstalledCommandPath(command string) string {
	for _, executableName := range commandExecutableNames(command) {
		path, err := s.lookPathFn(executableName)
		if err == nil && strings.TrimSpace(path) != "" {
			return path
		}
	}
	if path := scanCommonCLIPath(command); strings.TrimSpace(path) != "" {
		return path
	}
	return scanPathEnv(command)
}

func detectInstalledFromConfig(app App, home string, files []FileInfo) (bool, string) {
	directories := appInstallDirectories(app, home)
	for _, dir := range directories {
		if directoryExists(dir) {
			return true, dir
		}
	}
	for _, file := range files {
		info, err := os.Stat(file.Path)
		if err == nil && !info.IsDir() {
			return true, filepath.Dir(file.Path)
		}
	}
	return false, ""
}

func appInstallDirectories(app App, home string) []string {
	switch app {
	case AppClaudeCode:
		return []string{filepath.Join(home, ".claude")}
	case AppOpenCode:
		return []string{filepath.Join(home, ".config", "opencode")}
	case AppKiloCLI:
		return []string{filepath.Join(home, ".config", "kilo")}
	case AppCodexAI:
		return []string{filepath.Join(home, ".codex")}
	default:
		return nil
	}
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func scanCommonCLIPath(command string) string {
	candidates := make([]string, 0)
	names := commandExecutableNames(command)
	if appData := os.Getenv("APPDATA"); appData != "" {
		for _, name := range names {
			candidates = append(candidates, filepath.Join(appData, "npm", name))
		}
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		for _, name := range names {
			candidates = append(candidates, filepath.Join(localAppData, "pnpm", name), filepath.Join(localAppData, "Yarn", "bin", name))
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		for _, name := range names {
			candidates = append(candidates, filepath.Join(home, ".bun", "bin", name), filepath.Join(home, ".local", "bin", name), filepath.Join(home, "bin", name))
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func scanPathEnv(command string) string {
	pathEnv := strings.TrimSpace(os.Getenv("PATH"))
	if pathEnv == "" {
		return ""
	}
	names := commandExecutableNames(command)
	for _, directory := range filepath.SplitList(pathEnv) {
		trimmedDir := strings.TrimSpace(directory)
		if trimmedDir == "" {
			continue
		}
		for _, name := range names {
			candidate := filepath.Join(trimmedDir, name)
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	return ""
}

func commandExecutableNames(command string) []string {
	base := strings.TrimSpace(command)
	if base == "" {
		return nil
	}
	if runtime.GOOS != "windows" {
		return []string{base}
	}
	return []string{base, base + ".cmd", base + ".exe", base + ".bat"}
}

func extractVersion(raw string) string {
	matcher := regexp.MustCompile(`(\d+\.\d+(?:\.\d+)?)`)
	return matcher.FindString(strings.TrimSpace(raw))
}

func readClaudeStatus(files []FileInfo) (string, string, bool, error) {
	var currentBaseURL string
	var currentModel string
	onboardingDone := false
	for _, file := range files {
		if _, err := os.Stat(file.Path); os.IsNotExist(err) {
			continue
		}
		data, err := os.ReadFile(file.Path)
		if err != nil {
			return "", "", false, fmt.Errorf("read %s: %w", file.Path, err)
		}
		jsonDoc, err := parseJSONObject(data)
		if err != nil {
			return "", "", false, fmt.Errorf("parse %s: %w", file.Path, err)
		}
		if file.Name == ".claude.json" {
			onboardingDone = jsonBool(jsonDoc, "hasCompletedOnboarding")
			continue
		}
		env := jsonMap(jsonDoc, "env")
		currentBaseURL = stringValue(env["ANTHROPIC_BASE_URL"])
		currentModel = stringValue(jsonDoc["model"])
	}
	return currentBaseURL, currentModel, onboardingDone, nil
}

func readCodexStatus(files []FileInfo) (string, string, error) {
	var currentBaseURL string
	var currentModel string
	for _, file := range files {
		if file.Name != "config.toml" {
			continue
		}
		if _, err := os.Stat(file.Path); os.IsNotExist(err) {
			continue
		}
		data, err := os.ReadFile(file.Path)
		if err != nil {
			return "", "", fmt.Errorf("read %s: %w", file.Path, err)
		}
		root, sections := splitTOMLRootAndSections(string(data))
		currentModel = parseTOMLQuotedValue(root, "model")
		currentBaseURL = parseTOMLSectionQuotedValue(sections, "model_providers.custom", "base_url")
		break
	}
	return currentBaseURL, currentModel, nil
}

func readOpenCodeStatus(files []FileInfo) (string, string, error) {
	for _, file := range files {
		if _, err := os.Stat(file.Path); os.IsNotExist(err) {
			continue
		}
		data, err := os.ReadFile(file.Path)
		if err != nil {
			return "", "", fmt.Errorf("read %s: %w", file.Path, err)
		}
		jsonDoc, err := parseJSONObject(data)
		if err != nil {
			return "", "", fmt.Errorf("parse %s: %w", file.Path, err)
		}
		provider, ok := findOpenCodeProvider(jsonDoc)
		if !ok || provider == nil {
			return "", "", nil
		}
		options := jsonMap(provider, "options")
		models := jsonMap(provider, "models")
		currentModel := parseOpenCodeTopLevelModel(stringValue(jsonDoc["model"]))
		if currentModel == "" {
			currentModel = strings.TrimSpace(stringValue(options["model"]))
		}
		if currentModel == "" {
			currentModel = firstModelKey(models)
		}
		return stringValue(options["baseURL"]), currentModel, nil
	}
	return "", "", nil
}
