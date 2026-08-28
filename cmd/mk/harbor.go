package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// harborPinsPath is the pin file evaluation/harbor owns. mk reads it rather
// than restating the version, because a mirrored constant is a second place to
// forget: moving the pin has to change what this command checks for.
var harborPinsPath = filepath.Join("evaluation", "harbor", "pins.json")

// harborPins is the slice of the pin file mk needs. Only these fields are
// declared: evaluation/harbor.Pins is the full shape and the authority, and mk
// imports nothing from the module so that `./make` still runs when the rest of
// the tree does not compile. harbor_test.go holds the two readers to the same
// values.
type harborPins struct {
	Harbor struct {
		Version string `json:"version"`
		Install string `json:"install"`
	} `json:"harbor"`
	Dataset struct {
		Name string `json:"name"`
		Ref  string `json:"ref"`
	} `json:"dataset"`
}

func readHarborPins(path string) (harborPins, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return harborPins{}, err
	}
	var pins harborPins
	if err := json.Unmarshal(data, &pins); err != nil {
		return harborPins{}, fmt.Errorf("%s: %w", path, err)
	}
	if pins.Harbor.Version == "" {
		return harborPins{}, fmt.Errorf("%s names no Harbor version", path)
	}
	return pins, nil
}

// probeLevel is how much a failed probe means.
type probeLevel int

const (
	probeOK probeLevel = iota
	// probeInfo is a fact worth stating that blocks nothing.
	probeInfo
	// probeWarn is something that will stop a particular kind of run. The
	// caller decides whether that kind is the one it is about to do.
	probeWarn
	// probeFail is a benchmark run that cannot start at all.
	probeFail
)

// probe is one environment check and, when it fails, the exact command that
// fixes it. The fix is the whole point: doctor never installs anything, so a
// report that only says "missing" leaves the reader to guess a version.
type probe struct {
	Label  string
	Level  probeLevel
	Detail string
	Fix    string
}

// harborProbes reports whether this machine can run the Terminal-Bench
// benchmark, without changing anything about it.
//
// It is shared rather than inlined into doctor because the same answer is owed
// twice: once when a contributor asks "am I set up", and once as the preflight
// of the run itself. Discovering an unauthenticated Hub after a container is
// already pulling is the failure this exists to move earlier.
func harborProbes() []probe {
	pins, err := readHarborPins(harborPinsPath)
	if err != nil {
		return []probe{{
			Label:  "pins",
			Level:  probeFail,
			Detail: err.Error(),
		}}
	}

	return []probe{
		harborDatasetProbe(pins),
		harborVersionProbe(pins),
		harborAuthProbe(),
		harborSandboxProbe(),
		harborBinaryProbe(),
	}
}

// harborDatasetProbe states what this checkout is set up to measure. It checks
// nothing; it is here because the first question after "can I run it" is "run
// what", and a benchmark named without its immutable ref is two datasets.
func harborDatasetProbe(pins harborPins) probe {
	ref := pins.Dataset.Ref
	if len(ref) > 20 {
		ref = ref[:20] + "…"
	}
	return probe{
		Label:  "Dataset",
		Level:  probeInfo,
		Detail: fmt.Sprintf("%s pinned to %s", pins.Dataset.Name, ref),
	}
}

// harborVersionProbe requires an exact version match. The adapter reaches
// underscore-prefixed helpers on Harbor's installed-agent base class and the
// run CLI renamed a flag at 0.22.0, so "at least" is not a bound that means
// anything here — a newer Harbor is as likely to break the adapter as an older
// one, and it would break it inside a container that already cost money.
func harborVersionProbe(pins harborPins) probe {
	want := pins.Harbor.Version
	fix := pins.Harbor.Install
	if fix == "" {
		fix = "uv tool install harbor==" + want
	}

	version, err := capture("harbor", "--version")
	if err != nil {
		detail := "not installed"
		if !have("uv") {
			detail = "not installed, and neither is uv"
			fix = "install uv from https://docs.astral.sh/uv/, then: " + fix
		}
		return probe{Label: "Harbor", Level: probeFail, Detail: detail, Fix: fix}
	}
	got := oneLine(version)
	if got != want {
		return probe{
			Label:  "Harbor",
			Level:  probeFail,
			Detail: fmt.Sprintf("got %s; this repository is pinned to %s", got, want),
			Fix:    fix,
		}
	}
	return probe{Label: "Harbor", Level: probeOK, Detail: got}
}

// harborAuthProbe reports Hub sign-in, and blocks nothing.
//
// Harbor's client is anonymous when logged out and public reads keep working,
// so downloading the pinned public dataset and running trials against it needs
// no account. Sign-in gates the calls that resolve a user: publishing, `--upload`,
// org and key management, and any private dataset.
//
// The state is read from the output, not the exit code. `harbor auth status`
// exits zero when logged out — it prints "Not authenticated" and returns, and
// only a credential it cannot verify exits non-zero. Reading the code would
// report a logged-out machine as signed in.
func harborAuthProbe() probe {
	if !have("harbor") {
		return probe{
			Label:  "Harbor Hub",
			Level:  probeInfo,
			Detail: "unknown until Harbor is installed",
		}
	}
	out, err := capture("harbor", "auth", "status")
	return harborAuthState(out, err)
}

// harborAuthState is split out so the three states can be tested without an
// installed Harbor. The logged-out one is why: it exits zero, so a probe that
// read the exit code reported a signed-out machine as signed in, with the
// command's own "Not authenticated" text on the OK line.
func harborAuthState(out string, err error) probe {
	if err != nil {
		return probe{
			Label:  "Harbor Hub",
			Level:  probeWarn,
			Detail: "credentials present but not verifiable: " + oneLine(out),
			Fix:    "harbor auth login",
		}
	}
	if strings.Contains(out, "Not authenticated") {
		// No fix line: nothing is broken. The command is in the detail for the
		// reader who does want to publish.
		return probe{
			Label:  "Harbor Hub",
			Level:  probeInfo,
			Detail: "not signed in; a local run does not need it (`harbor auth login` to publish one)",
		}
	}
	return probe{Label: "Harbor Hub", Level: probeOK, Detail: oneLine(out)}
}

// harborSandboxProbe reports where trials would run. Docker is one answer and a
// cloud sandbox is the other, so neither missing one is a failure on its own.
func harborSandboxProbe() probe {
	if have("docker") {
		return probe{Label: "Trial sandbox", Level: probeOK, Detail: "docker (local)"}
	}
	if os.Getenv("DAYTONA_API_KEY") != "" {
		return probe{Label: "Trial sandbox", Level: probeOK, Detail: "daytona (DAYTONA_API_KEY is set)"}
	}
	return probe{
		Label:  "Trial sandbox",
		Level:  probeWarn,
		Detail: "no local Docker and no DAYTONA_API_KEY; trials have nowhere to run",
		Fix:    "install Docker for a local canary, or export DAYTONA_API_KEY for a cloud run",
	}
}

// harborTrialArch is the architecture the uploaded CLI has to be built for.
//
// It is the container's, which is not the host's. The Terminal-Bench images are
// amd64, so an Apple Silicon machine runs them emulated and an arm64 binary
// uploaded into one dies with an exec format error — after the image has been
// pulled and the trial has started. Reading runtime.GOARCH here told exactly
// those contributors to build the one binary that cannot run.
const harborTrialArch = "amd64"

// harborBinaryProbe looks for the artifact a trial would measure. It never
// blocks: the binary is one build away, and doctor answers questions rather
// than standing in the way of the command that fixes them.
func harborBinaryProbe() probe {
	wanted := cliBinary + "-linux-" + harborTrialArch
	if exists(filepath.Join(binDir, wanted)) {
		return probe{Label: "Linux CLI", Level: probeOK, Detail: wanted}
	}
	fix := mk() + " build cli linux/" + harborTrialArch
	// A binary for another architecture is worth naming rather than ignoring:
	// the reader built one deliberately, and "none built" would read as a bug in
	// doctor rather than as the mismatch it is.
	if other := otherLinuxBinaries(); len(other) > 0 {
		return probe{
			Label:  "Linux CLI",
			Level:  probeWarn,
			Detail: fmt.Sprintf("%s built, but the task images are %s", strings.Join(other, ", "), harborTrialArch),
			Fix:    fix,
		}
	}
	return probe{
		Label:  "Linux CLI",
		Level:  probeWarn,
		Detail: "none built; a trial uploads this binary rather than fetching one",
		Fix:    fix,
	}
}

func otherLinuxBinaries() []string {
	var found []string
	for _, arch := range []string{"amd64", "arm64"} {
		if arch == harborTrialArch {
			continue
		}
		name := cliBinary + "-linux-" + arch
		if exists(filepath.Join(binDir, name)) {
			found = append(found, name)
		}
	}
	return found
}

// reportHarborProbes prints one section and returns how many probes block a
// benchmark run outright.
func reportHarborProbes(probes []probe) int {
	failures := 0
	for _, p := range probes {
		switch p.Level {
		case probeOK:
			fmt.Printf("[OK]   %s: %s\n", p.Label, p.Detail)
		case probeInfo:
			fmt.Printf("[INFO] %s: %s\n", p.Label, p.Detail)
		case probeWarn:
			fmt.Printf("[WARN] %s: %s\n", p.Label, p.Detail)
		case probeFail:
			failures++
			fmt.Printf("[FAIL] %s: %s\n", p.Label, p.Detail)
		}
		// Only where something is actually blocked. "fix" on an informational
		// line would tell the reader to repair a machine that is already fine.
		if p.Fix != "" && (p.Level == probeWarn || p.Level == probeFail) {
			fmt.Printf("       fix: %s\n", p.Fix)
		}
	}
	return failures
}
