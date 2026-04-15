package models

import (
	"errors"
	"fmt"
	"strings"
)

type ProviderCapability struct {
	SupportsTools    bool
	SupportsThinking bool
	SupportsStream   bool
}

var capabilityByProvider = map[string]ProviderCapability{
	"codex": {SupportsTools: true, SupportsThinking: true, SupportsStream: true},
	"kiro":  {SupportsTools: true, SupportsThinking: true, SupportsStream: true},
}

var (
	ErrUnsupportedProvider = errors.New("unsupported provider")
	ErrUnsupportedTools    = errors.New("tools are not supported by provider")
	ErrUnsupportedThinking = errors.New("thinking is not supported by provider")
	ErrUnsupportedStream   = errors.New("stream is not supported by provider")
)

func CapabilityForProvider(provider string) (ProviderCapability, bool) {
	capability, ok := capabilityByProvider[provider]
	return capability, ok
}

func ValidateRequest(request Request, provider string) error {
	capability, ok := CapabilityForProvider(provider)
	if !ok {
		return ErrUnsupportedProvider
	}
	if err := ValidateToolRules(request, capability); err != nil {
		return err
	}
	if err := ValidateThinkingRules(request, capability); err != nil {
		return err
	}
	if request.Stream && !capability.SupportsStream {
		return ErrUnsupportedStream
	}
	if err := ValidateMessageRules(request); err != nil {
		return err
	}
	return nil
}

func ValidateToolRules(request Request, capability ProviderCapability) error {
	if len(request.Tools) > 0 && !capability.SupportsTools {
		return ErrUnsupportedTools
	}
	return nil
}

func ValidateThinkingRules(request Request, capability ProviderCapability) error {
	hasThinking := false
	for _, message := range request.Messages {
		if len(message.ThinkingBlocks) > 0 || len(ContentThinkingBlocks(message.Content)) > 0 {
			hasThinking = true
			break
		}
		if message.Role == RoleAssistant {
			continue
		}
		if _, ok := message.Content.(map[string]any); ok {
			hasThinking = true
			break
		}
	}
	if hasThinking && !capability.SupportsThinking {
		return ErrUnsupportedThinking
	}
	return nil
}

func ValidateMessageRules(request Request) error {
	declaredTools := make(map[string]struct{})
	seenToolResults := make(map[string]struct{})
	for _, message := range request.Messages {
		if err := ValidateMessageContent(message); err != nil {
			return err
		}
		if message.Role == RoleAssistant {
			for _, toolCall := range message.ToolCalls {
				if strings.TrimSpace(toolCall.ID) == "" {
					return fmt.Errorf("assistant tool call id is required")
				}
				if strings.TrimSpace(toolCall.Name) == "" {
					return fmt.Errorf("assistant tool call name is required")
				}
				declaredTools[strings.TrimSpace(toolCall.ID)] = struct{}{}
			}
		}
		for _, result := range ContentToolResults(message.Content) {
			id := strings.TrimSpace(result.ToolCallID)
			if id == "" {
				return fmt.Errorf("tool_result block requires tool_call_id")
			}
			if _, ok := seenToolResults[id]; ok {
				continue
			}
			seenToolResults[id] = struct{}{}
			if message.Role == RoleAssistant {
				return fmt.Errorf("assistant message cannot contain tool_result blocks")
			}
			if _, ok := declaredTools[id]; !ok && message.Role == RoleTool {
				return fmt.Errorf("tool result references undeclared tool_call_id: %s", id)
			}
		}
		if message.Role == RoleTool && len(message.ToolCalls) > 0 {
			return fmt.Errorf("tool role cannot declare tool calls")
		}
	}
	return nil
}

func ValidateMessageContent(message Message) error {
	for _, image := range ContentImages(message.Content) {
		if strings.TrimSpace(image.Data) == "" && strings.TrimSpace(image.URL) == "" {
			return fmt.Errorf("image block requires data or url")
		}
	}
	return nil
}

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
	for k, v := range args {
		cloned[k] = v
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
