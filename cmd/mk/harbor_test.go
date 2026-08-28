package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/evaluation/harbor"
)

// The task runner imports nothing from the module, so that `./make` still runs
// when the rest of the tree does not compile — which is when a contributor most
// needs `./make test` to work. That leaves mk with its own reader for the pin
// file, and this is what stops the two from disagreeing.
//
// A test file is allowed the import the command is not: `go run ./cmd/mk` does
// not compile _test.go, so the guard costs the runner nothing.
func TestTheTaskRunnerReadsTheSamePinsAsEvaluation(t *testing.T) {
	authoritative, err := harbor.LoadPins(filepath.Join("..", "..", harborPinsPath))
	if err != nil {
		t.Fatalf("LoadPins: %v", err)
	}
	mine, err := readHarborPins(filepath.Join("..", "..", harborPinsPath))
	if err != nil {
		t.Fatalf("readHarborPins: %v", err)
	}
	if mine.Harbor.Version != authoritative.Harbor.Version {
		t.Errorf("mk reads Harbor %q, evaluation/harbor reads %q",
			mine.Harbor.Version, authoritative.Harbor.Version)
	}
	if mine.Harbor.Install != authoritative.Harbor.Install {
		t.Errorf("mk reads install %q, evaluation/harbor reads %q",
			mine.Harbor.Install, authoritative.Harbor.Install)
	}
	if mine.Dataset.Name != authoritative.Dataset.Name {
		t.Errorf("mk reads dataset %q, evaluation/harbor reads %q",
			mine.Dataset.Name, authoritative.Dataset.Name)
	}
	if mine.Dataset.Ref != authoritative.Dataset.Ref {
		t.Errorf("mk reads ref %q, evaluation/harbor reads %q",
			mine.Dataset.Ref, authoritative.Dataset.Ref)
	}
}

// The path is relative to the repository root, which is where the shim puts the
// process. A rename that missed this constant would report every machine as
// unable to run the benchmark, with a message about a missing file.
func TestThePinnedPathIsWhereTheFileIs(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "..", harborPinsPath)); err != nil {
		t.Fatalf("harborPinsPath does not resolve from the repository root: %v", err)
	}
}

func TestReadHarborPinsRejectsAFileWithNoVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pins.json")
	if err := os.WriteFile(path, []byte(`{"dataset":{"name":"d"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readHarborPins(path); err == nil {
		t.Fatal("readHarborPins accepted a pin file naming no Harbor version")
	}
}

// Only a probe that stops a run outright counts. Hub sign-in and a missing
// Linux binary are warnings on purpose — one is a publishing decision and the
// other is fixed by the command the reader is about to run — and counting
// either would tell a contributor with a working local canary that their
// machine cannot run the benchmark.
func TestOnlyBlockingProbesCount(t *testing.T) {
	probes := []probe{
		{Label: "a", Level: probeOK},
		{Label: "b", Level: probeInfo},
		{Label: "c", Level: probeWarn, Fix: "sign in"},
		{Label: "d", Level: probeFail, Fix: "install it"},
		{Label: "e", Level: probeFail},
	}
	if got := reportHarborProbes(probes); got != 2 {
		t.Errorf("blocking probes = %d, want 2", got)
	}
}

// `harbor auth status` exits zero when logged out: it prints "Not
// authenticated" and returns, and only a credential it cannot verify exits
// non-zero. A probe that read the exit code reported a signed-out machine as
// signed in — an OK line carrying the command's own denial as its detail.
//
// Being signed out blocks nothing. Harbor's client is anonymous when logged
// out and public reads keep working, so the pinned dataset downloads and the
// trials run; sign-in gates publishing.
func TestHarborAuthStateReadsTheOutputNotTheExitCode(t *testing.T) {
	tests := []struct {
		name  string
		out   string
		err   error
		level probeLevel
	}{
		{
			name:  "logged out exits zero and blocks nothing",
			out:   "Not authenticated. Run `harbor auth login`.\n",
			level: probeInfo,
		},
		{name: "signed in", out: "Logged in as someone\n", level: probeOK},
		{name: "api key", out: "API key authentication (via HARBOR_API_KEY)\n", level: probeOK},
		{
			name:  "credentials that cannot be verified",
			out:   "Could not verify your credentials: network unreachable\n",
			err:   errUnverifiable,
			level: probeWarn,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := harborAuthState(tt.out, tt.err)
			if got.Level != tt.level {
				t.Errorf("level = %v, want %v (detail: %q)", got.Level, tt.level, got.Detail)
			}
			if got.Level == probeOK && strings.Contains(got.Detail, "Not authenticated") {
				t.Errorf("an OK line carries its own denial: %q", got.Detail)
			}
		})
	}
}

var errUnverifiable = errors.New("exit status 1")

func TestTheDatasetProbeNamesTheImmutableRef(t *testing.T) {
	pins, err := readHarborPins(filepath.Join("..", "..", harborPinsPath))
	if err != nil {
		t.Fatalf("readHarborPins: %v", err)
	}
	got := harborDatasetProbe(pins)
	if got.Level != probeInfo {
		t.Errorf("dataset probe level = %v, want informational: it checks nothing", got.Level)
	}
	if !strings.Contains(got.Detail, pins.Dataset.Name) {
		t.Errorf("dataset probe %q does not name the dataset", got.Detail)
	}
	// Truncated for the line, but never to the point of dropping the algorithm:
	// a bare hex prefix does not say what it is a digest of.
	if !strings.Contains(got.Detail, "sha256:") {
		t.Errorf("dataset probe %q does not show the ref", got.Detail)
	}
}
