package tool

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// Glob lists files matching a glob pattern under a workspace root.
// Symlinks: doublestar.Glob with WithFilesOnly does not follow symlinks to directories;
// symlinks to files are included as matches.
type Glob struct {
	workspaceTool
}

// NewGlob creates a Glob tool that searches for files under the given workspace root.
func NewGlob(workspaceRoot string) *Glob {
	return &Glob{workspaceTool{root: workspaceRoot}}
}

// Name returns the tool name for the LLM.
// Access implements llm.AccessDeclarer.
func (g *Glob) Access(_ map[string]any) llm.Access { return llm.AccessReadOnly }

func (g *Glob) Name() string { return ToolNameGlob }

// Description returns a short description so the LLM knows when to use this tool.
func (g *Glob) Description() string {
	return "Fast file pattern matching. Supports glob patterns like **/*.js or src/**/*.ts. Returns matching file paths sorted by modification time (newest first). Use to find files by name or extension. Prefer multiple parallel tool calls when useful; for open-ended search consider other strategies."
}

// Parameters returns the OpenAI-style JSON schema for the tool arguments.
func (g *Glob) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "The glob pattern to match files against (e.g. *.go, **/*.txt)",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to search in; if omitted, the tool's default root is used. Do not pass \"undefined\" or \"null\"—omit the field. Must be a valid path under the workspace if provided.",
			},
		},
		"required": []string{"pattern"},
	}
}

// Execute lists files under the search directory that match the pattern, sorted by modification time (newest first).
// Returns one absolute path per line, or "No files matched the pattern.", or an error for validation/system failures.
func (g *Glob) Execute(ctx context.Context, args map[string]any) (string, error) {
	pattern, err := parseRequiredString(args, "pattern")
	if err != nil {
		return "", err
	}
	searchDir, err := g.resolveSearchDir(args)
	if err != nil {
		return "", err
	}

	// doublestar.Glob expects pattern with / separator; Glob does not follow symlinks to dirs when using WithFilesOnly.
	patternSlash := filepath.ToSlash(pattern)
	fsys := os.DirFS(searchDir)
	matches, err := doublestar.Glob(fsys, patternSlash, doublestar.WithFilesOnly())
	if err != nil {
		if errors.Is(err, doublestar.ErrBadPattern) {
			return "", errors.New("invalid glob pattern")
		}
		return "", err
	}

	// Collect absolute paths and mod times for sorting (newest first).
	type entry struct {
		path string
		info fs.FileInfo
	}
	var entries []entry
	for _, rel := range matches {
		absPath := filepath.Join(searchDir, filepath.FromSlash(rel))
		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) || os.IsPermission(err) {
				continue
			}
			return "", err
		}
		if info.IsDir() {
			continue
		}
		entries = append(entries, entry{path: absPath, info: info})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].info.ModTime().After(entries[j].info.ModTime())
	})

	if len(entries) == 0 {
		return "No files matched the pattern.", nil
	}
	var sb strings.Builder
	for i, e := range entries {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(e.path)
	}
	return sb.String(), nil
}

// resolveSearchDir returns the directory to search: root, or root joined with optional path arg.
func (g *Glob) resolveSearchDir(args map[string]any) (string, error) {
	v, ok := args["path"]
	if !ok || v == nil {
		return g.root, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", errors.New("path must be a string")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return g.root, nil
	}
	return g.resolveDirPath(s)
}
