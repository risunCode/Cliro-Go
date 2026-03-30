package logger

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type Entry struct {
	Timestamp int64  `json:"timestamp"`
	Level     string `json:"level"`
	Scope     string `json:"scope"`
	Message   string `json:"message"`
}

type ClearResult struct {
	MemoryCleared bool   `json:"memoryCleared"`
	FileCleared   bool   `json:"fileCleared"`
	PendingRetry  bool   `json:"pendingRetry"`
	Error         string `json:"error,omitempty"`
}

type Logger struct {
	mu               sync.RWMutex
	entries          []Entry
	max              int
	minLevelPriority int
	ctx              context.Context
	file             *os.File
	filePath         string
	fileSizeCapBytes int64
	pendingFileReset bool
	resetRetryActive bool
}

const (
	levelDebug = 10
	levelInfo  = 20
	levelWarn  = 30
	levelError = 40
)

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

func New(max int) *Logger {
	if max <= 0 {
		max = 500
	}
	return &Logger{
		max:              max,
		entries:          make([]Entry, 0, max),
		minLevelPriority: levelInfo,
	}
}

func (l *Logger) SetMaxEntries(max int) {
	if max <= 0 {
		max = 1
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.max = max
	if len(l.entries) > l.max {
		l.entries = append([]Entry(nil), l.entries[len(l.entries)-l.max:]...)
	}
}

func (l *Logger) SetVerbosity(level string) {
	priority := normalizeLevelPriority(level)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevelPriority = priority
}

func (l *Logger) SetFileSizeCapMB(megabytes int) {
	if megabytes <= 0 {
		megabytes = 1
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.fileSizeCapBytes = int64(megabytes) * 1024 * 1024
}

func (l *Logger) AttachContext(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ctx = ctx
}

func (l *Logger) SetFile(path string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	l.filePath = path

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}

	if persistedEntries, loadErr := loadPersistedEntries(path, l.max); loadErr == nil && len(persistedEntries) > 0 {
		if len(l.entries) == 0 {
			l.entries = persistedEntries
		} else {
			mergedEntries := append(append([]Entry(nil), persistedEntries...), l.entries...)
			if len(mergedEntries) > l.max {
				mergedEntries = mergedEntries[len(mergedEntries)-l.max:]
			}
			l.entries = mergedEntries
		}
	}

	l.file = file
	return nil
}

func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	_ = l.file.Sync()
	_ = l.file.Close()
	l.file = nil
}

func formatEntry(entry Entry) string {
	timestamp := time.UnixMilli(entry.Timestamp).Format("2006-01-02 15:04:05.000")
	message := strings.ReplaceAll(entry.Message, "\n", "\\n")
	return fmt.Sprintf("%s [%s] [%s] %s\n", timestamp, entry.Level, entry.Scope, message)
}

func parsePersistedLine(line string) (Entry, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Entry{}, false
	}

	const timestampLayout = "2006-01-02 15:04:05.000"
	if len(line) < len(timestampLayout)+1 {
		return Entry{}, false
	}

	timestampText := line[:len(timestampLayout)]
	parsedTime, err := time.ParseInLocation(timestampLayout, timestampText, time.Local)
	if err != nil {
		return Entry{}, false
	}

	remainder := strings.TrimSpace(line[len(timestampLayout):])
	if !strings.HasPrefix(remainder, "[") {
		return Entry{}, false
	}

	levelEnd := strings.Index(remainder, "]")
	if levelEnd <= 1 {
		return Entry{}, false
	}
	level := strings.TrimSpace(remainder[1:levelEnd])
	remainder = strings.TrimSpace(remainder[levelEnd+1:])

	if !strings.HasPrefix(remainder, "[") {
		return Entry{}, false
	}
	scopeEnd := strings.Index(remainder, "]")
	if scopeEnd <= 1 {
		return Entry{}, false
	}
	scope := strings.TrimSpace(remainder[1:scopeEnd])
	message := strings.TrimSpace(remainder[scopeEnd+1:])
	message = strings.ReplaceAll(message, "\\n", "\n")

	return Entry{
		Timestamp: parsedTime.UnixMilli(),
		Level:     level,
		Scope:     scope,
		Message:   message,
	}, true
}

func loadPersistedEntries(path string, limit int) ([]Entry, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}
	defer file.Close()

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	entries := make([]Entry, 0, limit)
	for scanner.Scan() {
		entry, ok := parsePersistedLine(scanner.Text())
		if !ok {
			continue
		}
		entries = append(entries, entry)
		if limit > 0 && len(entries) > limit {
			entries = append([]Entry(nil), entries[len(entries)-limit:]...)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func (l *Logger) append(level, scope, message string) {
	entry := Entry{Timestamp: time.Now().UnixMilli(), Level: level, Scope: scope, Message: message}
	line := formatEntry(entry)
	l.mu.Lock()
	if normalizeLevelPriority(level) < l.minLevelPriority {
		l.mu.Unlock()
		return
	}

	l.entries = append(l.entries, entry)
	if len(l.entries) > l.max {
		l.entries = append([]Entry(nil), l.entries[len(l.entries)-l.max:]...)
	}
	if l.file == nil && strings.TrimSpace(l.filePath) != "" {
		openFlags := os.O_APPEND | os.O_CREATE | os.O_WRONLY
		if l.pendingFileReset {
			openFlags |= os.O_TRUNC
		}
		if reopened, err := os.OpenFile(l.filePath, openFlags, 0o600); err == nil {
			l.file = reopened
			l.pendingFileReset = false
		}
	}
	if l.file != nil {
		if l.fileSizeCapBytes > 0 {
			if info, err := l.file.Stat(); err == nil && info.Size()+int64(len(line)) > l.fileSizeCapBytes {
				_ = l.resetPersistentFileLocked()
				l.schedulePendingFileResetLocked()
			}
		}

		if l.file != nil {
			_, _ = l.file.WriteString(line)
		}
	}
	ctx := l.ctx
	l.mu.Unlock()
	if ctx != nil {
		wruntime.EventsEmit(ctx, "log:entry", entry)
	}
}

func (l *Logger) Info(scope, message string)  { l.append("INFO", scope, message) }
func (l *Logger) Warn(scope, message string)  { l.append("WARN", scope, message) }
func (l *Logger) Error(scope, message string) { l.append("ERROR", scope, message) }

func (l *Logger) Entries(limit int) []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if limit <= 0 || limit > len(l.entries) {
		limit = len(l.entries)
	}
	start := len(l.entries) - limit
	if start < 0 {
		start = 0
	}
	out := append([]Entry(nil), l.entries[start:]...)
	return out
}

func (l *Logger) resetPersistentFileLocked() error {
	l.pendingFileReset = true

	path := strings.TrimSpace(l.filePath)
	if path == "" {
		l.pendingFileReset = false
		return nil
	}

	var resetErr error

	if l.file != nil {
		if err := l.file.Truncate(0); err == nil {
			if _, seekErr := l.file.Seek(0, io.SeekStart); seekErr == nil {
				l.pendingFileReset = false
				return nil
			} else {
				resetErr = fmt.Errorf("seek log file: %w", seekErr)
			}
		} else {
			resetErr = fmt.Errorf("truncate log file: %w", err)
		}
		_ = l.file.Close()
		l.file = nil
	}

	reopened, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND|os.O_TRUNC, 0o600)
	if err != nil {
		if resetErr != nil {
			return fmt.Errorf("%v; reopen log file: %w", resetErr, err)
		}
		return fmt.Errorf("reopen log file: %w", err)
	}
	l.file = reopened
	l.pendingFileReset = false
	return nil
}

func (l *Logger) schedulePendingFileResetLocked() {
	if !l.pendingFileReset || l.resetRetryActive {
		return
	}

	l.resetRetryActive = true

	go func() {
		for attempt := 0; attempt < 20; attempt++ {
			time.Sleep(250 * time.Millisecond)

			l.mu.Lock()
			if !l.pendingFileReset {
				l.resetRetryActive = false
				l.mu.Unlock()
				return
			}
			_ = l.resetPersistentFileLocked()
			if !l.pendingFileReset {
				l.resetRetryActive = false
				l.mu.Unlock()
				return
			}
			l.mu.Unlock()
		}

		l.mu.Lock()
		l.resetRetryActive = false
		l.mu.Unlock()
	}()
}

func (l *Logger) ClearPersistent() ClearResult {
	l.mu.Lock()
	defer l.mu.Unlock()

	result := ClearResult{MemoryCleared: true, FileCleared: true}
	l.entries = l.entries[:0]
	err := l.resetPersistentFileLocked()
	l.schedulePendingFileResetLocked()

	result.PendingRetry = l.pendingFileReset
	result.FileCleared = !l.pendingFileReset
	if err != nil {
		result.Error = err.Error()
	}

	if strings.TrimSpace(l.filePath) == "" {
		result.PendingRetry = false
		result.FileCleared = true
		result.Error = ""
	}

	return result
}

func (l *Logger) Clear() ClearResult {
	return l.ClearPersistent()
}
