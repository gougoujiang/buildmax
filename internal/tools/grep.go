// Package tools provides concrete agent tools (e.g. read_file, glob, grep).
package tools

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

	"buildmax/internal/util"

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

// Grep is a tool that searches file contents by regex pattern under a workspace root.
// It implements the agent.Tool interface.
type Grep struct {
	ws *util.Workspace
}

// NewGrep creates a Grep tool that searches file contents under the given workspace.
func NewGrep(ws *util.Workspace) *Grep {
	return &Grep{ws: ws}
}

// Name returns the tool name for the LLM.
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

// Execute searches file contents under the tool's root for the given regex pattern.
// Returns formatted results based on output_mode, or "No matches found.", or an error.
func (g *Grep) Execute(ctx context.Context, args map[string]any) (string, error) {
	// Parse pattern (required)
	pattern, err := util.ParseRequiredString(args, "pattern")
	if err != nil {
		return "", err
	}

	// Parse optional flags
	caseInsensitive := util.ParseOptionalBool(args, "case_insensitive", false)
	multiline := util.ParseOptionalBool(args, "multiline", false)
	lineNumbers := util.ParseOptionalBool(args, "line_numbers", true)
	outputMode := util.ParseOptionalString(args, "output_mode", "files_with_matches")
	globFilter := util.ParseOptionalString(args, "glob", "")
	typeFilter := util.ParseOptionalString(args, "type", "")
	beforeCtx := util.ParseOptionalInt(args, "before_context", 0)
	afterCtx := util.ParseOptionalInt(args, "after_context", 0)
	ctxBoth := util.ParseOptionalInt(args, "context", 0)
	headLimit := util.ParseOptionalInt(args, "head_limit", 0)
	offset := util.ParseOptionalInt(args, "offset", 0)

	// context overrides before_context and after_context if set
	if ctxBoth > 0 {
		beforeCtx = ctxBoth
		afterCtx = ctxBoth
	}

	// Compile regex
	re, err := g.compilePattern(pattern, caseInsensitive, multiline)
	if err != nil {
		return "", err
	}

	// Resolve path
	searchPath, isFile, err := g.resolvePath(args)
	if err != nil {
		return "", err
	}

	// Collect candidate files
	files, err := g.collectFiles(searchPath, isFile, globFilter, typeFilter)
	if err != nil {
		return "", err
	}

	// Search each file
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

	// Format output based on mode
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
		return g.ws.Root, false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", false, errors.New("path must be a string")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return g.ws.Root, false, nil
	}

	resolved, err = g.ws.ResolvePath(s)
	if err != nil {
		return "", false, err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, errors.New("file not found")
		}
		if os.IsPermission(err) {
			return "", false, errors.New("permission denied")
		}
		return "", false, err
	}
	return resolved, !info.IsDir(), nil
}

// collectFiles returns the list of files to search, applying glob and type filters.
// If isFile is true, searchPath is a single file; otherwise it is a directory to walk.
func (g *Grep) collectFiles(searchPath string, isFile bool, globPattern, typeFilter string) ([]string, error) {
	if isFile {
		// Single file: check filters
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

	// Walk directory
	var files []string
	err := filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsPermission(err) {
				return nil // skip permission-denied
			}
			return nil // skip other walk errors
		}
		if d.IsDir() {
			return nil
		}

		// Apply glob filter against path relative to search root
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

		// Apply type filter
		if typeFilter != "" && !matchesType(path, typeFilter) {
			return nil
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort alphabetically for deterministic output
	sort.Strings(files)
	return files, nil
}

// matchesType checks if the file has an extension matching the given type shorthand.
func matchesType(filePath, typeFilter string) bool {
	exts, ok := typeExtensions[typeFilter]
	if !ok {
		return false // unknown type filter: no match
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
// If multiline is false, each line is tested individually.
// If multiline is true, the full content is searched and byte offsets are mapped to line numbers.
func (g *Grep) searchFile(filePath string, re *regexp.Regexp, multiline bool) (*fileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	// Remove trailing empty line artifact from Split if the file ends with \n
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	fr := &fileResult{
		path:  filePath,
		lines: lines,
	}

	if !multiline {
		// Line-by-line matching
		for i, line := range lines {
			if re.MatchString(line) {
				fr.matches = append(fr.matches, fileMatch{
					lineNum: i + 1, // 1-based
					text:    line,
				})
			}
		}
	} else {
		// Multiline: find all matches in full content, map byte offsets to line numbers
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

		// Convert matched line numbers to sorted fileMatch entries
		sortedNums := make([]int, 0, len(matchedLines))
		for ln := range matchedLines {
			sortedNums = append(sortedNums, ln)
		}
		sort.Ints(sortedNums)
		for _, ln := range sortedNums {
			if ln >= 1 && ln <= len(lines) {
				fr.matches = append(fr.matches, fileMatch{
					lineNum: ln,
					text:    lines[ln-1],
				})
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


