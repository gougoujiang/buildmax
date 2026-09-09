package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// desktopBuiltBinary returns the launchable executable `build desktop` produced,
// looking in the same places copyDesktopBinary copies from: the macOS .app
// bundle nests it, every other platform leaves it directly under build/bin.
func desktopBuiltBinary() (string, bool) {
	candidates := []string{filepath.Join(desktopDir, "build", "bin", exe(desktopBinary))}
	if runtime.GOOS == "darwin" {
		bundled := filepath.Join(desktopDir, "build", "bin", "BuildMax.app", "Contents", "MacOS", desktopBinary)
		candidates = append([]string{bundled}, candidates...)
	}
	for _, path := range candidates {
		if exists(path) {
			return path, true
		}
	}
	return "", false
}

// desktopLaunchDwell is how long the packaged app must stay up to pass. Long
// enough to clear the window and runtime init where a crash-on-launch shows,
// short enough to keep the check cheap.
const desktopLaunchDwell = 8 * time.Second

// e2eDesktopLaunch smoke-tests the packaged app by launching it and requiring it
// to stay up. `build desktop` and desktop-package.yml prove the app *builds*;
// this proves the built bundle *starts* -- catching a crash on launch (a missing
// runtime, a broken bundle, a bad asset embed) that a build never sees. It does
// not drive the window: surviving a short dwell is the whole assertion, which is
// all a CI runner without a real user session can promise.
func e2eDesktopLaunch() error {
	bin, ok := desktopBuiltBinary()
	if !ok {
		return fmt.Errorf("no desktop build to launch under %s; run `%s build desktop` first",
			filepath.Join(desktopDir, "build", "bin"), mk())
	}
	fmt.Printf("[e2e] Desktop launch smoke: starting %s and requiring it to stay up %s\n", bin, desktopLaunchDwell)

	home, err := os.MkdirTemp("", "buildmax-e2e-desktop-launch-")
	if err != nil {
		return fmt.Errorf("create an isolated BUILDMAX_HOME: %w", err)
	}
	defer os.RemoveAll(home)

	logPath := filepath.Join(os.TempDir(), "buildmax-desktop-launch.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create the launch log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "BUILDMAX_HOME="+home)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start the desktop app: %w", err)
	}
	fmt.Printf("[e2e] desktop app started (pid %d, BUILDMAX_HOME=%s, log %s)\n", cmd.Process.Pid, home, logPath)

	// A crash on launch makes Wait return before the dwell elapses; a healthy
	// app keeps running until the timer stops it.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case waitErr := <-done:
		return fmt.Errorf("the desktop app exited within %s (%v); see %s\n%s",
			desktopLaunchDwell, waitErr, logPath, lastLines(logPath, 20))
	case <-time.After(desktopLaunchDwell):
		killProcessGroup(cmd)
		<-done
		fmt.Printf("[e2e] desktop app stayed up for %s and was stopped: launch smoke passed\n", desktopLaunchDwell)
		return nil
	}
}

func lastLines(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
