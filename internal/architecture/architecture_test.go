package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var importRules = []struct {
	name      string
	dir       string
	forbidden []string
	// except exempts directories, and everything under them, for a rule whose
	// dependency exactly one package legitimately owns.
	except []string
}{
	{
		name: "core stays independent of adapters",
		dir:  "internal/core",
		forbidden: []string{
			"github.com/gougoujiang/buildmax/internal/agentapp",
			"github.com/gougoujiang/buildmax/internal/bootstrap",
			"github.com/gougoujiang/buildmax/internal/config",
			"github.com/gougoujiang/buildmax/internal/infra",
			"github.com/gougoujiang/buildmax/internal/interface",
			"github.com/gougoujiang/buildmax/internal/server",
			"github.com/gougoujiang/buildmax/internal/service",
		},
	},
	{
		name: "infra does not depend on upper layers",
		dir:  "internal/infra",
		forbidden: []string{
			"github.com/gougoujiang/buildmax/internal/bootstrap",
			"github.com/gougoujiang/buildmax/internal/interface",
			"github.com/gougoujiang/buildmax/internal/server",
		},
	},
	{
		name: "server does not depend on local interfaces or process bootstrap",
		dir:  "internal/server",
		forbidden: []string{
			"github.com/gougoujiang/buildmax/internal/bootstrap",
			"github.com/gougoujiang/buildmax/internal/interface",
			"github.com/gougoujiang/buildmax/internal/config",
		},
	},
	{
		// agentapp is deliberately absent: service/conversation/runtime imports
		// it for NewNonInteractivePolicy, and that one call is the whole
		// dependency. Removable, but real — and a rule that fails on main
		// teaches contributors to skip the suite.
		name: "service is reached by transports, not the reverse",
		dir:  "internal/service",
		forbidden: []string{
			"github.com/gougoujiang/buildmax/internal/bootstrap",
			"github.com/gougoujiang/buildmax/internal/interface",
			"github.com/gougoujiang/buildmax/internal/server",
		},
	},
	{
		name: "agentapp does not depend on the surfaces that assemble it",
		dir:  "internal/agentapp",
		forbidden: []string{
			"github.com/gougoujiang/buildmax/internal/bootstrap",
			"github.com/gougoujiang/buildmax/internal/interface",
			"github.com/gougoujiang/buildmax/internal/server",
		},
	},
	{
		// Above this boundary, "no such row" is model.ErrNotFound.
		name: "gorm stays inside the db implementation",
		dir:  "internal",
		forbidden: []string{
			"gorm.io/gorm",
			"gorm.io/driver",
		},
		except: []string{"internal/infra/db"},
	},
}

func TestInternalLayerImports(t *testing.T) {
	root := moduleRoot(t)
	for _, rule := range importRules {
		t.Run(rule.name, func(t *testing.T) {
			files := goFiles(t, filepath.Join(root, rule.dir))
			for _, path := range files {
				if excluded(rel(root, path), rule.except) {
					continue
				}
				f := parseFile(t, path)
				for _, imp := range f.Imports {
					importPath, err := strconv.Unquote(imp.Path.Value)
					if err != nil {
						t.Fatalf("unquote import in %s: %v", path, err)
					}
					for _, forbidden := range rule.forbidden {
						if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
							t.Errorf("%s imports forbidden package %s", rel(root, path), importPath)
						}
					}
				}
			}
		})
	}
}

func TestNoInternalTypeAliases(t *testing.T) {
	root := moduleRoot(t)
	for _, path := range goFiles(t, filepath.Join(root, "internal")) {
		f := parseFile(t, path)
		ast.Inspect(f, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok || !spec.Assign.IsValid() {
				return true
			}
			t.Errorf("%s contains type alias %s; prefer direct imports across refactors", rel(root, path), spec.Name.Name)
			return true
		})
	}
}

// Reached from production code, each of these is either a shipped capability
// that should not exist or a real dependency wired to a fake.
var testOnlyPackages = []string{
	"github.com/gougoujiang/buildmax/internal/mock",
	"github.com/gougoujiang/buildmax/internal/testsupport",
}

// The rule covers cmd/ and deployment/ as well as internal/, because "must not
// ship" is a statement about the binaries, and those are where they are built.
//
// evaluation/ is listed even though it ships nothing. Its tests drive the real
// binary against a scripted model and so import mockllm legitimately — test
// files are skipped below — but its runner and adapters must reach a real
// subject. An evaluation that answered its own model would report on the mock.
var testOnlyImportTrees = []string{"internal", "cmd", "deployment", "evaluation"}

// deployment/smoke exists only to make a smoke deterministic and is never
// released, so it is where a test-only import is the point rather than a
// mistake: its mock model is a packaging of internal/testsupport/mockllm.
var testOnlyImportExempt = []string{"deployment/smoke"}

func TestTestOnlyPackagesStayInTests(t *testing.T) {
	root := moduleRoot(t)
	for _, tree := range testOnlyImportTrees {
		for _, path := range goFiles(t, filepath.Join(root, tree)) {
			if strings.HasSuffix(path, "_test.go") || excluded(rel(root, path), testOnlyImportExempt) {
				continue
			}
			f := parseFile(t, path)
			for _, imp := range f.Imports {
				importPath, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					t.Fatalf("unquote import in %s: %v", path, err)
				}
				for _, pkg := range testOnlyPackages {
					if importPath == pkg || strings.HasPrefix(importPath, pkg+"/") {
						t.Errorf("%s is not a test file but imports test-only package %s",
							rel(root, path), importPath)
					}
				}
			}
		}
	}
}

func excluded(relPath string, exempt []string) bool {
	for _, dir := range exempt {
		if relPath == dir || strings.HasPrefix(relPath, dir+"/") {
			return true
		}
	}
	return false
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func goFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			out = append(out, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func parseFile(t *testing.T, path string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return f
}

func rel(root, path string) string {
	r, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(r)
}
