package tool

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/llm"

	"github.com/bmatcuk/doublestar/v4"
)

// typeExtensions maps type shorthand names to file extensions.
var typeExtensions = map[string][]string{
	"go":   {".go"},
	"js":   {".js"},
	"ts":   {".ts"},
	"tsx":  {".tsx"},
	"py":   {".py"},
	"java": {".java"},
	"rust": {".rs"},
	"c":    {".c"},
	"cpp":  {".cpp", ".cc", ".cxx"},
	"h":    {".h"},
	"css":  {".css"},
	"html": {".html", ".htm"},
	"json": {".json"},
	"yaml": {".yaml", ".yml"},
	"md":   {".md"},
	"sh":   {".sh"},
	"sql":  {".sql"},
	"xml":  {".xml"},
	"toml": {".toml"},
	"rb":   {".rb"},
}

// fileMatch represents one matching line in a file.
type fileMatch struct {
	lineNum int    // 1-based line number
	text    string // the line content
}

// fileResult holds all matches for one file, plus the full line slice for context retrieval.
type fileResult struct {
	path    string      // absolute path
	lines   []string    // all lines in the file (0-indexed)
	matches []fileMatch // matching lines
}

// Grep searches file contents by regex pattern under a workspace root.
type Grep struct {
	workspaceTool
}

// NewGrep creates a Grep tool that searches file contents under the given workspace root.
func NewGrep(workspaceRoot string) *Grep {
	return &Grep{workspaceTool{root: workspaceRoot}}
}

// Name returns the tool name for the LLM.
// Access implements llm.AccessDeclarer.
func (g *Grep) Access(_ map[string]any) llm.Access { return llm.AccessReadOnly }

func (g *Grep) Name() string { return ToolNameGrep }

// Description returns a short description so the LLM knows when to use this tool.
func (g *Grep) Description() string {
	return "Search file contents by regex pattern. Supports glob and file-type filters, three output modes (content with context lines, files_with_matches, count), case-insensitive and multiline flags. Use for finding code, strings, or patterns in files."
}

// Parameters returns the OpenAI-style JSON schema for the tool arguments.
func (g *Grep) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Regex pattern to search for in file contents",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File or directory to search in (equivalent to rg PATH). Defaults to workspace root.",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "Glob pattern to filter files (e.g. \"*.go\", \"*.{ts,tsx}\"). Equivalent to rg --glob.",
			},
			"type": map[string]any{
				"type":        "string",
				"description": "File type to search (e.g. go, js, py). Equivalent to rg --type. Common types: go, js, ts, py, java, rust, c, cpp, html, json, yaml, md.",
			},
			"output_mode": map[string]any{
				"type":        "string",
				"description": "Output mode: \"content\" shows matching lines with context, \"files_with_matches\" shows file paths only, \"count\" shows match counts per file. Defaults to \"files_with_matches\".",
				"enum":        []string{"content", "files_with_matches", "count"},
			},
			"before_context": map[string]any{
				"type":        "number",
				"description": "Lines to show before each match in content mode (equivalent to rg -B).",
			},
			"after_context": map[string]any{
				"type":        "number",
				"description": "Lines to show after each match in content mode (equivalent to rg -A).",
			},
			"context": map[string]any{
				"type":        "number",
				"description": "Lines to show before and after each match in content mode (equivalent to rg -C). Overrides before_context and after_context if set.",
			},
			"line_numbers": map[string]any{
				"type":        "boolean",
				"description": "Show line numbers in content mode output (equivalent to rg -n). Defaults to true.",
			},
			"case_insensitive": map[string]any{
				"type":        "boolean",
				"description": "Case-insensitive search (equivalent to rg -i).",
			},
			"multiline": map[string]any{
				"type":        "boolean",
				"description": "Multiline mode: . matches newlines, ^/$ match line boundaries (equivalent to rg -U --multiline-dotall).",
			},
			"head_limit": map[string]any{
				"type":        "number",
				"description": "Limit output entries. In content mode limits match entries; in files_with_matches/count limits files. 0 means unlimited.",
			},
			"offset": map[string]any{
				"type":        "number",
				"description": "Skip first N entries before applying head_limit (pagination).",
			},
		},
		"required": []string{"pattern"},
	}
}

// CheckArgs implements llm.ArgChecker. Grepping inside a sensitive file (credentials, private keys)
// triggers Ask so the user can confirm intent in interactive sessions.
func (g *Grep) CheckArgs(args map[string]any) llm.ToolAction {
	if path, ok := args["path"].(string); ok && isSensitivePath(path) {
		return llm.ToolActionAsk
	}
	return llm.ToolActionAllow
}

// Execute searches file contents under the tool's root for the given regex pattern.
// Returns formatted results based on output_mode, or "No matches found.", or an error.
func (g *Grep) Execute(ctx context.Context, args map[string]any) (string, error) {
	pattern, err := parseRequiredString(args, "pattern")
	if err != nil {
		return "", err
	}

	caseInsensitive := parseOptionalBool(args, "case_insensitive", false)
	multiline := parseOptionalBool(args, "multiline", false)
	lineNumbers := parseOptionalBool(args, "line_numbers", true)
	outputMode := parseOptionalString(args, "output_mode", "files_with_matches")
	globFilter := parseOptionalString(args, "glob", "")
	typeFilter := parseOptionalString(args, "type", "")
	beforeCtx := parseOptionalInt(args, "before_context", 0)
	afterCtx := parseOptionalInt(args, "after_context", 0)
	ctxBoth := parseOptionalInt(args, "context", 0)
	headLimit := parseOptionalInt(args, "head_limit", 0)
	offset := parseOptionalInt(args, "offset", 0)

	if ctxBoth > 0 {
		beforeCtx = ctxBoth
		afterCtx = ctxBoth
	}

	re, err := g.compilePattern(pattern, caseInsensitive, multiline)
	if err != nil {
		return "", err
	}

	searchPath, isFile, err := g.resolvePath(args)
	if err != nil {
		return "", err
	}

	files, err := g.collectFiles(searchPath, isFile, globFilter, typeFilter)
	if err != nil {
		return "", err
	}

	var results []fileResult
	for _, filePath := range files {
		fr, err := g.searchFile(filePath, re, multiline)
		if err != nil {
			slog.Debug("grep: skip file on read error", "path", filePath, "err", err)
			continue
		}
		if len(fr.matches) > 0 {
			results = append(results, *fr)
		}
	}

	if len(results) == 0 {
		return "No matches found.", nil
	}

	switch outputMode {
	case "content":
		return formatContent(results, beforeCtx, afterCtx, lineNumbers, offset, headLimit), nil
	case "count":
		return formatCount(results, offset, headLimit), nil
	default: // "files_with_matches"
		return formatFilesWithMatches(results, offset, headLimit), nil
	}
}

// compilePattern compiles the regex pattern with optional flags.
func (g *Grep) compilePattern(pattern string, caseInsensitive, multiline bool) (*regexp.Regexp, error) {
	prefix := ""
	if multiline {
		prefix += "(?ms)"
	}
	if caseInsensitive {
		prefix += "(?i)"
	}
	re, err := regexp.Compile(prefix + pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %v", err)
	}
	return re, nil
}

// resolvePath parses the optional path argument and resolves it under root.
// Returns the resolved path, whether it is a file (vs directory), and any error.
func (g *Grep) resolvePath(args map[string]any) (resolved string, isFile bool, err error) {
	v, ok := args["path"]
	if !ok || v == nil {
		return g.root, false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", false, errors.New("path must be a string")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return g.root, false, nil
	}
	return g.resolveAnyPath(s)
}

// collectFiles returns the list of files to search, applying glob and type filters.
// If isFile is true, searchPath is a single file; otherwise it is a directory to walk.
func (g *Grep) collectFiles(searchPath string, isFile bool, globPattern, typeFilter string) ([]string, error) {
	if isFile {
		if globPattern != "" {
			relPath := filepath.Base(searchPath)
			matched, err := doublestar.Match(filepath.ToSlash(globPattern), relPath)
			if err != nil {
				return nil, fmt.Errorf("invalid glob filter: %v", err)
			}
			if !matched {
				return nil, nil
			}
		}
		if typeFilter != "" && !matchesType(searchPath, typeFilter) {
			return nil, nil
		}
		return []string{searchPath}, nil
	}

	var files []string
	err := filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip walk errors (permission denied etc.)
		}
		if d.IsDir() {
			return nil
		}

		if globPattern != "" {
			relPath, relErr := filepath.Rel(searchPath, path)
			if relErr != nil {
				return nil
			}
			matched, matchErr := doublestar.Match(filepath.ToSlash(globPattern), filepath.ToSlash(relPath))
			if matchErr != nil || !matched {
				return nil
			}
		}

		if typeFilter != "" && !matchesType(path, typeFilter) {
			return nil
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

// matchesType checks if the file has an extension matching the given type shorthand.
func matchesType(filePath, typeFilter string) bool {
	exts, ok := typeExtensions[typeFilter]
	if !ok {
		return false
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	for _, e := range exts {
		if ext == e {
			return true
		}
	}
	return false
}

// searchFile reads a file and finds all lines matching the regex.
func (g *Grep) searchFile(filePath string, re *regexp.Regexp, multiline bool) (*fileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	fr := &fileResult{path: filePath, lines: lines}

	if !multiline {
		for i, line := range lines {
			if re.MatchString(line) {
				fr.matches = append(fr.matches, fileMatch{lineNum: i + 1, text: line})
			}
		}
	} else {
		matchedLines := make(map[int]bool)
		locs := re.FindAllStringIndex(content, -1)
		for _, loc := range locs {
			startLine := byteOffsetToLine(content, loc[0])
			endLine := byteOffsetToLine(content, loc[1]-1)
			if loc[1] == 0 {
				endLine = startLine
			}
			for ln := startLine; ln <= endLine; ln++ {
				matchedLines[ln] = true
			}
		}

		sortedNums := make([]int, 0, len(matchedLines))
		for ln := range matchedLines {
			sortedNums = append(sortedNums, ln)
		}
		sort.Ints(sortedNums)
		for _, ln := range sortedNums {
			if ln >= 1 && ln <= len(lines) {
				fr.matches = append(fr.matches, fileMatch{lineNum: ln, text: lines[ln-1]})
			}
		}
	}

	return fr, nil
}

// byteOffsetToLine converts a byte offset in content to a 1-based line number.
func byteOffsetToLine(content string, offset int) int {
	if offset < 0 {
		return 1
	}
	if offset >= len(content) {
		offset = len(content) - 1
	}
	line := 1
	for i := 0; i < offset; i++ {
		if content[i] == '\n' {
			line++
		}
	}
	return line
}
