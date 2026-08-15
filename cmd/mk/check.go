package main

import (
	"fmt"
	"strings"
)

func cmdCheck(args []string) error {
	scope := "all"
	if len(args) > 0 {
		scope = args[0]
	}
	if len(args) > 1 {
		return fmt.Errorf("usage: %s check [go|portal|desktop|docs|all]", mk())
	}

	checks := map[string]func() error{
		"go":      checkGo,
		"portal":  checkPortal,
		"desktop": checkDesktop,
		"docs":    checkDocs,
	}
	if scope == "all" {
		for _, name := range []string{"go", "portal", "desktop", "docs"} {
			if err := runCheck(name, checks[name]); err != nil {
				return err
			}
		}
		fmt.Println("[check] all scopes passed")
		return nil
	}
	check, ok := checks[scope]
	if !ok {
		return fmt.Errorf("unknown check scope %q; use go, portal, desktop, docs, or all", scope)
	}
	return runCheck(scope, check)
}

func runCheck(name string, check func() error) error {
	fmt.Printf("[check] %s\n", name)
	if err := check(); err != nil {
		return fmt.Errorf("check %s: %w", name, err)
	}
	fmt.Printf("[check] %s passed\n", name)
	return nil
}

func checkGo() error {
	files, err := capture("git", "ls-files", "*.go")
	if err != nil {
		return fmt.Errorf("list tracked Go files: %w", err)
	}
	if files != "" {
		args := append([]string{"-l"}, strings.Split(files, "\n")...)
		unformatted, err := capture("gofmt", args...)
		if err != nil {
			return fmt.Errorf("gofmt: %w", err)
		}
		if unformatted != "" {
			return fmt.Errorf("unformatted Go files:\n%s\nrun: git ls-files '*.go' | xargs gofmt -w", unformatted)
		}
	}
	if err := runCmd("go", "mod", "tidy", "-diff"); err != nil {
		return fmt.Errorf("go.mod is not tidy: %w", err)
	}
	if err := runCmd("go", "build", "./..."); err != nil {
		return err
	}
	if err := runCmd("go", "vet", "./..."); err != nil {
		return err
	}
	if err := cmdTest([]string{"race"}); err != nil {
		return err
	}
	return cmdLint()
}

func checkPortal() error {
	if err := buildGUI(); err != nil {
		return err
	}
	if err := ensureNPMDeps("portal"); err != nil {
		return err
	}
	for _, args := range [][]string{{"run", "lint"}, {"run", "build"}, {"test"}} {
		if err := runIn("portal", "npm", args...); err != nil {
			return err
		}
	}
	return nil
}

func checkDesktop() error {
	if err := buildGUI(); err != nil {
		return err
	}
	if err := ensureNPMDeps("desktop/frontend"); err != nil {
		return err
	}
	for _, args := range [][]string{{"run", "lint"}, {"run", "build"}, {"test"}} {
		if err := runIn("desktop/frontend", "npm", args...); err != nil {
			return err
		}
	}
	return nil
}

func checkDocs() error {
	if err := runCmd("go", "test", "./internal/architecture"); err != nil {
		return err
	}
	return runCmd("npm", "exec", "--yes", "--package=markdownlint-cli2@0.23.2", "--", "markdownlint-cli2")
}

func ensureNPMDeps(dir string) error {
	if isDir(dir + "/node_modules") {
		return nil
	}
	if err := runIn(dir, "npm", "ci"); err != nil {
		return fmt.Errorf("%s npm ci: %w", dir, err)
	}
	return nil
}
