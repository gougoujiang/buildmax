package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// desktopReleaseDir is where `release desktop` writes its downloadable
// artifacts. It sits under dist/ so it shares the release-output ignore rule
// GoReleaser already relies on, and stays out of bin/, which holds the
// launchable binaries `build` produces for local `run`.
const desktopReleaseDir = "dist/desktop"

// cmdReleaseDesktop builds the Wails app and packages it into a downloadable
// artifact for the host platform: a .dmg on macOS, the self-contained .exe on
// Windows. GoReleaser cannot produce these -- it runs on one Linux runner, and
// a native macOS bundle needs a macOS runner -- so the desktop release is a
// per-OS job (.github/workflows/desktop-release.yml) that calls this on each.
//
// The artifacts are unsigned during alpha, so a downloaded macOS bundle is held
// by Gatekeeper and a Windows binary warns under SmartScreen until the user
// clears it; manual/install.md documents the one-time step. Signing and
// notarization are the next increment, not a reason to ship nothing.
func cmdReleaseDesktop(args []string) error {
	if len(args) != 0 {
		return usageErrorf("release", "release desktop takes no arguments")
	}
	// Build gui first: buildDesktop refuses to run without gui/dist, which the
	// desktop frontend consumes through a file: dependency.
	if err := buildGUI(); err != nil {
		return err
	}
	if err := buildDesktop(); err != nil {
		return err
	}
	if err := os.MkdirAll(desktopReleaseDir, 0o755); err != nil {
		return err
	}

	version := resolveVersion()
	base := fmt.Sprintf("%s_%s_%s_%s", desktopBinary, version, runtime.GOOS, runtime.GOARCH)
	switch runtime.GOOS {
	case "darwin":
		return packageDesktopMacOS(base)
	case "windows":
		return packageDesktopWindows(base)
	default:
		return fmt.Errorf("release desktop packages only macOS and Windows; %s desktop bundles are not distributed", runtime.GOOS)
	}
}

// packageDesktopMacOS wraps the .app bundle in a compressed .dmg with hdiutil,
// which ships with macOS. The user mounts it and drags BuildMax.app out; the
// bundle is unsigned, so the first launch needs the Gatekeeper step in the
// install guide.
func packageDesktopMacOS(base string) error {
	app := filepath.Join(desktopDir, "build", "bin", "BuildMax.app")
	if !isDir(app) {
		return fmt.Errorf("no macOS bundle at %s; `build desktop` did not produce one", app)
	}
	out := filepath.Join(desktopReleaseDir, base+".dmg")
	// -ov overwrites a stale artifact from a rerun; UDZO is the compressed,
	// read-only format a distributed dmg uses.
	if err := runCmd("hdiutil", "create", "-volname", "BuildMax", "-srcfolder", app, "-ov", "-format", "UDZO", out); err != nil {
		return fmt.Errorf("build dmg: %w", err)
	}
	return finishDesktopArtifact(out)
}

// packageDesktopWindows publishes the Wails .exe directly. It is self-contained
// -- a single executable a user runs without an installer -- so an NSIS step
// would add a toolchain dependency without changing what the user downloads and
// runs. SmartScreen warns on the unsigned binary until the user chooses to run
// it anyway.
func packageDesktopWindows(base string) error {
	exePath := filepath.Join(desktopDir, "build", "bin", exe(desktopBinary))
	if !exists(exePath) {
		return fmt.Errorf("no Windows binary at %s; `build desktop` did not produce one", exePath)
	}
	out := filepath.Join(desktopReleaseDir, base+".exe")
	if err := copyFile(exePath, out, 0o755); err != nil {
		return fmt.Errorf("copy desktop binary: %w", err)
	}
	return finishDesktopArtifact(out)
}

// finishDesktopArtifact writes a sha256 checksum next to the artifact and
// reports both paths. A checksum matters more for an unsigned download than a
// signed one: it is the only integrity check a user has before running it.
func finishDesktopArtifact(artifact string) error {
	sum, err := sha256File(artifact)
	if err != nil {
		return err
	}
	// The `<hex>  <name>` layout is what `sha256sum -c` and `shasum -a 256 -c`
	// read, so a user can verify the download with the tool they already have.
	line := fmt.Sprintf("%s  %s\n", sum, filepath.Base(artifact))
	checksum := artifact + ".sha256"
	if err := os.WriteFile(checksum, []byte(line), 0o644); err != nil {
		return fmt.Errorf("write checksum: %w", err)
	}
	logf("desktop", "Packaged %s", artifact)
	logf("desktop", "Checksum %s", checksum)
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
