package main

import (
	"fmt"
	"os"
	"strings"
)

func cmdDoctor(args []string) error {
	all := false
	if len(args) > 1 || (len(args) == 1 && args[0] != "all") {
		return fmt.Errorf("usage: %s doctor [all]", mk())
	}
	if len(args) == 1 {
		all = true
	}
	failures := 0
	fmt.Println("BuildMax contributor doctor")
	fmt.Println("===========================")

	goWant := "go" + fileValue("go.mod", "go ")
	nodeWant := "v" + strings.TrimSpace(readText(".node-version"))
	npmWant := strings.TrimPrefix(fileValue("portal/package.json", `"packageManager": "npm@`), "")
	npmWant = strings.TrimSuffix(npmWant, `",`)
	wailsWant := "v" + fileValue("go.mod", "github.com/wailsapp/wails/v2 v")

	failures += requiredVersion("Go", "go", []string{"version"}, goWant)
	failures += requiredVersion("git", "git", []string{"--version"}, "git version")
	if all {
		failures += requiredExactVersion("Node", "node", []string{"--version"}, nodeWant)
		failures += requiredExactVersion("npm", "npm", []string{"--version"}, npmWant)
	} else {
		optionalExactVersion("Node", "node", []string{"--version"}, nodeWant, "needed for gui, Portal, and Desktop")
		optionalExactVersion("npm", "npm", []string{"--version"}, npmWant, "needed for gui, Portal, and Desktop")
	}
	optionalVersion("Wails CLI", "wails", []string{"version"}, wailsWant, "optional; ./make build runs the pinned version through Go")
	optionalPresence("Docker", "docker", "needed only for Compose/container work")
	optionalPresence("kind", "kind", "needed only for local Kubernetes work")
	optionalPresence("kubectl", "kubectl", "needed only for Kubernetes work")
	// actionlint runs its shell script pass only when shellcheck is on PATH, and
	// says nothing when it is not, so a `run:` block can pass ./make check ci and
	// still fail on the runner. Reporting it here is the only warning available.
	optionalPresence("shellcheck", "shellcheck", "actionlint's shell script pass in ./make check ci needs it")

	status, err := capture("git", "status", "--porcelain")
	if err != nil {
		fmt.Printf("[FAIL] workspace: %v\n", err)
		failures++
	} else if status == "" {
		fmt.Println("[OK]   workspace: clean")
	} else {
		fmt.Println("[WARN] workspace: has local changes (preserve them; do not reset another contributor's work)")
	}

	fmt.Println()
	fmt.Println("Safe default commands: ./make build cli, ./make test, ./make lint, ./make check <scope>")
	fmt.Println("External-effect commands: install, release, compose, kind, and publication workflows")
	if failures > 0 {
		return fmt.Errorf("contributor doctor found %d required toolchain problem(s)", failures)
	}
	if all {
		fmt.Println("Summary: full contributor toolchain is ready")
	} else {
		fmt.Println("Summary: core contributor toolchain is ready; use 'doctor all' for frontend requirements")
	}
	return nil
}

func optionalExactVersion(label, command string, args []string, want, note string) {
	version, err := capture(command, args...)
	if err != nil {
		fmt.Printf("[INFO] %s: not installed (%s)\n", label, note)
		return
	}
	if version != want {
		fmt.Printf("[WARN] %s: got %s; project uses exactly %s (%s)\n", label, oneLine(version), want, note)
		return
	}
	fmt.Printf("[OK]   %s: %s\n", label, oneLine(version))
}

func requiredExactVersion(label, command string, args []string, want string) int {
	version, err := capture(command, args...)
	if err != nil {
		fmt.Printf("[FAIL] %s: %s not found\n", label, command)
		return 1
	}
	if version != want {
		fmt.Printf("[FAIL] %s: got %s; want exactly %s\n", label, oneLine(version), want)
		return 1
	}
	fmt.Printf("[OK]   %s: %s\n", label, oneLine(version))
	return 0
}

func requiredVersion(label, command string, args []string, want string) int {
	version, err := capture(command, args...)
	if err != nil {
		fmt.Printf("[FAIL] %s: %s not found\n", label, command)
		return 1
	}
	if !strings.Contains(version, want) {
		fmt.Printf("[FAIL] %s: got %s; want %s\n", label, oneLine(version), want)
		return 1
	}
	fmt.Printf("[OK]   %s: %s\n", label, oneLine(version))
	return 0
}

func optionalVersion(label, command string, args []string, want, note string) {
	version, err := capture(command, args...)
	if err != nil {
		fmt.Printf("[INFO] %s: not installed (%s)\n", label, note)
		return
	}
	if !strings.Contains(version, want) {
		fmt.Printf("[WARN] %s: got %s; project uses %s (%s)\n", label, oneLine(version), want, note)
		return
	}
	fmt.Printf("[OK]   %s: %s\n", label, oneLine(version))
}

func optionalPresence(label, command, note string) {
	if have(command) {
		fmt.Printf("[OK]   %s: installed (%s)\n", label, note)
		return
	}
	fmt.Printf("[INFO] %s: not installed (%s)\n", label, note)
}

func oneLine(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	if len(value) > 120 {
		return value[:117] + "..."
	}
	return value
}

func readText(path string) string {
	body, _ := os.ReadFile(path)
	return string(body)
}

func fileValue(path, prefix string) string {
	for _, line := range strings.Split(readText(path), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return "unknown"
}
