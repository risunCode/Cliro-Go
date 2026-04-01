package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoggerRedactsSecrets(t *testing.T) {
	l := New(10)
	l.Info("auth", `Authorization: Bearer abc123 accessToken="token-1" refresh_token=token-2 client_secret:"token-3"`)

	entries := l.Entries(1)
	if len(entries) != 1 {
		t.Fatalf("entries len = %d", len(entries))
	}
	message := entries[0].Message
	if strings.Contains(message, "abc123") || strings.Contains(message, "token-1") || strings.Contains(message, "token-2") || strings.Contains(message, "token-3") {
		t.Fatalf("expected redacted message, got %q", message)
	}
	if strings.Count(message, "[REDACTED]") < 3 {
		t.Fatalf("expected multiple redactions, got %q", message)
	}
}

func TestLoggerSetFileLoadsPersistedEntriesAndAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	seed := marshalEntryLine(Entry{Timestamp: time.Now().UnixMilli(), Level: "INFO", Scope: "app", Message: "booted"})
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	l := New(10)
	if err := l.SetFile(path); err != nil {
		t.Fatalf("SetFile: %v", err)
	}
	defer l.Close()

	l.Info("app", "ready")

	entries := l.Entries(10)
	if len(entries) != 2 {
		t.Fatalf("entries len = %d", len(entries))
	}
	if entries[0].Message != "booted" || entries[1].Message != "ready" {
		t.Fatalf("unexpected entries: %#v", entries)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "booted") || !strings.Contains(body, "ready") {
		t.Fatalf("unexpected file contents: %s", body)
	}
}

func TestLoggerClearPersistentClearsMemoryAndFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	l := New(10)
	if err := l.SetFile(path); err != nil {
		t.Fatalf("SetFile: %v", err)
	}
	defer l.Close()

	l.Info("app", "hello")
	result := l.ClearPersistent()
	if !result.MemoryCleared {
		t.Fatalf("expected memory cleared: %#v", result)
	}
	if len(l.Entries(10)) != 0 {
		t.Fatalf("expected no memory entries after clear")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if strings.TrimSpace(string(data)) != "" {
		t.Fatalf("expected empty file after clear, got %q", string(data))
	}
}

func TestLoggerInfoEventPersistsStructuredJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	l := New(10)
	if err := l.SetFile(path); err != nil {
		t.Fatalf("SetFile: %v", err)
	}
	defer l.Close()

	l.InfoEvent("proxy", "request.completed", String("request_id", "req-1"), String("route", "openai_chat"), Int("status", 200), Bool("stream", false), String("access_token", "secret-value"))

	entries := l.Entries(1)
	if len(entries) != 1 {
		t.Fatalf("entries len = %d", len(entries))
	}
	entry := entries[0]
	if entry.Event != "request.completed" {
		t.Fatalf("event = %q", entry.Event)
	}
	if entry.RequestID != "req-1" {
		t.Fatalf("request id = %q", entry.RequestID)
	}
	if entry.Fields["route"] != "openai_chat" {
		t.Fatalf("route field = %#v", entry.Fields["route"])
	}
	if entry.Fields["status"] != 200 {
		t.Fatalf("status field = %#v", entry.Fields["status"])
	}
	if entry.Fields["access_token"] != "[REDACTED]" {
		t.Fatalf("access_token field = %#v", entry.Fields["access_token"])
	}
	if strings.Contains(entry.Message, "secret-value") {
		t.Fatalf("message leaked secret: %q", entry.Message)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one jsonl line, got %d", len(lines))
	}
	var persisted Entry
	if err := json.Unmarshal([]byte(lines[0]), &persisted); err != nil {
		t.Fatalf("unmarshal persisted entry: %v", err)
	}
	if persisted.Event != "request.completed" || persisted.RequestID != "req-1" {
		t.Fatalf("unexpected persisted entry: %#v", persisted)
	}
}
