package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFrontendsDedupeReact guards every frontend that consumes @buildmax/gui
// against bundling a second copy of React.
//
// gui is a symlinked file: dependency that externalises react, so its bare
// `import "react"` resolves from gui's own real path — where npm has installed
// react as a peer. A bundle built without resolve.dedupe then carries two React
// instances, the hook dispatcher of the one that rendered is null, and the app
// opens on a blank window. Desktop shipped exactly that.
func TestFrontendsDedupeReact(t *testing.T) {
	root := moduleRoot(t)

	frontends := []string{
		filepath.Join(root, "portal"),
		filepath.Join(root, "desktop", "frontend"),
	}
	for _, dir := range frontends {
		pkg, err := os.ReadFile(filepath.Join(dir, "package.json"))
		if err != nil {
			t.Fatalf("read package.json in %s: %v", rel(root, dir), err)
		}
		if !strings.Contains(string(pkg), "@buildmax/gui") {
			continue
		}
		config := viteConfigPath(t, dir)
		source, err := os.ReadFile(config)
		if err != nil {
			t.Fatalf("read %s: %v", rel(root, config), err)
		}
		deduped, ok := dedupeList(string(source))
		if !ok {
			t.Errorf("%s has no resolve.dedupe list (see portal/vite.config.js)", rel(root, config))
			continue
		}
		for _, specifier := range []string{"'react'", "'react-dom'", "'react/jsx-runtime'"} {
			if !strings.Contains(deduped, specifier) {
				t.Errorf("%s does not dedupe %s; a second React instance makes every hook throw and the app render nothing", rel(root, config), specifier)
			}
		}
	}
}

func viteConfigPath(t *testing.T, dir string) string {
	t.Helper()
	for _, name := range []string{"vite.config.js", "vite.config.ts", "vite.config.mjs"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	t.Fatalf("no vite config in %s", dir)
	return ""
}

// dedupeList returns the contents of the resolve.dedupe array, so a specifier
// named only in a comment does not pass for one that is actually deduped.
func dedupeList(source string) (string, bool) {
	start := strings.Index(source, "dedupe:")
	if start < 0 {
		return "", false
	}
	rest := source[start:]
	end := strings.Index(rest, "]")
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}
