package cliconfig

import (
	"path/filepath"
	"strings"
	"time"
)

type App string

const (
	AppClaudeCode App = "claude-code"
	AppOpenCode   App = "opencode-cli"
	AppKiloCLI    App = "kilo-cli"
	AppCodexAI    App = "codex-ai"
)

type FileInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type Status struct {
	ID             string     `json:"id"`
	Label          string     `json:"label"`
	Installed      bool       `json:"installed"`
	InstallPath    string     `json:"installPath,omitempty"`
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

type CatalogModel struct {
	ID      string `json:"id"`
	OwnedBy string `json:"ownedBy"`
}

type installProbeCache struct {
	Installed bool
	Path      string
	Version   string
	CheckedAt time.Time
}

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
		id:      AppKiloCLI,
		label:   "Kilo CLI Config",
		command: "kilo",
		files: func(home string) []FileInfo {
			return []FileInfo{{Name: "opencode.json", Path: filepath.Join(home, ".config", "kilo", "opencode.json")}}
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
	if app == AppCodexAI || app == AppOpenCode || app == AppKiloCLI || app == AppClaudeCode {
		if strings.HasSuffix(trimmed, "/v1") {
			return trimmed
		}
		return trimmed + "/v1"
	}
	return strings.TrimSuffix(trimmed, "/v1")
}
