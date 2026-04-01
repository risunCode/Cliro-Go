//go:build !windows

package cliconfig

import "os/exec"

func configureCommand(cmd *exec.Cmd) {
	_ = cmd
}
