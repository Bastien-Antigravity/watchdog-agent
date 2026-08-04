//go:build !windows

package utils

import (
	"os/exec"
	"syscall"
)

// SetSysProcAttrGroup sets the process group attributes for Unix-like systems
func SetSysProcAttrGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// KillProcessGroup forcefully terminates a process and all of its subprocesses (process group)
func KillProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	} else {
		_ = cmd.Process.Kill()
	}
}

// KillProcessGroupByID forcefully terminates a process group using its PID
func KillProcessGroupByID(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
