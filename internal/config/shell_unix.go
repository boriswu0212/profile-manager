//go:build !windows

package config

import "os/exec"

func shellCommand(cmd string) *exec.Cmd {
	return exec.Command("sh", "-c", cmd)
}
