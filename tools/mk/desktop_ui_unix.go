//go:build !windows

package main

import (
	"os/exec"
	"syscall"
	"time"
)

// setProcessGroup puts the child in its own process group, so killProcessGroup
// can stop everything `wails dev` spawns rather than only the `go run` it
// starts as.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup asks the group to stop, then insists. `wails dev`'s own
// children (the compiled wails binary, the native app, Vite) do not always
// exit on their parent's death alone.
func killProcessGroup(cmd *exec.Cmd) {
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	time.Sleep(2 * time.Second)
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}
