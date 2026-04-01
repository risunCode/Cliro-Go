package rules

import "strings"

// RemapToolCallArgs fixes argument names for known tool schemas.
func RemapToolCallArgs(name string, args map[string]any) map[string]any {
	if strings.EqualFold(strings.TrimSpace(name), "EnterPlanMode") {
		return map[string]any{}
	}

	remapped := cloneArgs(args)
	toolName := strings.ToLower(strings.TrimSpace(name))

	switch toolName {
	case "grep", "search", "search_code_definitions", "search_code_snippets":
		moveArg(remapped, "description", "pattern")
		moveArg(remapped, "query", "pattern")
		moveFirstPath(remapped, true)
	case "glob":
		moveArg(remapped, "description", "pattern")
		moveArg(remapped, "query", "pattern")
		moveFirstPath(remapped, true)
	case "read":
		moveArg(remapped, "path", "file_path")
	case "ls":
		if _, ok := remapped["path"]; !ok {
			remapped["path"] = "."
		}
	default:
		moveFirstPath(remapped, false)
	}

	return remapped
}

func cloneArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return map[string]any{}
	}

	cloned := make(map[string]any, len(args))
	for key, value := range args {
		cloned[key] = value
	}
	return cloned
}

func moveArg(args map[string]any, from string, to string) {
	if args == nil {
		return
	}
	value, ok := args[from]
	if !ok {
		return
	}
	delete(args, from)
	if _, exists := args[to]; !exists {
		args[to] = value
	}
}

func moveFirstPath(args map[string]any, removeSource bool) {
	if args == nil {
		return
	}
	if _, exists := args["path"]; exists {
		return
	}
	if path, ok := firstPathValue(args["paths"]); ok {
		args["path"] = path
		if removeSource {
			delete(args, "paths")
		}
	}
}

func firstPathValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed, trimmed != ""
	case []any:
		if len(typed) != 1 {
			return "", false
		}
		path, ok := typed[0].(string)
		if !ok {
			return "", false
		}
		trimmed := strings.TrimSpace(path)
		return trimmed, trimmed != ""
	default:
		return "", false
	}
}
