package logger

import (
	"context"
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

type Logger struct {
	mu      sync.RWMutex
	entries []Entry
	max     int
	ctx     context.Context
}

func New(max int) *Logger {
	if max <= 0 {
		max = 500
	}
	return &Logger{max: max, entries: make([]Entry, 0, max)}
}

func (l *Logger) AttachContext(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ctx = ctx
}

func (l *Logger) append(level, scope, message string) {
	entry := Entry{Timestamp: time.Now().UnixMilli(), Level: level, Scope: scope, Message: message}
	l.mu.Lock()
	l.entries = append(l.entries, entry)
	if len(l.entries) > l.max {
		l.entries = append([]Entry(nil), l.entries[len(l.entries)-l.max:]...)
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

func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = l.entries[:0]
}
