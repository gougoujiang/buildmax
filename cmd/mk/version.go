package main

import (
	"fmt"
	"strconv"
	"strings"
)

const versionPkg = "github.com/gougoujiang/buildmax/internal/config"

// resolveCommitSHA reports the short git SHA for HEAD, or "dev" when git or the
// repository is unavailable. A dirty working tree is marked, so a binary built
// from uncommitted changes says so.
func resolveCommitSHA() string {
	if !have("git") {
		return "dev"
	}
	sha, err := capture("git", "rev-parse", "--short=7", "HEAD")
	if err != nil || sha == "" {
		return "dev"
	}
	if isDirty() {
		sha += "-dirty"
	}
	return sha
}

// resolveVersion derives the version a build calls itself from git tags, which
// keeps the tag the single source of truth. On an exact tag: "0.1.0". Between
// tags: "0.1.0-3-gabc1234". With no tags or no git: "dev".
func resolveVersion() string {
	if !have("git") {
		return "dev"
	}
	described, err := capture("git", "describe", "--tags", "--match", "v[0-9]*")
	if err != nil || described == "" {
		return "dev"
	}
	return strings.TrimPrefix(described, "v")
}

func isDirty() bool {
	return !succeeds("git", "diff", "--quiet", "HEAD", "--")
}

// ldflags injects the version and commit the same way .goreleaser.yaml does, so
// a local build reports its provenance like a released one.
func ldflags() string {
	return fmt.Sprintf("-X %s.Version=%s -X %s.Commit=%s",
		versionPkg, resolveVersion(), versionPkg, resolveCommitSHA())
}

// cmdBump creates the next release tag locally and leaves pushing to the
// developer, because pushing the tag is what triggers the release build.
// Versions live in git tags rather than in a source file; see resolveVersion.
func cmdBump(args []string) error {
	bump := "patch"
	if len(args) > 0 && args[0] != "" {
		bump = args[0]
	}
	switch bump {
	case "patch", "minor", "major":
	default:
		return fmt.Errorf("bump must be patch, minor, or major (got: %s)", bump)
	}
	if !have("git") {
		return fmt.Errorf("git is required to tag a release")
	}
	if isDirty() {
		return fmt.Errorf("working tree has uncommitted changes; commit them before tagging")
	}

	current, err := capture("git", "describe", "--tags", "--match", "v[0-9]*", "--abbrev=0")
	if err != nil || current == "" {
		current = "v0.0.0"
	}
	major, minor, patch, err := parseVersion(current)
	if err != nil {
		return err
	}
	switch bump {
	case "patch":
		patch++
	case "minor":
		minor, patch = minor+1, 0
	case "major":
		major, minor, patch = major+1, 0, 0
	}

	next := fmt.Sprintf("v%d.%d.%d", major, minor, patch)
	if succeeds("git", "rev-parse", "-q", "--verify", "refs/tags/"+next) {
		return fmt.Errorf("tag %s already exists", next)
	}
	if err := runCmd("git", "tag", "-a", next, "-m", next); err != nil {
		return fmt.Errorf("could not create tag %s: %w", next, err)
	}
	fmt.Printf("Tagged: %s -> %s (%s)\n", current, next, bump)
	fmt.Println("Push it to build and publish the release:")
	fmt.Printf("  git push origin %s\n", next)
	fmt.Printf("Undo with: git tag -d %s\n", next)
	return nil
}

// parseVersion reads "v0.1.0" or "v0.1.0-alpha" as 0, 1, 0. Missing components
// count as zero, so a "v1" tag is still usable.
func parseVersion(tag string) (major, minor, patch int, err error) {
	base := strings.TrimPrefix(tag, "v")
	if i := strings.Index(base, "-"); i >= 0 {
		base = base[:i]
	}
	out := [3]int{}
	for i, part := range strings.SplitN(base, ".", 3) {
		if part == "" {
			continue
		}
		n, convErr := strconv.Atoi(part)
		if convErr != nil {
			return 0, 0, 0, fmt.Errorf("could not parse version from tag %q", tag)
		}
		out[i] = n
	}
	return out[0], out[1], out[2], nil
}
