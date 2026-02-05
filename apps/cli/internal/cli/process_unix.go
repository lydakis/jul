//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

func setDetachedProcess(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
