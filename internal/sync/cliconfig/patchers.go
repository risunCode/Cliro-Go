package cliconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

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


func escapeTOMLString(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return replacer.Replace(value)
}

func patchFile(app App, fileName string, content string, baseURL string, apiKey string, model string) (string, error) {
	switch app {
	case AppClaudeCode:
		return patchClaudeFile(fileName, content, baseURL, apiKey, model)
	case AppOpenCode, AppKiloCLI:
		return patchOpenCodeFile(content, baseURL, apiKey, model)
	case AppCodexAI:
		return patchCodexFile(fileName, content, baseURL, apiKey, model)
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
	jsonDoc["$schema"] = "https://opencode.ai/config.json"
	providers := jsonMap(jsonDoc, "provider")
	provider := map[string]any{
		"npm": "@ai-sdk/openai-compatible",
		"options": map[string]any{
			"baseURL": baseURL,
			"apiKey":  apiKey,
		},
		"models": map[string]any{},
	}
	if model != "" {
		provider["models"].(map[string]any)[model] = buildOpenCodeModelConfig(model)
	}
	providers["CLIRO"] = provider
	delete(providers, "cliro-go")
	delete(providers, "cliro")
	jsonDoc["provider"] = providers
	if model != "" {
		jsonDoc["model"] = model
	}
	permission := jsonMap(jsonDoc, "permission")
	permission["bash"] = "allow"
	jsonDoc["permission"] = permission
	return marshalJSON(jsonDoc)
}

func parseOpenCodeTopLevelModel(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if slash := strings.Index(trimmed, "/"); slash >= 0 && slash < len(trimmed)-1 {
		return strings.TrimSpace(trimmed[slash+1:])
	}
	return trimmed
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

func findOpenCodeProvider(jsonDoc map[string]any) (map[string]any, bool) {
	providers, ok := mapFromAny(jsonDoc["provider"])
	if !ok {
		return nil, false
	}
	for _, key := range []string{"CLIRO", "cliro-go", "cliro"} {
		if provider, ok := mapFromAny(providers[key]); ok {
			return provider, true
		}
	}
	for _, raw := range providers {
		if provider, ok := mapFromAny(raw); ok {
			return provider, true
		}
	}
	return nil, false
}

func firstModelKey(models map[string]any) string {
	keys := make([]string, 0, len(models))
	for key := range models {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		keys = append(keys, trimmed)
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	return keys[0]
}

func buildOpenCodeModelConfig(model string) map[string]any {
	contextLimit, outputLimit := openCodeModelLimits(model)
	return map[string]any{
		"name": openCodeModelName(model),
		"limit": map[string]any{
			"context": contextLimit,
			"output":  outputLimit,
		},
		"modalities": map[string]any{
			"input":  []string{"text", "image", "pdf"},
			"output": []string{"text"},
		},
		"reasoning": true,
	}
}

func openCodeModelLimits(model string) (int, int) {
	normalized := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(normalized, "gpt-5.4"):
		return 1000000, 64000
	case strings.HasPrefix(normalized, "gpt-"), strings.HasPrefix(normalized, "o1"), strings.HasPrefix(normalized, "o3"), strings.HasPrefix(normalized, "o4"):
		return 400000, 128000
	default:
		return 200000, 64000
	}
}

func openCodeModelName(model string) string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return "Custom Model"
	}
	normalized := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(normalized, "gpt-5.4"):
		return "ChatGPT 5.4"
	case strings.HasPrefix(normalized, "gpt-5.3"):
		return "ChatGPT Codex 5.3"
	case strings.HasPrefix(normalized, "gpt-5.2"):
		return "ChatGPT Codex 5.2"
	case strings.HasPrefix(normalized, "gpt-5.1"):
		return "ChatGPT Codex 5.1"
	case strings.HasPrefix(normalized, "claude-sonnet-4.5"):
		return "Claude Sonnet 4.5"
	case strings.HasPrefix(normalized, "claude-sonnet-4"):
		return "Claude Sonnet 4"
	case strings.HasPrefix(normalized, "claude-opus-4.5"):
		return "Claude Opus 4.5"
	case strings.HasPrefix(normalized, "claude-haiku-4.5"):
		return "Claude Haiku 4.5"
	default:
		return trimmed
	}
}

func mapFromAny(value any) (map[string]any, bool) {
	if existing, ok := value.(map[string]any); ok && existing != nil {
		return existing, true
	}
	if existing, ok := value.(map[string]interface{}); ok && existing != nil {
		out := make(map[string]any, len(existing))
		for childKey, childValue := range existing {
			out[childKey] = childValue
		}
		return out, true
	}
	return nil, false
}

func jsonMap(parent map[string]any, key string) map[string]any {
	if existing, ok := mapFromAny(parent[key]); ok {
		return existing
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
