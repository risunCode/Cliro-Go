package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func StableThinkingSignature(thinking string) string {
	trimmed := strings.TrimSpace(thinking)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return "sig_" + hex.EncodeToString(sum[:])
}
