package architecture_test

// Package naming. conventions.md says a package is named for what it owns, and
// this is the half of that rule a machine can check: a name that describes a
// container rather than a capability.
//
// The rule exists because those names have no wrong answer. Nobody can tell you
// that a file does not belong in `util`, so everything ends up there, and the
// package stops saying anything about what it holds. `internal/core/model`
// reached 22 files across eleven capabilities that way.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// containerNames describe what a package holds rather than what it owns.
//
// `core` is deliberately absent: it is a dependency-layer prefix, not a package
// name, and no Go package is called that. The packages beneath it each carry a
// capability.
var containerNames = map[string]bool{
	"common":  true,
	"shared":  true,
	"utils":   true,
	"helpers": true,
	"base":    true,
	"misc":    true,
	"models":  true,
	"types":   true,
	"lib":     true,
}

// knownContainerNames are the two that predate this rule. They are debt, named
// here so that the test can hold the line without pretending they are fine, and
// so that removing one is a matter of deleting a line rather than remembering
// this list exists.
//
// util holds atomicfile, id, and workspace -- three capabilities with names,
// under one that has none. Splitting it is not urgent; adding a third package
// like it would be a decision nobody made.
var knownContainerNames = map[string]bool{
	"internal/util": true,
}

// TestNoNewContainerNamedPackages fails when a package is named for what it
// contains. Adding one to knownContainerNames is not a fix; it is a decision to
// keep the debt, and it should read as one in review.
func TestNoNewContainerNamedPackages(t *testing.T) {
	root := moduleRoot(t)
	for _, tree := range []string{"internal", "cmd", "tools", "evaluation"} {
		base := filepath.Join(root, tree)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return nil
			}
			name := info.Name()
			if name == "node_modules" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			rel = filepath.ToSlash(rel)
			if knownContainerNames[rel] {
				return nil
			}
			if !containerNames[strings.ToLower(name)] {
				return nil
			}
			// Only a directory holding Go source is a package.
			entries, readErr := os.ReadDir(path)
			if readErr != nil {
				return readErr
			}
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
					t.Errorf("%s is named for what it contains rather than what it owns; "+
						"see the naming rule in docs/contribute/conventions.md", rel)
					break
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", tree, err)
		}
	}
}

// TestKnownContainerNamesStillExist keeps the exemption list from outliving
// what it exempts. A stale entry silently permits a name nobody meant to allow.
func TestKnownContainerNamesStillExist(t *testing.T) {
	root := moduleRoot(t)
	for rel := range knownContainerNames {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("%s is exempted from the naming rule and no longer exists; delete the entry", rel)
		}
	}
}
