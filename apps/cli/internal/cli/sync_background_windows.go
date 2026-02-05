//go:build windows

package cli

import "syscall"

func syncRunActive(run syncRun) bool {
	if run.PID <= 0 {
		return false
	}
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(run.PID))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	var code uint32
	if err := syscall.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == syscall.STILL_ACTIVE
}
