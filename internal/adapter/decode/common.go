package decode

import (
	"fmt"
	"strings"

	"cliro-go/internal/adapter/ir"
)

func roleFromString(value string) ir.Role {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "system":
		return ir.RoleSystem
	case "developer":
		return ir.RoleDeveloper
	case "assistant":
		return ir.RoleAssistant
	case "tool":
		return ir.RoleTool
	default:
		return ir.RoleUser
	}
}

func validateModel(model string) error {
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("model is required")
	}
	return nil
}
