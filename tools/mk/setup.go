package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// setup is doctor's write half: doctor names the command that fixes each
// missing piece, and setup runs them.
//
// They stay two commands rather than one command with a flag. Doctor's contract
// is that it changes nothing, which is what makes it the safe first command in
// an unfamiliar checkout; a `--fix` flag would cost that sentence for every
// reader who only wanted the diagnosis. The scope argument matches what doctor
// reports, so the pair reads as one idea: doctor says what is missing, `setup
// <scope>` creates or installs it.
func cmdSetup(args []string) error {
	if len(args) != 1 {
		return usageErrorf("setup", "setup takes one scope: local or harbor")
	}
	switch args[0] {
	case "local":
		return setupLocal()
	case "harbor":
		return setupHarbor()
	default:
		return usageErrorf("setup", "unknown setup scope: %s", args[0])
	}
}

// harborSetup carries the directories setup had to add to this process's PATH.
// uv installs into ~/.local/bin, which a shell that is already open does not
// have, so without this the report at the end would call the tools it just
// installed missing — and the reader would never be told why the next terminal
// cannot find them either.
type harborSetup struct {
	adopted []string
}

// setupHarbor installs what a Terminal-Bench run needs and then re-reads the
// machine with doctor's own probes. Sharing the probe list is the point: what
// setup installs and what doctor requires cannot drift apart, and the last
// thing printed is the same report the contributor would get by asking.
func setupHarbor() error {
	pins, err := readHarborPins(harborPinsPath)
	if err != nil {
		return err
	}

	fmt.Println("BuildMax benchmark setup")
	fmt.Println("========================")

	setup := &harborSetup{}
	uv, err := setup.ensureUV()
	if err != nil {
		return err
	}
	if err := setup.ensureHarbor(uv, pins); err != nil {
		return err
	}
	if err := setup.ensureLinuxCLI(); err != nil {
		return err
	}

	fmt.Println()
	failures := reportHarborProbes(harborProbes())
	fmt.Println()
	for _, dir := range setup.adopted {
		printPathHint(dir)
		fmt.Println()
	}
	if failures > 0 {
		// A sandbox is the case this reaches: Docker is an installer with a
		// licence and a GUI, and picking one for a contributor is not this
		// command's business. The probe already printed both ways out.
		return fmt.Errorf("setup finished with %d problem(s) it cannot fix from here", failures)
	}
	fmt.Println("Summary: this checkout can run the pinned Terminal-Bench target")
	fmt.Println("See evaluation/harbor/README.md for what a run costs and how to start one")
	return nil
}

const (
	uvInstallScript     = "https://astral.sh/uv/install.sh"
	uvInstallPowerShell = "https://astral.sh/uv/install.ps1"
	uvDocumentation     = "https://docs.astral.sh/uv/"
)

func (s *harborSetup) ensureUV() (string, error) {
	if path := toolPath("uv"); path != "" {
		logf("setup", "uv: %s", path)
		s.adopt(path)
		return path, nil
	}
	name, args, err := uvInstallCommand(runtime.GOOS, fetchTool())
	if err != nil {
		return "", err
	}
	logf("setup", "uv: not installed")
	// Printed before it runs. This is the only step that fetches and executes
	// code from outside the repository, so a contributor who does not want that
	// has to be able to see exactly what it was and stop.
	logf("setup", "running: %s", commandLine(name, args))
	if err := runCmd(name, args...); err != nil {
		return "", err
	}
	path := toolPath("uv")
	if path == "" {
		return "", fmt.Errorf("the uv installer finished but left no uv on PATH or in %s; install it from %s and re-run",
			strings.Join(toolInstallDirs(), ", "), uvDocumentation)
	}
	logf("setup", "uv: installed at %s", path)
	s.adopt(path)
	return path, nil
}

// uvInstallCommand builds the installer Astral documents for this platform. It
// is separate from running it because the shape is what breaks: a wrong
// one-liner is only ever discovered on a machine that has no uv, which is
// exactly the machine that cannot debug it.
func uvInstallCommand(goos, fetcher string) (string, []string, error) {
	if goos == "windows" {
		return "powershell", []string{"-ExecutionPolicy", "ByPass", "-c", "irm " + uvInstallPowerShell + " | iex"}, nil
	}
	switch fetcher {
	case "curl":
		return "sh", []string{"-c", "curl -LsSf " + uvInstallScript + " | sh"}, nil
	case "wget":
		return "sh", []string{"-c", "wget -qO- " + uvInstallScript + " | sh"}, nil
	}
	return "", nil, fmt.Errorf("uv is missing and this machine has neither curl nor wget; install uv from %s and re-run", uvDocumentation)
}

func fetchTool() string {
	switch {
	case have("curl"):
		return "curl"
	case have("wget"):
		return "wget"
	}
	return ""
}

func (s *harborSetup) ensureHarbor(uv string, pins harborPins) error {
	want := pins.Harbor.Version
	installed := ""
	if path := toolPath("harbor"); path != "" {
		if out, err := capture(path, "--version"); err == nil {
			installed = oneLine(out)
		}
		if installed == want {
			logf("setup", "Harbor: %s at %s", want, path)
			s.adopt(path)
			return nil
		}
	}
	if installed == "" {
		logf("setup", "Harbor: installing %s", want)
	} else {
		// Not an upgrade: the adapter reaches Harbor internals and the run CLI
		// renamed a flag, so a newer Harbor is as wrong here as an older one.
		logf("setup", "Harbor: replacing %s with the pinned %s", installed, want)
	}
	args := harborInstallArgs(want, installed != "")
	logf("setup", "running: %s", commandLine(uv, args))
	if err := runCmd(uv, args...); err != nil {
		return err
	}
	path := toolPath("harbor")
	if path == "" {
		return fmt.Errorf("uv reported success but left no harbor on PATH or in %s", strings.Join(toolInstallDirs(), ", "))
	}
	logf("setup", "Harbor: installed at %s", path)
	s.adopt(path)
	return nil
}

// harborInstallArgs builds the argv from the pinned version rather than by
// splitting the pin file's install line: a sentence written for a human to read
// is not an argument vector. TestHarborInstallArgsMatchThePinnedLine holds the
// two together, so a pin file that switched installers could not leave this
// silently running uv.
func harborInstallArgs(version string, replace bool) []string {
	args := []string{"tool", "install"}
	if replace {
		// uv leaves an already-installed tool alone otherwise, which would keep
		// a version the adapter is not written against and report success.
		args = append(args, "--force")
	}
	return append(args, "harbor=="+version)
}

func (s *harborSetup) ensureLinuxCLI() error {
	if probe := harborBinaryProbe(); probe.Level == probeOK {
		logf("setup", "Linux CLI: %s", probe.Detail)
		return nil
	}
	return cmdBuild([]string{"cli", "linux/" + harborTrialArch})
}

// toolPath finds a tool the way a run will, and then where uv would have put
// one. PATH alone answers "is it installed" wrongly for the whole of the first
// setup: the installer writes into a directory this process inherited its PATH
// without.
func toolPath(name string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	for _, dir := range toolInstallDirs() {
		candidate := filepath.Join(dir, exe(name))
		if exists(candidate) {
			return candidate
		}
	}
	return ""
}

// toolInstallDirs are where uv puts an executable when nothing overrides it,
// most specific first.
func toolInstallDirs() []string {
	var dirs []string
	for _, key := range []string{"UV_TOOL_BIN_DIR", "XDG_BIN_HOME"} {
		if dir := os.Getenv(key); dir != "" {
			dirs = append(dirs, dir)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return dirs
	}
	// ~/.cargo/bin is where uv's own installer used to land, and a contributor
	// who installed it years ago still has it there.
	return append(dirs, filepath.Join(home, ".local", "bin"), filepath.Join(home, ".cargo", "bin"))
}

func (s *harborSetup) adopt(path string) {
	dir := filepath.Dir(path)
	if onPath(dir) {
		return
	}
	for _, seen := range s.adopted {
		if seen == dir {
			return
		}
	}
	s.adopted = append(s.adopted, dir)
	_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
