package logger

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func (l *Logger) SetFile(path string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	l.filePath = strings.TrimSpace(path)
	if l.filePath == "" {
		return nil
	}

	file, err := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}

	if persistedEntries, loadErr := loadPersistedEntries(l.filePath, l.max); loadErr == nil && len(persistedEntries) > 0 {
		if len(l.entries) == 0 {
			l.entries = persistedEntries
		} else {
			mergedEntries := append(append([]Entry(nil), persistedEntries...), l.entries...)
			l.entries = mergedEntries
			l.trimEntriesLocked()
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

func loadPersistedEntries(path string, limit int) ([]Entry, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
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

func (l *Logger) writePersistentLineLocked(line string) {
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
	if l.file == nil {
		return
	}

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
