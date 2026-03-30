//go:build !windows

package clisync

import "os/exec"

func configureCommand(cmd *exec.Cmd) {
	_ = cmd
}
