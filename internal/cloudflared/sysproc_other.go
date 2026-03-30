//go:build !windows

package cloudflared

import "os/exec"

func configureCommand(cmd *exec.Cmd) {
	_ = cmd
}
