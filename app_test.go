package main

import (
	"os"
	"path/filepath"
	"testing"

	"cliro-go/internal/account"
	"cliro-go/internal/config"
	"github.com/wailsapp/wails/v2/pkg/options"
)

func TestBuildSecondLaunchNotice(t *testing.T) {
	notice := buildSecondLaunchNotice(options.SecondInstanceData{
		Args:             []string{"--foo", "bar"},
		WorkingDirectory: `C:\Users\AceLova`,
	})

	if notice.Message == "" {
		t.Fatalf("expected non-empty message")
	}
	if notice.WorkingDirectory != `C:\Users\AceLova` {
		t.Fatalf("working directory = %q", notice.WorkingDirectory)
	}
	if len(notice.Args) != 2 {
		t.Fatalf("args length = %d, want 2", len(notice.Args))
	}
	if notice.ReceivedAt == 0 {
		t.Fatalf("expected received timestamp")
	}
}

func TestGetStateIncludesStartupWarnings(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "accounts.json"), []byte(`{"accounts":[{"id":"legacy"}]}`), 0o600); err != nil {
		t.Fatalf("write legacy accounts: %v", err)
	}

	store, err := config.NewManager(dataDir)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	app := &App{store: store, pool: account.NewPool(store)}
	state := app.GetState()
	if len(state.StartupWarnings) == 0 {
		t.Fatalf("expected startup warnings in app state")
	}
}
