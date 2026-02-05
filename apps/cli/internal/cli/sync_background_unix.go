//go:build !windows

package cli

import (
	"os"
	"syscall"
)

func syncRunActive(run syncRun) bool {
	if run.PID <= 0 {
		return false
	}
	proc, err := os.FindProcess(run.PID)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
