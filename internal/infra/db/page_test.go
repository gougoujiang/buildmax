package db

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestClampPageAppliesADefaultAndACeiling(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		limit, offset         int
		wantLimit, wantOffset int
	}{
		{"no preference", 0, 0, defaultPageSize, 0},
		{"negative", -5, -5, defaultPageSize, 0},
		{"under the ceiling", 10, 20, 10, 20},
		{"over the ceiling", maxPageSize + 1, 0, maxPageSize, 0},
	} {
		gotLimit, gotOffset := clampPage(tc.limit, tc.offset)
		if gotLimit != tc.wantLimit || gotOffset != tc.wantOffset {
			t.Errorf("%s: clampPage(%d, %d) = %d, %d; want %d, %d",
				tc.name, tc.limit, tc.offset, gotLimit, gotOffset, tc.wantLimit, tc.wantOffset)
		}
	}
}

// capPage must not turn "everything" into a page: its callers ask for a whole
// set and would silently lose rows if it applied a default.
func TestCapPageBoundsWithoutPaging(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		limit, offset         int
		wantLimit, wantOffset int
	}{
		{"everything", 0, 0, maxUnboundedPage, 0},
		{"negative reads as everything", -1, 0, maxUnboundedPage, 0},
		{"a stated limit is kept", 25, 5, 25, 5},
		{"over the ceiling", maxUnboundedPage + 1, 0, maxUnboundedPage, 0},
		{"negative offset", 10, -3, 10, 0},
	} {
		gotLimit, gotOffset := capPage(tc.limit, tc.offset)
		if gotLimit != tc.wantLimit || gotOffset != tc.wantOffset {
			t.Errorf("%s: capPage(%d, %d) = %d, %d; want %d, %d",
				tc.name, tc.limit, tc.offset, gotLimit, gotOffset, tc.wantLimit, tc.wantOffset)
		}
	}
}

// A ceiling nobody applies is a ceiling that does not exist. Every store method
// taking a limit has to bound it, and this is cheaper to keep true than a
// reviewer noticing the twelfth one.
func TestEveryPagedQueryBoundsItsLimit(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			// The bounding functions are the answer, not a caller of it.
			if !ok || fn.Body == nil || !takesLimitOffset(fn) ||
				fn.Name.Name == "clampPage" || fn.Name.Name == "capPage" {
				continue
			}
			if !boundsItsLimit(fn) {
				t.Errorf("%s: %s takes limit and offset without calling clampPage or capPage",
					path, fn.Name.Name)
			}
		}
	}
}

func takesLimitOffset(fn *ast.FuncDecl) bool {
	var names []string
	for _, p := range fn.Type.Params.List {
		for _, n := range p.Names {
			names = append(names, n.Name)
		}
	}
	var hasLimit, hasOffset bool
	for _, n := range names {
		hasLimit = hasLimit || n == "limit"
		hasOffset = hasOffset || n == "offset"
	}
	return hasLimit && hasOffset
}

func boundsItsLimit(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := call.Fun.(*ast.Ident); ok && (id.Name == "clampPage" || id.Name == "capPage") {
			found = true
		}
		return true
	})
	return found
}
