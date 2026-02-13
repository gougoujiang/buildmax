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

// Grep is a tool that searches file contents by regex pattern under a root directory.
// It implements the agent.Tool interface.
type Grep struct {
	root string // absolute path; all resolved paths must be under this
}

// NewGrep creates a Grep tool that searches file contents under root.
// If root is empty, the current working directory is used.
// root is normalized and absolutized; an error is returned if it cannot be resolved.
func NewGrep(root string) (*Grep, error) {
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Grep{root: filepath.Clean(abs)}, nil
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
	pattern, err := parseGrepPattern(args)
	if err != nil {
		return "", err
	}

	// Parse optional flags
	caseInsensitive := parseBool(args, "case_insensitive", false)
	multiline := parseBool(args, "multiline", false)
	lineNumbers := parseBool(args, "line_numbers", true)
	outputMode := parseString(args, "output_mode", "files_with_matches")
	globFilter := parseString(args, "glob", "")
	typeFilter := parseString(args, "type", "")
	beforeCtx := parseInt(args, "before_context", 0)
	afterCtx := parseInt(args, "after_context", 0)
	ctxBoth := parseInt(args, "context", 0)
	headLimit := parseInt(args, "head_limit", 0)
	offset := parseInt(args, "offset", 0)

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

	joined := filepath.Join(g.root, s)
	resolved, err = filepath.Abs(filepath.Clean(joined))
	if err != nil {
		return "", false, err
	}
	rel, err := filepath.Rel(g.root, resolved)
	if err != nil {
		return "", false, errors.New("path outside allowed root")
	}
	if rel == ".." || strings.HasPrefix(rel, "..") {
		return "", false, errors.New("path outside allowed root")
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

// formatFilesWithMatches returns one absolute path per line for files with matches.
func formatFilesWithMatches(results []fileResult, offset, limit int) string {
	if len(results) == 0 {
		return "No matches found."
	}

	// Apply offset
	start := offset
	if start > len(results) {
		start = len(results)
	}
	items := results[start:]

	// Apply limit
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}

	if len(items) == 0 {
		return "No matches found."
	}

	var sb strings.Builder
	for i, r := range items {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(r.path)
	}
	return sb.String()
}

// formatCount returns "filepath: N" per line for files with matches.
func formatCount(results []fileResult, offset, limit int) string {
	if len(results) == 0 {
		return "No matches found."
	}

	// Apply offset
	start := offset
	if start > len(results) {
		start = len(results)
	}
	items := results[start:]

	// Apply limit
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}

	if len(items) == 0 {
		return "No matches found."
	}

	var sb strings.Builder
	for i, r := range items {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(fmt.Sprintf("%s: %d", r.path, len(r.matches)))
	}
	return sb.String()
}

// formatContent returns ripgrep-style grouped output with optional context lines.
// offset and limit apply to match entries (not output lines).
func formatContent(results []fileResult, before, after int, showLineNumbers bool, offset, limit int) string {
	if len(results) == 0 {
		return "No matches found."
	}

	// First, flatten all match entries across files to apply offset/limit.
	type indexedMatch struct {
		fileIdx  int
		matchIdx int
	}
	var allMatches []indexedMatch
	for fi, r := range results {
		for mi := range r.matches {
			allMatches = append(allMatches, indexedMatch{fileIdx: fi, matchIdx: mi})
		}
	}

	// Apply offset
	start := offset
	if start > len(allMatches) {
		start = len(allMatches)
	}
	visible := allMatches[start:]

	// Apply limit
	if limit > 0 && limit < len(visible) {
		visible = visible[:limit]
	}

	if len(visible) == 0 {
		return "No matches found."
	}

	// Group visible matches by file index, preserving order
	type fileGroup struct {
		fileIdx    int
		matchLines map[int]bool // set of 1-based line numbers that are actual matches
	}
	var groups []fileGroup
	groupMap := make(map[int]int) // fileIdx → index in groups

	for _, vm := range visible {
		idx, ok := groupMap[vm.fileIdx]
		if !ok {
			idx = len(groups)
			groupMap[vm.fileIdx] = idx
			groups = append(groups, fileGroup{
				fileIdx:    vm.fileIdx,
				matchLines: make(map[int]bool),
			})
		}
		lineNum := results[vm.fileIdx].matches[vm.matchIdx].lineNum
		groups[idx].matchLines[lineNum] = true
	}

	var sb strings.Builder
	for gi, grp := range groups {
		if gi > 0 {
			sb.WriteByte('\n')
		}
		fr := results[grp.fileIdx]
		totalLines := len(fr.lines)

		// Compute display ranges from the visible match lines + context
		matchNums := make([]int, 0, len(grp.matchLines))
		for ln := range grp.matchLines {
			matchNums = append(matchNums, ln)
		}
		sort.Ints(matchNums)

		// Build ranges [start, end] (1-based, inclusive)
		var ranges []lineRange
		for _, m := range matchNums {
			s := m - before
			if s < 1 {
				s = 1
			}
			e := m + after
			if e > totalLines {
				e = totalLines
			}
			ranges = append(ranges, lineRange{s, e})
		}

		// Merge overlapping or adjacent ranges
		merged := mergeRanges(ranges)

		// Emit file header
		sb.WriteString(fr.path)
		sb.WriteByte('\n')

		for ri, rng := range merged {
			if ri > 0 {
				sb.WriteString("--\n")
			}
			for ln := rng.start; ln <= rng.end; ln++ {
				lineText := ""
				if ln >= 1 && ln <= totalLines {
					lineText = fr.lines[ln-1]
				}
				isMatch := grp.matchLines[ln]
				sep := "-"
				if isMatch {
					sep = ":"
				}
				if showLineNumbers {
					sb.WriteString(fmt.Sprintf("%d%s%s\n", ln, sep, lineText))
				} else {
					sb.WriteString(fmt.Sprintf("%s%s\n", sep, lineText))
				}
			}
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// lineRange represents a range of 1-based line numbers [start, end] (inclusive).
type lineRange struct {
	start, end int
}

// mergeRanges merges overlapping or adjacent ranges.
func mergeRanges(ranges []lineRange) []lineRange {
	if len(ranges) == 0 {
		return nil
	}
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].start < ranges[j].start
	})
	merged := []lineRange{ranges[0]}
	for _, r := range ranges[1:] {
		last := &merged[len(merged)-1]
		if r.start <= last.end+1 {
			// Overlapping or adjacent: extend
			if r.end > last.end {
				last.end = r.end
			}
		} else {
			merged = append(merged, r)
		}
	}
	return merged
}

// --- Arg parsing helpers ---

// parseGrepPattern extracts and validates the required pattern argument.
func parseGrepPattern(args map[string]any) (string, error) {
	v, ok := args["pattern"]
	if !ok {
		return "", errors.New("missing pattern")
	}
	s, ok := v.(string)
	if !ok {
		return "", errors.New("pattern must be a string")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("pattern is empty")
	}
	return s, nil
}

// parseString extracts an optional string argument with a default.
func parseString(args map[string]any, key, defaultVal string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok {
		return defaultVal
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultVal
	}
	return s
}

// parseBool extracts an optional boolean argument with a default.
func parseBool(args map[string]any, key string, defaultVal bool) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	b, ok := v.(bool)
	if !ok {
		return defaultVal
	}
	return b
}

// parseInt extracts an optional integer argument with a default.
// JSON numbers arrive as float64.
func parseInt(args map[string]any, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	switch x := v.(type) {
	case float64:
		if x >= 0 {
			return int(x)
		}
	case int:
		if x >= 0 {
			return x
		}
	case int64:
		if x >= 0 {
			return int(x)
		}
	}
	return defaultVal
}
