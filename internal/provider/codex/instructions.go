package codex

import (
	_ "embed"
	"strings"
)

//go:embed default_instructions.md
var embeddedDefaultInstructions string

func defaultCodexInstructions() string {
	return strings.TrimSpace(embeddedDefaultInstructions)
}
