package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// moduleRoot walks up from the working directory to the directory holding
// go.mod, so mk behaves the same whether it is invoked from the repo root or
// from a subdirectory.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if exists(filepath.Join(dir, "go.mod")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found: run this from inside the buildmax repository")
		}
		dir = parent
	}
}

// runCmd runs a command with our stdio attached, so build output streams live
// and interactive programs (the TUI, Vite's dev server) behave as if they had
// been started directly.
func runCmd(name string, args ...string) error {
	return runWith("", nil, name, args...)
}

// runIn runs a command with dir as its working directory.
func runIn(dir, name string, args ...string) error {
	return runWith(dir, nil, name, args...)
}

// runWith runs a command in dir with extra KEY=value entries appended to the
// current environment. An empty dir means the current directory.
func runWith(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", commandLine(name, args), err)
	}
	return nil
}

// runStdin feeds input to a command's stdin. It stands in for a shell pipe.
func runStdin(input, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", commandLine(name, args), err)
	}
	return nil
}

// capture returns a command's trimmed stdout. Its stderr is discarded, because
// every caller either has a fallback value or wraps the error itself.
func capture(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return strings.TrimSpace(string(out)), err
}

// succeeds reports whether a command exits zero, for the cases where the exit
// code is the answer (for example `git diff --quiet`).
func succeeds(name string, args ...string) bool {
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

func commandLine(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	return name + " " + strings.Join(args, " ")
}

// have reports whether a program is on PATH.
func have(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	// O_TRUNC rather than a remove-then-create, so replacing a binary that is
	// currently on PATH keeps the same inode permissions.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// OpenFile honours perm only when it creates the file, so set it explicitly
	// for the overwrite case.
	return os.Chmod(dst, perm)
}

func logf(tag, format string, args ...any) {
	fmt.Printf("[%s] %s\n", tag, fmt.Sprintf(format, args...))
}

func warnf(tag, format string, args ...any) {
	fmt.Printf("[%s] Warning: %s\n", tag, fmt.Sprintf(format, args...))
}
