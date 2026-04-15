package logger

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Entry struct {
	Timestamp int64          `json:"timestamp"`
	Level     string         `json:"level"`
	Scope     string         `json:"scope"`
	Event     string         `json:"event"`
	RequestID string         `json:"requestId,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

type Field struct {
	Key   string
	Value any
}

type ClearResult struct {
	MemoryCleared bool   `json:"memoryCleared"`
	FileCleared   bool   `json:"fileCleared"`
	PendingRetry  bool   `json:"pendingRetry"`
	Error         string `json:"error,omitempty"`
}

const (
	levelDebug = 10
	levelInfo  = 20
	levelWarn  = 30
	levelError = 40
)

func F(key string, value any) Field { return Field{Key: key, Value: value} }

func Err(err error) Field {
	if err == nil {
		return Field{Key: "error", Value: ""}
	}
	return Field{Key: "error", Value: err.Error()}
}

func normalizeLevelPriority(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return levelDebug
	case "warn", "warning":
		return levelWarn
	case "error":
		return levelError
	default:
		return levelInfo
	}
}

func normalizeLevel(level string) string {
	switch normalizeLevelPriority(level) {
	case levelDebug:
		return "DEBUG"
	case levelWarn:
		return "WARN"
	case levelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

func normalizeScope(scope string) string {
	trimmed := strings.TrimSpace(scope)
	if trimmed == "" {
		return "system"
	}
	return trimmed
}

func newEntry(level, scope, event string, fields ...Field) Entry {
	entryFields, requestID := buildEntryFields(fields)
	return Entry{
		Timestamp: time.Now().UnixMilli(),
		Level:     normalizeLevel(level),
		Scope:     normalizeScope(scope),
		Event:     strings.TrimSpace(event),
		RequestID: requestID,
		Fields:    entryFields,
	}
}

func buildEntryFields(fields []Field) (map[string]any, string) {
	if len(fields) == 0 {
		return nil, ""
	}

	entryFields := make(map[string]any, len(fields))
	requestID := ""
	for _, field := range fields {
		key := normalizeFieldKey(field.Key)
		if key == "" {
			continue
		}
		value := redactFieldValue(key, field.Value)
		if isRequestIDKey(key) {
			requestID = strings.TrimSpace(fieldStringValue(value))
			if requestID == "" {
				continue
			}
			continue
		}
		entryFields[key] = value
	}
	if len(entryFields) == 0 {
		entryFields = nil
	}
	return entryFields, requestID
}

func normalizeFieldKey(key string) string {
	return strings.TrimSpace(key)
}

func isRequestIDKey(key string) bool {
	return strings.EqualFold(strings.TrimSpace(key), "request_id") || strings.EqualFold(strings.TrimSpace(key), "requestId")
}

func fieldStringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case error:
		return typed.Error()
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case int8:
		return strconv.FormatInt(int64(typed), 10)
	case int16:
		return strconv.FormatInt(int64(typed), 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint8:
		return strconv.FormatUint(uint64(typed), 10)
	case uint16:
		return strconv.FormatUint(uint64(typed), 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(encoded)
	}
}

func marshalEntryLine(entry Entry) string {
	encoded, err := json.Marshal(entry)
	if err != nil {
		fallback := fmt.Sprintf(`{"timestamp":%d,"level":%q,"scope":%q,"event":%q}`, entry.Timestamp, entry.Level, entry.Scope, entry.Event)
		return fallback + "\n"
	}
	return string(encoded) + "\n"
}

func parsePersistedLine(line string) (Entry, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Entry{}, false
	}

	var entry Entry
	if err := json.Unmarshal([]byte(line), &entry); err != nil || entry.Timestamp <= 0 {
		return Entry{}, false
	}
	return normalizePersistedEntry(entry), true
}

func normalizePersistedEntry(entry Entry) Entry {
	entry.Level = normalizeLevel(entry.Level)
	entry.Scope = normalizeScope(entry.Scope)
	entry.Event = strings.TrimSpace(entry.Event)
	entry.RequestID = strings.TrimSpace(entry.RequestID)
	if len(entry.Fields) > 0 {
		normalized := make(map[string]any, len(entry.Fields))
		for key, value := range entry.Fields {
			normalized[normalizeFieldKey(key)] = redactFieldValue(key, value)
		}
		entry.Fields = normalized
	}
	return entry
}
