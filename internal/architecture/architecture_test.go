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
}{
	{
		name: "core stays independent of adapters",
		dir:  "internal/core",
		forbidden: []string{
			"buildmax/internal/bootstrap",
			"buildmax/internal/config",
			"buildmax/internal/execution",
			"buildmax/internal/infra",
			"buildmax/internal/interface",
			"buildmax/internal/server",
		},
	},
	{
		name: "infra does not depend on upper layers",
		dir:  "internal/infra",
		forbidden: []string{
			"buildmax/internal/bootstrap",
			"buildmax/internal/execution",
			"buildmax/internal/interface",
			"buildmax/internal/server",
		},
	},
	{
		name: "execution does not depend on presentation or process bootstrap",
		dir:  "internal/execution",
		forbidden: []string{
			"buildmax/internal/bootstrap",
			"buildmax/internal/interface",
			"buildmax/internal/server",
		},
	},
	{
		name: "server does not depend on local interfaces or process bootstrap",
		dir:  "internal/server",
		forbidden: []string{
			"buildmax/internal/bootstrap",
			"buildmax/internal/config",
			"buildmax/internal/interface",
		},
	},
}

func TestInternalLayerImports(t *testing.T) {
	root := moduleRoot(t)
	for _, rule := range importRules {
		t.Run(rule.name, func(t *testing.T) {
			files := goFiles(t, filepath.Join(root, rule.dir))
			for _, path := range files {
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
