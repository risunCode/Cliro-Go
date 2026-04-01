//go:build windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

func isProtocolRegistered() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\kiro`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	// Check if URL Protocol value exists
	_, _, err = key.GetStringValue("URL Protocol")
	return err == nil
}

func registerProtocol() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks if any
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Create kiro key
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\kiro`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed to create kiro registry key: %w", err)
	}
	defer key.Close()

	// Set default value
	if err := key.SetStringValue("", "URL:Kiro Protocol"); err != nil {
		return fmt.Errorf("failed to set default value: %w", err)
	}

	// Set URL Protocol value
	if err := key.SetStringValue("URL Protocol", ""); err != nil {
		return fmt.Errorf("failed to set URL Protocol value: %w", err)
	}

	// Create DefaultIcon key
	iconKey, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\kiro\DefaultIcon`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed to create DefaultIcon key: %w", err)
	}
	defer iconKey.Close()

	iconValue := fmt.Sprintf(`"%s",0`, exePath)
	if err := iconKey.SetStringValue("", iconValue); err != nil {
		return fmt.Errorf("failed to set icon value: %w", err)
	}

	// Create shell\open\command key
	cmdKey, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\kiro\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed to create command key: %w", err)
	}
	defer cmdKey.Close()

	cmdValue := fmt.Sprintf(`"%s" "%%1"`, exePath)
	if err := cmdKey.SetStringValue("", cmdValue); err != nil {
		return fmt.Errorf("failed to set command value: %w", err)
	}

	return nil
}

func ensureProtocolRegistered() (bool, error) {
	if isProtocolRegistered() {
		return false, nil
	}

	if err := registerProtocol(); err != nil {
		return false, err
	}

	return true, nil
}
