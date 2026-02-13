package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- NewGrep ---

func TestNewGrep(t *testing.T) {
	t.Run("empty root uses cwd", func(t *testing.T) {
		g, err := NewGrep("")
		if err != nil {
			t.Fatalf("NewGrep(\"\"): %v", err)
		}
		if g.root == "" {
			t.Error("root should not be empty")
		}
	})
	t.Run("root is normalized", func(t *testing.T) {
		dir := t.TempDir()
		g, err := NewGrep(dir)
		if err != nil {
			t.Fatalf("NewGrep: %v", err)
		}
		abs, _ := filepath.Abs(dir)
		if g.root != filepath.Clean(abs) {
			t.Errorf("root = %q, want %q", g.root, filepath.Clean(abs))
		}
	})
}

// --- Name, Description, Parameters ---

func TestGrep_Name_Description_Parameters(t *testing.T) {
	dir := t.TempDir()
	g, err := NewGrep(dir)
	if err != nil {
		t.Fatal(err)
	}
	if g.Name() != ToolNameGrep {
		t.Errorf("Name() = %q, want Grep", g.Name())
	}
	if g.Description() == "" {
		t.Error("Description() should not be empty")
	}
	params := g.Parameters()
	if params == nil {
		t.Fatal("Parameters() should not be nil")
	}
	m, ok := params.(map[string]any)
	if !ok {
		t.Fatalf("Parameters() type = %T, want map[string]any", params)
	}
	if m["type"] != "object" {
		t.Errorf("parameters type = %v, want object", m["type"])
	}
	req, ok := m["required"].([]string)
	if !ok || len(req) != 1 || req[0] != "pattern" {
		t.Errorf("required = %v, want [pattern]", m["required"])
	}
	props, ok := m["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties missing or wrong type")
	}
	expectedProps := []string{
		"pattern", "path", "glob", "type", "output_mode",
		"before_context", "after_context", "context",
		"line_numbers", "case_insensitive", "multiline",
		"head_limit", "offset",
	}
	for _, key := range expectedProps {
		if _, ok := props[key]; !ok {
			t.Errorf("parameters missing property %q", key)
		}
	}
}

// --- Pattern validation ---

func TestGrep_Execute_missingPattern(t *testing.T) {
	dir := t.TempDir()
	g, _ := NewGrep(dir)
	_, err := g.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing pattern")
	}
	if !strings.Contains(err.Error(), "pattern") {
		t.Errorf("error = %v, want mention of pattern", err)
	}
}

func TestGrep_Execute_emptyPattern(t *testing.T) {
	dir := t.TempDir()
	g, _ := NewGrep(dir)
	_, err := g.Execute(context.Background(), map[string]any{"pattern": "   "})
	if err == nil {
		t.Fatal("expected error for empty pattern")
	}
	if !strings.Contains(err.Error(), "pattern") {
		t.Errorf("error = %v, want mention of pattern", err)
	}
}

func TestGrep_Execute_invalidRegex(t *testing.T) {
	dir := t.TempDir()
	g, _ := NewGrep(dir)
	_, err := g.Execute(context.Background(), map[string]any{"pattern": "[invalid"})
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
	if !strings.Contains(err.Error(), "invalid regex") {
		t.Errorf("error = %v, want mention of invalid regex", err)
	}
}

// --- Path validation ---

func TestGrep_Execute_pathOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	g, _ := NewGrep(dir)
	_, err := g.Execute(context.Background(), map[string]any{"pattern": "x", "path": ".."})
	if err == nil {
		t.Fatal("expected error for path outside root")
	}
	if !strings.Contains(err.Error(), "outside") {
		t.Errorf("error = %v, want mention of outside", err)
	}
}

func TestGrep_Execute_pathNotFound(t *testing.T) {
	dir := t.TempDir()
	g, _ := NewGrep(dir)
	_, err := g.Execute(context.Background(), map[string]any{"pattern": "x", "path": "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want mention of not found", err)
	}
}

func TestGrep_Execute_pathIsFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "hello.go")
	os.WriteFile(f, []byte("hello world\nfoo bar\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"path":    "hello.go",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, f) {
		t.Errorf("result should contain file path %q, got %q", f, result)
	}
}

func TestGrep_Execute_pathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "a.txt"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello\n"), 0644)

	g, _ := NewGrep(dir)
	// Search only sub directory
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"path":    "sub",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "a.txt") {
		t.Errorf("result should contain a.txt, got %q", result)
	}
	if strings.Contains(result, "b.txt") {
		t.Errorf("result should NOT contain b.txt when path=sub, got %q", result)
	}
}

func TestGrep_Execute_omittedPathSearchesRoot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "root.txt"), []byte("findme\n"), 0644)
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "nested.txt"), []byte("findme\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{"pattern": "findme"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "root.txt") || !strings.Contains(result, "nested.txt") {
		t.Errorf("result should contain both root.txt and nested.txt, got %q", result)
	}
}

// --- Output modes ---

func TestGrep_Execute_filesWithMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("func main() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("func test() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("no match here\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":     "func",
		"output_mode": "files_with_matches",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 file paths, got %d: %q", len(lines), result)
	}
	if !strings.Contains(result, "a.go") || !strings.Contains(result, "b.go") {
		t.Errorf("result should contain a.go and b.go, got %q", result)
	}
	if strings.Contains(result, "c.txt") {
		t.Error("result should not contain c.txt")
	}
}

func TestGrep_Execute_countMode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("line1\nline2\nline3\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("line1\nno match\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":     "line",
		"output_mode": "count",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// a.go has 3 matches, b.go has 1 match
	if !strings.Contains(result, "a.go: 3") {
		t.Errorf("result should contain 'a.go: 3', got %q", result)
	}
	if !strings.Contains(result, "b.go: 1") {
		t.Errorf("result should contain 'b.go: 1', got %q", result)
	}
}

func TestGrep_Execute_contentMode(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.go")
	os.WriteFile(f, []byte("alpha\nbeta\ngamma\ndelta\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":     "beta",
		"output_mode": "content",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Should contain file header and matched line with line number
	if !strings.Contains(result, "test.go") {
		t.Errorf("result should contain file name, got %q", result)
	}
	if !strings.Contains(result, "2:beta") {
		t.Errorf("result should contain '2:beta', got %q", result)
	}
}

// --- Filters ---

func TestGrep_Execute_globFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"glob":    "*.go",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "a.go") {
		t.Errorf("result should contain a.go, got %q", result)
	}
	if strings.Contains(result, "b.txt") {
		t.Errorf("result should NOT contain b.txt, got %q", result)
	}
}

func TestGrep_Execute_typeFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.py"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("hello\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"type":    "go",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "a.go") {
		t.Errorf("result should contain a.go, got %q", result)
	}
	if strings.Contains(result, "b.py") || strings.Contains(result, "c.txt") {
		t.Errorf("result should NOT contain b.py or c.txt, got %q", result)
	}
}

func TestGrep_Execute_globAndTypeFilter(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "a.go"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(sub, "b.go"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "c.go"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(sub, "d.py"), []byte("hello\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"glob":    "src/*.go",
		"type":    "go",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Only src/a.go and src/b.go should match (glob restricts to src/, type restricts to .go)
	if !strings.Contains(result, "a.go") || !strings.Contains(result, "b.go") {
		t.Errorf("result should contain a.go and b.go, got %q", result)
	}
	if strings.Contains(result, "c.go") {
		t.Errorf("result should NOT contain c.go (not in src/), got %q", result)
	}
	if strings.Contains(result, "d.py") {
		t.Errorf("result should NOT contain d.py, got %q", result)
	}
}

// --- Context lines ---

func TestGrep_Execute_beforeContext(t *testing.T) {
	dir := t.TempDir()
	content := "line1\nline2\nline3\ntarget\nline5\n"
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":        "target",
		"output_mode":    "content",
		"before_context": float64(2),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "2-line2") {
		t.Errorf("result should contain context line '2-line2', got %q", result)
	}
	if !strings.Contains(result, "3-line3") {
		t.Errorf("result should contain context line '3-line3', got %q", result)
	}
	if !strings.Contains(result, "4:target") {
		t.Errorf("result should contain match line '4:target', got %q", result)
	}
}

func TestGrep_Execute_afterContext(t *testing.T) {
	dir := t.TempDir()
	content := "line1\ntarget\nline3\nline4\nline5\n"
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":       "target",
		"output_mode":   "content",
		"after_context": float64(2),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "2:target") {
		t.Errorf("result should contain match line '2:target', got %q", result)
	}
	if !strings.Contains(result, "3-line3") {
		t.Errorf("result should contain context line '3-line3', got %q", result)
	}
	if !strings.Contains(result, "4-line4") {
		t.Errorf("result should contain context line '4-line4', got %q", result)
	}
}

func TestGrep_Execute_contextOverlap(t *testing.T) {
	dir := t.TempDir()
	// Two matches close together: lines 2 and 4, with context=1
	content := "line1\nmatchA\nline3\nmatchB\nline5\n"
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":     "match",
		"output_mode": "content",
		"context":     float64(1),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// With overlap merging, lines 1-5 should be one continuous group (no "--" separator)
	if strings.Contains(result, "--") {
		t.Errorf("overlapping context should be merged (no -- separator), got %q", result)
	}
	// line3 should appear once as context
	if count := strings.Count(result, "line3"); count != 1 {
		t.Errorf("line3 should appear once, got %d times in %q", count, result)
	}
}

func TestGrep_Execute_contextSeparator(t *testing.T) {
	dir := t.TempDir()
	// Two matches far apart: lines 1 and 7, with context=0
	content := "matchA\nline2\nline3\nline4\nline5\nline6\nmatchB\n"
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":     "match",
		"output_mode": "content",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Non-adjacent match groups should be separated by "--"
	if !strings.Contains(result, "--") {
		t.Errorf("non-adjacent groups should be separated by --, got %q", result)
	}
}

// --- Flags ---

func TestGrep_Execute_caseInsensitive(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("Hello World\nhello world\nHELLO WORLD\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":          "hello",
		"output_mode":      "count",
		"case_insensitive": true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "3") {
		t.Errorf("case-insensitive search should match 3 lines, got %q", result)
	}
}

func TestGrep_Execute_lineNumbersFalse(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hello world\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":      "hello",
		"output_mode":  "content",
		"line_numbers": false,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Should NOT have "1:" prefix (line number)
	if strings.Contains(result, "1:") {
		t.Errorf("line_numbers=false should omit line numbers, got %q", result)
	}
	// Should still have the match with ":" prefix
	if !strings.Contains(result, ":hello world") {
		t.Errorf("result should contain ':hello world', got %q", result)
	}
}

func TestGrep_Execute_multiline(t *testing.T) {
	dir := t.TempDir()
	content := "start\nhello\nworld\nend\n"
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":     "hello\\nworld",
		"output_mode": "content",
		"multiline":   true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Should match lines 2 and 3
	if !strings.Contains(result, "2:hello") {
		t.Errorf("multiline match should include line 2, got %q", result)
	}
	if !strings.Contains(result, "3:world") {
		t.Errorf("multiline match should include line 3, got %q", result)
	}
}

// --- Pagination ---

func TestGrep_Execute_headLimit_filesWithMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("hello\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":    "hello",
		"head_limit": float64(2),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 2 {
		t.Errorf("head_limit=2 should return 2 files, got %d: %q", len(lines), result)
	}
}

func TestGrep_Execute_offset_filesWithMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("hello\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"offset":  float64(1),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 2 {
		t.Errorf("offset=1 with 3 files should return 2, got %d: %q", len(lines), result)
	}
	// First file (a.txt) should be skipped
	if strings.Contains(result, "a.txt") {
		t.Errorf("offset=1 should skip a.txt, got %q", result)
	}
}

func TestGrep_Execute_offsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "d.txt"), []byte("hello\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":    "hello",
		"offset":     float64(1),
		"head_limit": float64(2),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 2 {
		t.Errorf("offset=1 head_limit=2 should return 2 files, got %d: %q", len(lines), result)
	}
}

func TestGrep_Execute_headLimit_contentMode(t *testing.T) {
	dir := t.TempDir()
	content := "match1\nmatch2\nmatch3\nmatch4\n"
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":     "match",
		"output_mode": "content",
		"head_limit":  float64(2),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Should contain only first 2 match lines
	if !strings.Contains(result, "1:match1") || !strings.Contains(result, "2:match2") {
		t.Errorf("result should contain match1 and match2, got %q", result)
	}
	if strings.Contains(result, "3:match3") || strings.Contains(result, "4:match4") {
		t.Errorf("result should NOT contain match3 or match4, got %q", result)
	}
}

// --- No matches ---

func TestGrep_Execute_noMatches_allModes(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("nothing here\n"), 0644)

	modes := []string{"files_with_matches", "content", "count"}
	g, _ := NewGrep(dir)
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			result, err := g.Execute(context.Background(), map[string]any{
				"pattern":     "zzzznotfound",
				"output_mode": mode,
			})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if result != "No matches found." {
				t.Errorf("result = %q, want 'No matches found.'", result)
			}
		})
	}
}

// --- Default output mode is files_with_matches ---

func TestGrep_Execute_defaultOutputMode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("hello\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern": "hello",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Default is files_with_matches: should just contain the file path
	if !strings.Contains(result, "a.go") {
		t.Errorf("default mode should return file path, got %q", result)
	}
	// Should not contain line numbers (that's content mode)
	if strings.Contains(result, "1:") {
		t.Errorf("default mode should not have line numbers, got %q", result)
	}
}

// --- Count mode with head_limit ---

func TestGrep_Execute_headLimit_countMode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x\n"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("x\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":     "x",
		"output_mode": "count",
		"head_limit":  float64(1),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 1 {
		t.Errorf("head_limit=1 count mode should return 1 line, got %d: %q", len(lines), result)
	}
}

// --- Context with context param overriding before/after ---

func TestGrep_Execute_contextOverridesBeforeAfter(t *testing.T) {
	dir := t.TempDir()
	content := "line1\nline2\ntarget\nline4\nline5\n"
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":        "target",
		"output_mode":    "content",
		"before_context": float64(0),
		"after_context":  float64(0),
		"context":        float64(1),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// context=1 should override before=0 and after=0
	if !strings.Contains(result, "2-line2") {
		t.Errorf("result should contain context line '2-line2', got %q", result)
	}
	if !strings.Contains(result, "4-line4") {
		t.Errorf("result should contain context line '4-line4', got %q", result)
	}
}

// --- Multiple files in content mode ---

func TestGrep_Execute_contentMode_multipleFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello from a\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello from b\n"), 0644)

	g, _ := NewGrep(dir)
	result, err := g.Execute(context.Background(), map[string]any{
		"pattern":     "hello",
		"output_mode": "content",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Should contain both file headers
	if !strings.Contains(result, "a.txt") || !strings.Contains(result, "b.txt") {
		t.Errorf("content mode should show both files, got %q", result)
	}
}
