package logger

import (
	"context"
	"os"
	"sync"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

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
	l.trimEntriesLocked()
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

const logEntryEventName = "log:entry"

func (l *Logger) AttachContext(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ctx = ctx
}

func (l *Logger) append(level, scope, event string, fields ...Field) {
	l.appendEntry(newEntry(level, scope, event, fields...))
}

func (l *Logger) appendEntry(entry Entry) {
	line := marshalEntryLine(entry)

	l.mu.Lock()
	if normalizeLevelPriority(entry.Level) < l.minLevelPriority {
		l.mu.Unlock()
		return
	}
	l.appendMemoryLocked(entry)
	l.writePersistentLineLocked(line)
	ctx := l.ctx
	l.mu.Unlock()

	emitEntry(ctx, entry)
}

func emitEntry(ctx context.Context, entry Entry) {
	if ctx == nil {
		return
	}
	wruntime.EventsEmit(ctx, logEntryEventName, entry)
}

func (l *Logger) appendMemoryLocked(entry Entry) {
	l.entries = append(l.entries, entry)
	l.trimEntriesLocked()
}

func (l *Logger) trimEntriesLocked() {
	if l.max <= 0 {
		l.max = 1
	}
	if len(l.entries) <= l.max {
		return
	}
	l.entries = append([]Entry(nil), l.entries[len(l.entries)-l.max:]...)
}

func (l *Logger) Debug(scope, event string, fields ...Field) {
	l.append("DEBUG", scope, event, fields...)
}
func (l *Logger) Info(scope, event string, fields ...Field) {
	l.append("INFO", scope, event, fields...)
}
func (l *Logger) Warn(scope, event string, fields ...Field) {
	l.append("WARN", scope, event, fields...)
}
func (l *Logger) Error(scope, event string, fields ...Field) {
	l.append("ERROR", scope, event, fields...)
}

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
	return append([]Entry(nil), l.entries[start:]...)
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

	if l.filePath == "" {
		result.PendingRetry = false
		result.FileCleared = true
		result.Error = ""
	}

	return result
}

func (l *Logger) Clear() ClearResult {
	return l.ClearPersistent()
}
