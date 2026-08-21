package main

import (
	"fmt"
	"os"
	"strconv"
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

	goWant := fileValue("go.mod", "go ")
	nodeWant := "v" + strings.TrimSpace(readText(".node-version"))
	npmWant := strings.TrimPrefix(fileValue("portal/package.json", `"packageManager": "npm@`), "")
	npmWant = strings.TrimSuffix(npmWant, `",`)
	wailsWant := "v" + fileValue("go.mod", "github.com/wailsapp/wails/v2 v")

	failures += requiredGoVersion(goWant)
	failures += requiredPresence("git", "git", []string{"--version"})
	if all {
		failures += requiredExactVersion("Node", "node", []string{"--version"}, nodeWant)
		failures += requiredExactVersion("npm", "npm", []string{"--version"}, npmWant)
	} else {
		optionalExactVersion("Node", "node", []string{"--version"}, nodeWant, "needed for gui, Portal, and Desktop")
		optionalExactVersion("npm", "npm", []string{"--version"}, npmWant, "needed for gui, Portal, and Desktop")
	}
	optionalVersion("Wails CLI", "wails", []string{"version"}, wailsWant, "optional; ./make build runs the pinned version through Go")
	optionalPresence("Docker", "docker", "needed only for Compose/container work")
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

// requiredPresence reports a tool that has to exist but has no version floor
// worth enforcing. git is the only one: every version in circulation can run
// what this repository asks of it.
func requiredPresence(label, command string, args []string) int {
	version, err := capture(command, args...)
	if err != nil {
		fmt.Printf("[FAIL] %s: %s not found\n", label, command)
		return 1
	}
	fmt.Printf("[OK]   %s: %s\n", label, oneLine(version))
	return 0
}

// requiredGoVersion compares the installed Go against the go directive, which
// is a lower bound rather than a pin. A substring match read it as an exact
// version, so upgrading to the next patch release turned doctor red while
// breaking nothing — `go build` is perfectly happy above the floor.
func requiredGoVersion(want string) int {
	output, err := capture("go", "version")
	if err != nil {
		fmt.Println("[FAIL] Go: go not found")
		return 1
	}
	ok, known := goVersionSatisfies(output, want)
	switch {
	case !known:
		// An unreadable version string is not a reason to block: the toolchain is
		// present, and the build is the real gate.
		fmt.Printf("[WARN] Go: cannot compare %s against go.mod's %s\n", oneLine(output), want)
		return 0
	case !ok:
		fmt.Printf("[FAIL] Go: got %s; go.mod needs %s or newer\n", oneLine(output), want)
		return 1
	}
	fmt.Printf("[OK]   Go: %s\n", oneLine(output))
	return 0
}

// goVersionSatisfies reports whether `go version` output clears the go.mod
// floor, and whether the comparison could be made at all.
func goVersionSatisfies(output, want string) (ok, known bool) {
	floor, parsed := versionParts(want)
	if !parsed {
		return false, false
	}
	got, parsed := versionParts(goVersionToken(output))
	if !parsed {
		return false, false
	}
	for i := range got {
		if got[i] != floor[i] {
			return got[i] > floor[i], true
		}
	}
	return true, true
}

// goVersionToken picks the toolchain out of `go version` output:
// "go version go1.26.6 darwin/arm64" gives "go1.26.6". Development builds print
// "devel go1.28-<hash>" in that slot, so the token is found by shape rather
// than by position.
func goVersionToken(output string) string {
	for _, field := range strings.Fields(output) {
		if rest, found := strings.CutPrefix(field, "go"); found && rest != "" && rest[0] >= '0' && rest[0] <= '9' {
			return rest
		}
	}
	return ""
}

// versionParts reads the leading major.minor.patch of a Go version, padding a
// missing patch with zero: the first release of a minor is "go1.26", not
// "go1.26.0". A prerelease suffix is dropped rather than ordered — an rc of a
// later minor already clears an earlier floor, which is all this has to decide.
func versionParts(value string) ([3]int, bool) {
	var parts [3]int
	value = strings.TrimPrefix(strings.TrimSpace(value), "go")
	if value == "" {
		return parts, false
	}
	for i, field := range strings.SplitN(value, ".", 3) {
		digits := 0
		for digits < len(field) && field[digits] >= '0' && field[digits] <= '9' {
			digits++
		}
		if digits == 0 {
			return parts, false
		}
		number, err := strconv.Atoi(field[:digits])
		if err != nil {
			return parts, false
		}
		parts[i] = number
	}
	return parts, true
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
