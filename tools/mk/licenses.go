// License chores: the third-party notice file shipped with every release, and
// the npm production-dependency license policy.
//
// These ran as scripts/gen-third-party-notices.sh and
// scripts/check-npm-licenses.mjs until they moved here. The shell half could
// not run on native Windows, and the whole point of tools/mk is that every
// contributor runs the same task code. Both still depend only on the standard
// library.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// noticeSeparator frames each module's block. Its width, and every byte of the
// header below, reproduce what the shell script emitted: NOTICE-THIRD-PARTY is
// the Apache-2.0 section 4(d) attribution shipped in release archives, so the
// port had to be byte-identical rather than merely equivalent.
const noticeSeparator = "================================================================"

const noticeHeader = `BuildMax third-party notices

BuildMax itself is licensed under Apache-2.0; see LICENSE.
The binaries in this distribution statically link the modules below.
Each module's own license text follows, unmodified.

A summary of the license mix, and how to regenerate this file, is in
docs/contribute/dependency-licenses.md in the source repository.

`

// cmdNotices produces NOTICE-THIRD-PARTY: the license text of every module
// linked into the released binaries.
func cmdNotices(args []string) error {
	fs := flag.NewFlagSet("release notices", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	out := "NOTICE-THIRD-PARTY"
	if fs.NArg() > 0 {
		out = fs.Arg(0)
	}

	tool, err := goLicensesPath()
	if err != nil {
		return err
	}

	work, err := os.MkdirTemp("", "buildmax-notices-")
	if err != nil {
		return fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(work)

	// go-licenses refuses to write into an existing directory, so name a child.
	save := filepath.Join(work, "licenses")
	cmd := exec.Command(tool, "save", "./cmd/...", "--save_path="+save, "--force")
	// go-licenses logs a wall of warnings about packages it cannot classify even
	// on a successful run, so its stderr is held back and shown only on failure.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go-licenses save: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}

	count, err := writeNotices(out, save)
	if err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d modules)\n", out, count)
	return nil
}

// goLicensesPath finds the go-licenses binary, on PATH or in GOPATH/bin, and
// says how to install it when it is missing.
func goLicensesPath() (string, error) {
	if path, err := exec.LookPath("go-licenses"); err == nil {
		return path, nil
	}
	gopath, err := exec.Command("go", "env", "GOPATH").Output()
	if err == nil {
		candidate := filepath.Join(strings.TrimSpace(string(gopath)), "bin", exe("go-licenses"))
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("go-licenses not found. Install it with:\n  go install github.com/google/go-licenses@v1.6.0")
}

// writeNotices concatenates every license file under saveDir into outPath and
// reports how many it wrote.
func writeNotices(outPath, saveDir string) (int, error) {
	files, err := licenseFiles(saveDir)
	if err != nil {
		return 0, err
	}

	var buf bytes.Buffer
	buf.WriteString(noticeHeader)
	for _, file := range files {
		rel, err := filepath.Rel(saveDir, file)
		if err != nil {
			return 0, fmt.Errorf("locate %s under %s: %w", file, saveDir, err)
		}
		module := filepath.ToSlash(filepath.Dir(rel))
		body, err := os.ReadFile(file)
		if err != nil {
			return 0, fmt.Errorf("read license %s: %w", file, err)
		}
		fmt.Fprintf(&buf, "%s\n%s\n%s\n\n", noticeSeparator, module, noticeSeparator)
		buf.Write(body)
		buf.WriteString("\n")
	}

	if err := os.WriteFile(outPath, buf.Bytes(), 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", outPath, err)
	}
	return len(files), nil
}

// licenseFiles returns every license-shaped file under root, sorted by byte
// order. The order is the file's contract: regenerating must produce an
// identical document, and byte order is what `LC_ALL=C sort` gave the shell
// version this replaces.
func licenseFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, "LICENSE") ||
			strings.HasPrefix(name, "COPYING") ||
			strings.HasPrefix(name, "NOTICE") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk saved licenses: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

// npmLockfiles are the three package-lock.json files whose production
// dependencies ship to users.
var npmLockfiles = []string{
	"gui/package-lock.json",
	"portal/package-lock.json",
	"desktop/frontend/package-lock.json",
}

// npmAllowedLicenses is the permissive set that can be redistributed under
// Apache-2.0 without added obligations. Anything else is a decision, not a
// detail, so the check fails and a human looks at it.
var npmAllowedLicenses = map[string]bool{
	"Apache-2.0":   true,
	"BSD-2-Clause": true,
	"BSD-3-Clause": true,
	"ISC":          true,
	"MIT":          true,
	"0BSD":         true,
}

type npmPackage struct {
	Name    string          `json:"name"`
	License json.RawMessage `json:"license"`
	Dev     bool            `json:"dev"`
	Link    bool            `json:"link"`
}

// cmdNPMLicenses fails when a production npm dependency carries a license
// outside the allowed set, or carries none at all.
func cmdNPMLicenses(args []string) error {
	fs := flag.NewFlagSet("release licenses", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	targets := npmLockfiles
	if fs.NArg() > 0 {
		targets = fs.Args()
	}

	var failures []string
	for _, lockfile := range targets {
		checked, counts, lockFailures, err := checkNPMLockfile(lockfile)
		if err != nil {
			return err
		}
		failures = append(failures, lockFailures...)

		licenses := make([]string, 0, len(counts))
		for license := range counts {
			licenses = append(licenses, license)
		}
		sort.Strings(licenses)
		summary := make([]string, 0, len(licenses))
		for _, license := range licenses {
			summary = append(summary, fmt.Sprintf("%s=%d", license, counts[license]))
		}
		fmt.Printf("%s: checked %d production packages (%s)\n",
			lockfile, checked, strings.Join(summary, ", "))
	}

	if len(failures) > 0 {
		fmt.Fprintln(os.Stderr, "npm production dependency license check failed:")
		for _, failure := range failures {
			fmt.Fprintf(os.Stderr, "- %s\n", failure)
		}
		return fmt.Errorf("%d npm production dependencies violate the license policy", len(failures))
	}
	return nil
}

// checkNPMLockfile reports how many production packages it inspected, the
// license mix, and one message per violation.
func checkNPMLockfile(lockfile string) (int, map[string]int, []string, error) {
	body, err := os.ReadFile(lockfile)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("read %s: %w", lockfile, err)
	}
	var lock struct {
		Packages json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal(body, &lock); err != nil {
		return 0, nil, nil, fmt.Errorf("parse %s: %w", lockfile, err)
	}
	packages := map[string]npmPackage{}
	if len(lock.Packages) == 0 || json.Unmarshal(lock.Packages, &packages) != nil {
		return 0, nil, []string{lockfile + ": package-lock.json has no packages map"}, nil
	}

	// Sorted so a failing run names the same package first every time.
	paths := make([]string, 0, len(packages))
	for path := range packages {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	counts := map[string]int{}
	var failures []string
	checked := 0
	for _, path := range paths {
		metadata := packages[path]
		// The empty path is the repository's own package. Linked @buildmax/gui
		// entries point to that same local package and carry no lockfile license.
		if path == "" || metadata.Dev || metadata.Link {
			continue
		}
		checked++

		name := metadata.Name
		if name == "" {
			name = filepath.Base(path)
		}
		var license string
		if json.Unmarshal(metadata.License, &license) != nil || license == "" {
			failures = append(failures, fmt.Sprintf("%s: %s has no license metadata", lockfile, name))
			continue
		}
		if !npmAllowedLicenses[license] {
			failures = append(failures, fmt.Sprintf("%s: %s uses unapproved license %s", lockfile, name, license))
			continue
		}
		counts[license]++
	}
	return checked, counts, failures, nil
}
