//go:build windows

package main

import "os/exec"

// setProcessGroup is a no-op on Windows: os.Process.Kill already terminates
// the process this starts. Its own children (the native app, Vite) are not
// guaranteed to go with it — native-Windows CI is not covered yet, and this
// is the gap to close when it is. See AGENTS.md.
func setProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) {
	_ = cmd.Process.Kill()
}
