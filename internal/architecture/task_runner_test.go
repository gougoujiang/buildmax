package architecture_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// kindSeedFlagRe matches a flag literal in cmd/mk's `model add` command line.
var kindSeedFlagRe = regexp.MustCompile(`"(--[a-z][a-z0-9-]*)"`)

// modelAddFlagRe matches a flag the add command defines.
var modelAddFlagRe = regexp.MustCompile(`fs\.(?:String|Int|Bool)\("([a-z][a-z0-9-]*)"`)

// TestKindSeedFlagsExist holds the task runner's model seeding against the
// command it drives. cmd/mk depends only on the standard library, so it cannot
// import the flag set and the two drift apart silently: `model add` parses with
// flag.ContinueOnError, which means a flag it no longer defines fails the whole
// seed rather than being ignored. This caught `--prompt-cache`, left behind
// when the settings key became a cache_control block.
func TestKindSeedFlagsExist(t *testing.T) {
	root := repoRoot(t)

	seed, err := os.ReadFile(filepath.Join(root, "cmd", "mk", "kind_seed.go"))
	if err != nil {
		t.Fatalf("read cmd/mk/kind_seed.go: %v", err)
	}
	const marker = "func kindCatalogModelArgs("
	start := strings.Index(string(seed), marker)
	if start < 0 {
		t.Fatalf("cmd/mk/kind_seed.go has no %q; this test can no longer find the command line", marker)
	}
	end := strings.Index(string(seed)[start:], "\n}\n")
	if end < 0 {
		t.Fatal("cmd/mk/kind_seed.go: kindCatalogModelArgs does not close where expected")
	}

	admin, err := os.ReadFile(filepath.Join(root, "internal", "bootstrap", "model_admin.go"))
	if err != nil {
		t.Fatalf("read internal/bootstrap/model_admin.go: %v", err)
	}
	defined := map[string]bool{}
	for _, m := range modelAddFlagRe.FindAllStringSubmatch(string(admin), -1) {
		defined["--"+m[1]] = true
	}
	if len(defined) < 5 {
		t.Fatalf("found only %d flags in model_admin.go; the flag set's shape changed", len(defined))
	}

	emitted := map[string]bool{}
	for _, m := range kindSeedFlagRe.FindAllStringSubmatch(string(seed)[start:start+end], -1) {
		emitted[m[1]] = true
		if !defined[m[1]] {
			t.Errorf("cmd/mk passes %s to `model add`, which does not define it", m[1])
		}
	}
	if len(emitted) < 5 {
		t.Fatalf("found only %d flags in kindCatalogModelArgs; this test is no longer reading the command line", len(emitted))
	}
}
