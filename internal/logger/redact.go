package logger

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type redactionPattern struct {
	re   *regexp.Regexp
	repl string
}

var redactionPatterns = []redactionPattern{
	{re: regexp.MustCompile(`(?i)(authorization\s*[:=]\s*bearer\s+)([^\s",;]+)`), repl: `${1}[REDACTED]`},
	{re: regexp.MustCompile(`(?i)(bearer\s+)([A-Za-z0-9._~+/=-]+)`), repl: `${1}[REDACTED]`},
	{re: regexp.MustCompile(`(?i)((?:access[_-]?token|refresh[_-]?token|id[_-]?token|api[_-]?key|proxy[_-]?api[_-]?key|client[_-]?secret|authorization)\s*[:=]\s*")([^"]*)(")`), repl: `${1}[REDACTED]${3}`},
	{re: regexp.MustCompile(`(?i)((?:access[_-]?token|refresh[_-]?token|id[_-]?token|api[_-]?key|proxy[_-]?api[_-]?key|client[_-]?secret|authorization)\s*[:=]\s*)([^\s,;]+)`), repl: `${1}[REDACTED]`},
	{re: regexp.MustCompile(`(?i)("(?:access[_-]?token|refresh[_-]?token|id[_-]?token|api[_-]?key|proxy[_-]?api[_-]?key|client[_-]?secret|authorization)"\s*:\s*")([^"]*)(")`), repl: `${1}[REDACTED]${3}`},
}

var sensitiveFieldNamePattern = regexp.MustCompile(`(?i)(^|[_-])(access[_-]?token|refresh[_-]?token|id[_-]?token|client[_-]?secret|authorization|api[_-]?key|proxy[_-]?api[_-]?key|password|cookie)($|[_-])`)

func redactMessage(message string) string {
	redacted := message
	for _, pattern := range redactionPatterns {
		redacted = pattern.re.ReplaceAllString(redacted, pattern.repl)
	}
	return redacted
}

func redactFieldValue(key string, value any) any {
	if sensitiveFieldNamePattern.MatchString(strings.TrimSpace(key)) {
		return "[REDACTED]"
	}
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return redactMessage(typed)
	case error:
		return redactMessage(typed.Error())
	case fmt.Stringer:
		return redactMessage(typed.String())
	case []byte:
		return redactMessage(string(typed))
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return value
		}
		redacted := redactMessage(string(encoded))
		if redacted == string(encoded) {
			return value
		}
		var out any
		if err := json.Unmarshal([]byte(redacted), &out); err == nil {
			return out
		}
		return redacted
	}
}
