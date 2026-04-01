package platform

import (
	"fmt"
	"runtime"
)

const opencodeVersion = "1.2.27"

// BuildOpencodeUserAgent mocks opencode user-agent format
// Format: opencode/{version} ({os} {osVersion}; {arch})
// Examples:
//   - Windows: opencode/1.2.27 (windows 10.0.26100; amd64)
//   - Mac Intel: opencode/1.2.27 (darwin 23.0.0; amd64)
//   - Mac ARM: opencode/1.2.27 (darwin 23.0.0; arm64)
//   - Linux: opencode/1.2.27 (linux 6.5.0; amd64)
func BuildOpencodeUserAgent() string {
	osVersion := getOSVersion()
	if osVersion != "" {
		return fmt.Sprintf("opencode/%s (%s %s; %s)",
			opencodeVersion,
			runtime.GOOS,
			osVersion,
			runtime.GOARCH)
	}

	// Fallback if version detection fails
	return fmt.Sprintf("opencode/%s (%s; %s)",
		opencodeVersion,
		runtime.GOOS,
		runtime.GOARCH)
}
