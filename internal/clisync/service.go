package clisync

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"cliro-go/internal/logger"
	"cliro-go/internal/route"
)

type App string

const (
	AppClaudeCode App = "claude-code"
	AppOpenCode   App = "opencode-cli"
	AppCodexAI    App = "codex-ai"
	AppGeminiCLI  App = "gemini-cli"
)

type FileInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type Status struct {
	ID             string     `json:"id"`
	Label          string     `json:"label"`
	Installed      bool       `json:"installed"`
	Version        string     `json:"version,omitempty"`
	Synced         bool       `json:"synced"`
	CurrentBaseURL string     `json:"currentBaseUrl,omitempty"`
	CurrentModel   string     `json:"currentModel,omitempty"`
	Files          []FileInfo `json:"files"`
}

type SyncResult struct {
	ID             string     `json:"id"`
	Label          string     `json:"label"`
	Model          string     `json:"model,omitempty"`
	CurrentBaseURL string     `json:"currentBaseUrl,omitempty"`
	Files          []FileInfo `json:"files"`
}

type Model struct {
	ID      string `json:"id"`
	OwnedBy string `json:"ownedBy"`
}

type Service struct {
	log          *logger.Logger
	homeDirFn    func() (string, error)
	lookPathFn   func(string) (string, error)
	nowFn        func() time.Time
	installMu    sync.Mutex
	installCache map[string]installProbeCache
}

type installProbeCache struct {
	Installed bool
	Version   string
	CheckedAt time.Time
}

const installProbeCacheTTL = 60 * time.Second

type appDefinition struct {
	id      App
	label   string
	command string
	files   func(home string) []FileInfo
}

var appDefinitions = []appDefinition{
	{
		id:      AppClaudeCode,
		label:   "Claude Code Config",
		command: "claude",
		files: func(home string) []FileInfo {
			return []FileInfo{
				{Name: ".claude.json", Path: filepath.Join(home, ".claude.json")},
				{Name: "settings.json", Path: filepath.Join(home, ".claude", "settings.json")},
			}
		},
	},
	{
		id:      AppOpenCode,
		label:   "OpenCode Config",
		command: "opencode",
		files: func(home string) []FileInfo {
			return []FileInfo{{Name: "opencode.json", Path: filepath.Join(home, ".config", "opencode", "opencode.json")}}
		},
	},
	{
		id:      AppCodexAI,
		label:   "Codex AI Config",
		command: "codex",
		files: func(home string) []FileInfo {
			return []FileInfo{
				{Name: "auth.json", Path: filepath.Join(home, ".codex", "auth.json")},
				{Name: "config.toml", Path: filepath.Join(home, ".codex", "config.toml")},
			}
		},
	},
	{
		id:      AppGeminiCLI,
		label:   "Gemini CLI Config",
		command: "gemini",
		files: func(home string) []FileInfo {
			return []FileInfo{
				{Name: ".env", Path: filepath.Join(home, ".gemini", ".env")},
				{Name: "settings.json", Path: filepath.Join(home, ".gemini", "settings.json")},
				{Name: "config.json", Path: filepath.Join(home, ".gemini", "config.json")},
			}
		},
	},
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

func (s *Service) ModelCatalog() []Model {
	catalog := route.CatalogModels(route.DefaultThinkingSuffix)
	out := make([]Model, 0, len(catalog))
	for _, model := range catalog {
		out = append(out, Model{ID: model.ID, OwnedBy: model.OwnedBy})
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
	installed, version := s.getInstallStatus(app.command)
	if !installed && hasAnyExistingFile(files) {
		installed = true
	}
	status := Status{
		ID:        string(app.id),
		Label:     app.label,
		Installed: installed,
		Version:   version,
		Files:     files,
	}

	switch app.id {
	case AppClaudeCode:
		currentBaseURL, currentModel, onboardingDone, err := readClaudeStatus(files)
		if err != nil {
			return Status{}, err
		}
		status.CurrentBaseURL = currentBaseURL
		status.CurrentModel = currentModel
		status.Synced = onboardingDone && sameURL(currentBaseURL, expectedBaseURL)
	case AppOpenCode:
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
	case AppGeminiCLI:
		currentBaseURL, currentModel, authConfigured, err := readGeminiStatus(files)
		if err != nil {
			return Status{}, err
		}
		status.CurrentBaseURL = currentBaseURL
		status.CurrentModel = currentModel
		status.Synced = authConfigured && sameURL(currentBaseURL, expectedBaseURL)
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

func (s *Service) checkInstalled(command string) (bool, string) {
	path, err := s.lookPathFn(command)
	if err != nil {
		path = scanCommonCLIPath(command)
	}
	if strings.TrimSpace(path) == "" {
		return false, ""
	}

	cmd := exec.Command(path, "--version")
	configureCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return true, ""
	}

	return true, extractVersion(string(output))
}

func (s *Service) getInstallStatus(command string) (bool, string) {
	if s == nil {
		return false, ""
	}

	now := time.Now()
	if s.nowFn != nil {
		now = s.nowFn()
	}

	s.installMu.Lock()
	if cached, ok := s.installCache[command]; ok && now.Sub(cached.CheckedAt) < installProbeCacheTTL {
		s.installMu.Unlock()
		return cached.Installed, cached.Version
	}
	s.installMu.Unlock()

	installed, version := s.checkInstalled(command)

	s.installMu.Lock()
	s.installCache[command] = installProbeCache{
		Installed: installed,
		Version:   version,
		CheckedAt: now,
	}
	s.installMu.Unlock()

	return installed, version
}

func hasAnyExistingFile(files []FileInfo) bool {
	for _, file := range files {
		info, err := os.Stat(file.Path)
		if err == nil && !info.IsDir() {
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

func appDefinitionByID(id App) (appDefinition, bool) {
	for _, item := range appDefinitions {
		if item.id == id {
			return item, true
		}
	}
	return appDefinition{}, false
}

func expectedBaseURLForApp(app App, baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return ""
	}
	if app == AppCodexAI || app == AppOpenCode {
		if strings.HasSuffix(trimmed, "/v1") {
			return trimmed
		}
		return trimmed + "/v1"
	}
	return strings.TrimSuffix(trimmed, "/v1")
}

func scanCommonCLIPath(command string) string {
	candidates := make([]string, 0)
	if appData := os.Getenv("APPDATA"); appData != "" {
		candidates = append(candidates, filepath.Join(appData, "npm", command+cmdExt()))
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		candidates = append(candidates,
			filepath.Join(localAppData, "pnpm", command+cmdExt()),
			filepath.Join(localAppData, "Yarn", "bin", command+cmdExt()),
		)
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		candidates = append(candidates,
			filepath.Join(home, ".bun", "bin", command+binExt()),
			filepath.Join(home, ".local", "bin", command+binExt()),
			filepath.Join(home, "bin", command+binExt()),
		)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func cmdExt() string {
	if runtime.GOOS == "windows" {
		return ".cmd"
	}
	return ""
}

func binExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func extractVersion(raw string) string {
	matcher := regexp.MustCompile(`(\d+\.\d+(?:\.\d+)?)`)
	match := matcher.FindString(strings.TrimSpace(raw))
	return match
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

func readGeminiStatus(files []FileInfo) (string, string, bool, error) {
	var currentBaseURL string
	var currentModel string
	authConfigured := false
	for _, file := range files {
		if _, err := os.Stat(file.Path); os.IsNotExist(err) {
			continue
		}
		data, err := os.ReadFile(file.Path)
		if err != nil {
			return "", "", false, fmt.Errorf("read %s: %w", file.Path, err)
		}
		if file.Name == ".env" {
			content := string(data)
			currentBaseURL = parseEnvValue(content, "GOOGLE_GEMINI_BASE_URL")
			currentModel = parseEnvValue(content, "GOOGLE_GEMINI_MODEL")
			continue
		}
		jsonDoc, err := parseJSONObject(data)
		if err != nil {
			return "", "", false, fmt.Errorf("parse %s: %w", file.Path, err)
		}
		if stringValue(jsonMap(jsonMap(jsonDoc, "security"), "auth")["selectedType"]) == "gemini-api-key" {
			authConfigured = true
		}
	}
	return currentBaseURL, currentModel, authConfigured, nil
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
		provider := jsonMap(jsonMap(jsonDoc, "provider"), "cliro-go")
		options := jsonMap(provider, "options")
		models := jsonMap(provider, "models")
		currentModel := ""
		for modelID := range models {
			currentModel = modelID
			break
		}
		return stringValue(options["baseURL"]), currentModel, nil
	}
	return "", "", nil
}

func patchFile(app App, fileName string, content string, baseURL string, apiKey string, model string) (string, error) {
	switch app {
	case AppClaudeCode:
		return patchClaudeFile(fileName, content, baseURL, apiKey, model)
	case AppOpenCode:
		return patchOpenCodeFile(content, baseURL, apiKey, model)
	case AppCodexAI:
		return patchCodexFile(fileName, content, baseURL, apiKey, model)
	case AppGeminiCLI:
		return patchGeminiFile(fileName, content, baseURL, apiKey, model)
	default:
		return "", fmt.Errorf("unsupported cli sync target: %s", app)
	}
}

func patchClaudeFile(fileName string, content string, baseURL string, apiKey string, model string) (string, error) {
	jsonDoc, err := parseJSONObject([]byte(content))
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", fileName, err)
	}
	if fileName == ".claude.json" {
		jsonDoc["hasCompletedOnboarding"] = true
		return marshalJSON(jsonDoc)
	}

	env := jsonMap(jsonDoc, "env")
	env["ANTHROPIC_BASE_URL"] = baseURL
	if apiKey != "" {
		env["ANTHROPIC_API_KEY"] = apiKey
	} else {
		delete(env, "ANTHROPIC_API_KEY")
	}
	delete(env, "ANTHROPIC_AUTH_TOKEN")
	delete(env, "ANTHROPIC_MODEL")
	delete(env, "ANTHROPIC_DEFAULT_HAIKU_MODEL")
	delete(env, "ANTHROPIC_DEFAULT_OPUS_MODEL")
	delete(env, "ANTHROPIC_DEFAULT_SONNET_MODEL")
	jsonDoc["env"] = env
	if model != "" {
		jsonDoc["model"] = model
	}
	return marshalJSON(jsonDoc)
}

func patchCodexFile(fileName string, content string, baseURL string, apiKey string, model string) (string, error) {
	if fileName == "auth.json" {
		jsonDoc, err := parseJSONObject([]byte(content))
		if err != nil {
			return "", fmt.Errorf("parse %s: %w", fileName, err)
		}
		jsonDoc["OPENAI_API_KEY"] = apiKey
		jsonDoc["OPENAI_BASE_URL"] = baseURL
		return marshalJSON(jsonDoc)
	}
	return patchCodexTOML(content, baseURL, model), nil
}

func patchOpenCodeFile(content string, baseURL string, apiKey string, model string) (string, error) {
	jsonDoc, err := parseJSONObject([]byte(content))
	if err != nil {
		return "", fmt.Errorf("parse opencode.json: %w", err)
	}
	if _, ok := jsonDoc["$schema"]; !ok {
		jsonDoc["$schema"] = "https://opencode.ai/config.json"
	}
	providers := jsonMap(jsonDoc, "provider")
	provider := jsonMap(providers, "cliro-go")
	provider["name"] = "CLIro-Go"
	provider["npm"] = "@ai-sdk/openai-compatible"
	options := jsonMap(provider, "options")
	options["baseURL"] = baseURL
	options["apiKey"] = apiKey
	provider["options"] = options
	models := jsonMap(provider, "models")
	for key := range models {
		delete(models, key)
	}
	if model != "" {
		models[model] = map[string]any{"name": model}
	}
	provider["models"] = models
	providers["cliro-go"] = provider
	jsonDoc["provider"] = providers
	return marshalJSON(jsonDoc)
}

func patchGeminiFile(fileName string, content string, baseURL string, apiKey string, model string) (string, error) {
	if fileName == ".env" {
		return patchEnvFile(content, map[string]string{
			"GOOGLE_GEMINI_BASE_URL": baseURL,
			"GEMINI_API_KEY":         apiKey,
			"GOOGLE_GEMINI_MODEL":    model,
		}), nil
	}

	jsonDoc, err := parseJSONObject([]byte(content))
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", fileName, err)
	}
	security := jsonMap(jsonDoc, "security")
	auth := jsonMap(security, "auth")
	auth["selectedType"] = "gemini-api-key"
	security["auth"] = auth
	jsonDoc["security"] = security
	return marshalJSON(jsonDoc)
}

func patchCodexTOML(content string, baseURL string, model string) string {
	root, sections := splitTOMLRootAndSections(content)
	rootLines := make([]string, 0)
	for _, line := range strings.Split(root, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "model_provider") || strings.HasPrefix(trimmed, "model =") || strings.HasPrefix(trimmed, "openai_api_key") || strings.HasPrefix(trimmed, "openai_base_url") {
			continue
		}
		rootLines = append(rootLines, line)
	}
	newRoot := []string{`model_provider = "custom"`}
	if model != "" {
		newRoot = append(newRoot, fmt.Sprintf(`model = "%s"`, escapeTOMLString(model)))
	}
	newRoot = append(newRoot, rootLines...)

	sectionBody := []string{
		"[model_providers.custom]",
		`name = "custom"`,
		`wire_api = "responses"`,
		`requires_openai_auth = true`,
		fmt.Sprintf(`base_url = "%s"`, escapeTOMLString(baseURL)),
	}
	if model != "" {
		sectionBody = append(sectionBody, fmt.Sprintf(`model = "%s"`, escapeTOMLString(model)))
	}
	sections = replaceTOMLSection(sections, "model_providers.custom", strings.Join(sectionBody, "\n"))

	resultParts := []string{strings.TrimSpace(strings.Join(newRoot, "\n"))}
	trimmedSections := strings.TrimSpace(sections)
	if trimmedSections != "" {
		resultParts = append(resultParts, trimmedSections)
	}
	return strings.Join(resultParts, "\n\n") + "\n"
}

func patchEnvFile(content string, assignments map[string]string) string {
	lines := make([]string, 0)
	if trimmed := strings.TrimSpace(content); trimmed != "" {
		lines = strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	}

	seen := map[string]bool{}
	for index, line := range lines {
		for key, value := range assignments {
			prefix := key + "="
			if strings.HasPrefix(strings.TrimSpace(line), prefix) {
				if strings.TrimSpace(value) == "" {
					lines[index] = ""
				} else {
					lines[index] = prefix + value
				}
				seen[key] = true
			}
		}
	}

	for key, value := range assignments {
		if seen[key] || strings.TrimSpace(value) == "" {
			continue
		}
		lines = append(lines, key+"="+value)
	}

	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n") + "\n"
}

func splitTOMLRootAndSections(content string) (string, string) {
	matcher := regexp.MustCompile(`(?m)^\[`)
	location := matcher.FindStringIndex(content)
	if location == nil {
		return content, ""
	}
	return content[:location[0]], content[location[0]:]
}

func replaceTOMLSection(sections string, sectionName string, replacement string) string {
	normalized := strings.ReplaceAll(sections, "\r\n", "\n")
	trimmedReplacement := strings.TrimSpace(replacement)
	if strings.TrimSpace(normalized) == "" {
		return trimmedReplacement
	}

	lines := strings.Split(normalized, "\n")
	header := "[" + sectionName + "]"
	start := -1
	end := len(lines)
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start == -1 {
			if trimmed == header {
				start = index
			}
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			end = index
			break
		}
	}

	if start == -1 {
		return strings.TrimSpace(normalized) + "\n\n" + trimmedReplacement
	}

	updated := make([]string, 0, len(lines)+(strings.Count(trimmedReplacement, "\n")+1))
	updated = append(updated, lines[:start]...)
	updated = append(updated, strings.Split(trimmedReplacement, "\n")...)
	updated = append(updated, lines[end:]...)
	return strings.TrimSpace(strings.Join(updated, "\n"))
}

func parseTOMLQuotedValue(content string, key string) string {
	pattern := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*=\s*"([^"]+)"\s*$`)
	match := pattern.FindStringSubmatch(content)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func parseTOMLSectionQuotedValue(sections string, sectionName string, key string) string {
	normalized := strings.ReplaceAll(sections, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	header := "[" + sectionName + "]"
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inSection {
			if trimmed == header {
				inSection = true
			}
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			break
		}
		if value := parseTOMLQuotedValue(line, key); value != "" {
			return value
		}
	}
	return ""
}

func parseEnvValue(content string, key string) string {
	pattern := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `=(.*)$`)
	match := pattern.FindStringSubmatch(content)
	if len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func parseJSONObject(data []byte) (map[string]any, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return map[string]any{}, nil
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return map[string]any{}, nil
	}
	return doc, nil
}

func jsonMap(parent map[string]any, key string) map[string]any {
	if existing, ok := parent[key].(map[string]any); ok && existing != nil {
		return existing
	}
	if existing, ok := parent[key].(map[string]interface{}); ok && existing != nil {
		out := make(map[string]any, len(existing))
		for childKey, childValue := range existing {
			out[childKey] = childValue
		}
		return out
	}
	out := map[string]any{}
	parent[key] = out
	return out
}

func jsonBool(doc map[string]any, key string) bool {
	value, ok := doc[key].(bool)
	return ok && value
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func marshalJSON(doc map[string]any) (string, error) {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

func ensureFileParent(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", path, err)
	}
	return nil
}

func createBackupIfNeeded(file FileInfo) error {
	if _, err := os.Stat(file.Path); os.IsNotExist(err) {
		return nil
	}
	backupPath := file.Path + ".cliro-go.bak"
	if _, err := os.Stat(backupPath); err == nil {
		return nil
	}
	data, err := os.ReadFile(file.Path)
	if err != nil {
		return fmt.Errorf("read backup source %s: %w", file.Path, err)
	}
	if err := writeFileAtomic(backupPath, data); err != nil {
		return err
	}
	return nil
}

func writeFileAtomic(path string, data []byte) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

func sameURL(left string, right string) bool {
	return strings.TrimRight(strings.TrimSpace(left), "/") == strings.TrimRight(strings.TrimSpace(right), "/")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func escapeTOMLString(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return replacer.Replace(value)
}
