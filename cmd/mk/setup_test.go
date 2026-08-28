package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupTakesExactlyOneKnownScope(t *testing.T) {
	for _, args := range [][]string{nil, {}, {"portal"}, {"harbor", "extra"}} {
		err := cmdSetup(args)
		if err == nil {
			t.Fatalf("cmdSetup(%q) returned no error", args)
		}
		if !strings.Contains(err.Error(), "setup takes one scope") {
			t.Errorf("cmdSetup(%q) error = %v, want the usage line", args, err)
		}
	}
}

// The installer one-liner is only ever exercised on a machine with no uv, which
// is the machine least able to work out what went wrong with it. These are the
// commands Astral documents.
func TestUVInstallCommandPerPlatform(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		fetcher string
		want    string
	}{
		{name: "macOS with curl", goos: "darwin", fetcher: "curl",
			want: "sh -c curl -LsSf https://astral.sh/uv/install.sh | sh"},
		{name: "Linux with only wget", goos: "linux", fetcher: "wget",
			want: "sh -c wget -qO- https://astral.sh/uv/install.sh | sh"},
		{name: "Windows has its own", goos: "windows", fetcher: "",
			want: "powershell -ExecutionPolicy ByPass -c irm https://astral.sh/uv/install.ps1 | iex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args, err := uvInstallCommand(tt.goos, tt.fetcher)
			if err != nil {
				t.Fatalf("uvInstallCommand(%q, %q) = %v", tt.goos, tt.fetcher, err)
			}
			if got := commandLine(name, args); got != tt.want {
				t.Errorf("uvInstallCommand(%q, %q) = %q, want %q", tt.goos, tt.fetcher, got, tt.want)
			}
		})
	}
}

// Failing here is the whole point: a machine with no way to fetch the installer
// has to be told to install uv itself, not watch a command fail inside a shell.
func TestUVInstallCommandNeedsAFetcher(t *testing.T) {
	_, _, err := uvInstallCommand("linux", "")
	if err == nil {
		t.Fatal("uvInstallCommand with no fetcher returned no error")
	}
	if !strings.Contains(err.Error(), uvDocumentation) {
		t.Errorf("error = %v, want it to name %s", err, uvDocumentation)
	}
}

// setup builds the argv itself, so this is what stops it from running uv
// against a pin file that has moved on to another installer.
func TestHarborInstallArgsMatchThePinnedLine(t *testing.T) {
	pins, err := readHarborPins(filepath.Join("..", "..", harborPinsPath))
	if err != nil {
		t.Fatalf("readHarborPins: %v", err)
	}
	got := commandLine("uv", harborInstallArgs(pins.Harbor.Version, false))
	if got != pins.Harbor.Install {
		t.Errorf("setup would run %q; pins.json documents %q", got, pins.Harbor.Install)
	}
}

// A wrong version already installed is the case uv would otherwise leave alone
// while reporting success.
func TestHarborInstallArgsForceReplaceAWrongVersion(t *testing.T) {
	args := harborInstallArgs("0.22.0", true)
	if commandLine("uv", args) != "uv tool install --force harbor==0.22.0" {
		t.Errorf("replacement command = %q", commandLine("uv", args))
	}
}

func TestToolPathFindsWhatUVInstalledOffPath(t *testing.T) {
	dir := t.TempDir()
	// A name no real machine has on PATH, so this tests the fallback and not
	// whichever uv the contributor already installed.
	tool := "buildmax-not-a-real-tool"
	path := filepath.Join(dir, exe(tool))
	if err := os.WriteFile(path, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UV_TOOL_BIN_DIR", dir)
	if got := toolPath(tool); got != path {
		t.Errorf("toolPath(%q) = %q, want %q", tool, got, path)
	}
	if got := toolPath(tool + "-missing"); got != "" {
		t.Errorf("toolPath found %q for a tool that does not exist", got)
	}
}

// Without this the report at the end of setup would call the tools setup just
// installed missing, because this process inherited its PATH before they existed.
func TestAdoptPutsAFreshInstallOnThisProcessPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", filepath.Join(dir, "elsewhere"))
	setup := &harborSetup{}
	setup.adopt(filepath.Join(dir, exe("uv")))
	setup.adopt(filepath.Join(dir, exe("harbor")))

	if len(setup.adopted) != 1 || setup.adopted[0] != dir {
		t.Fatalf("adopted = %q, want one entry for %q", setup.adopted, dir)
	}
	if !onPath(dir) {
		t.Errorf("PATH is %q, which does not contain %q", os.Getenv("PATH"), dir)
	}
}

func TestAdoptLeavesAPathDirectoryAlone(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	setup := &harborSetup{}
	setup.adopt(filepath.Join(dir, exe("uv")))
	if len(setup.adopted) != 0 {
		t.Errorf("adopted = %q, want nothing: the directory is already on PATH", setup.adopted)
	}
}
