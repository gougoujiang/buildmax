package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultTimeout = 5 * time.Second
	maxPatchRunes  = 30_000
)

type ChangeStatus string

const (
	StatusAdded    ChangeStatus = "added"
	StatusModified ChangeStatus = "modified"
	StatusDeleted  ChangeStatus = "deleted"
	StatusRenamed  ChangeStatus = "renamed"
)

type WorkspaceDiff struct {
	Workspace string        `json:"workspace"`
	Files     []ChangedFile `json:"files"`
	Error     string        `json:"error,omitempty"`
}

type ChangedFile struct {
	Path      string       `json:"path"`
	OldPath   string       `json:"old_path,omitempty"`
	Status    ChangeStatus `json:"status"`
	Additions int          `json:"additions"`
	Deletions int          `json:"deletions"`
	Patch     string       `json:"patch,omitempty"`
	Binary    bool         `json:"binary,omitempty"`
	Truncated bool         `json:"truncated,omitempty"`
}

func ReadWorkspace(ctx context.Context, workspace string) (WorkspaceDiff, error) {
	if strings.TrimSpace(workspace) == "" {
		return WorkspaceDiff{}, errors.New("workspace required")
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return WorkspaceDiff{}, err
	}
	if _, err := runGit(ctx, root, "rev-parse", "--is-inside-work-tree"); err != nil {
		return WorkspaceDiff{Workspace: root, Error: "not a git repository"}, nil
	}

	statusOut, err := runGit(ctx, root, "status", "--porcelain=v1")
	if err != nil {
		return WorkspaceDiff{}, err
	}
	files := parseStatus(statusOut)
	numstat := parseNumstat(mustRunGit(ctx, root, "diff", "--numstat", "HEAD", "--"))
	for i := range files {
		if counts, ok := numstat[files[i].Path]; ok {
			files[i].Additions = counts.additions
			files[i].Deletions = counts.deletions
		}
		patch, binary, truncated := readPatch(ctx, root, files[i])
		files[i].Patch = patch
		files[i].Binary = binary
		files[i].Truncated = truncated
	}
	return WorkspaceDiff{Workspace: root, Files: files}, nil
}

func readPatch(ctx context.Context, root string, f ChangedFile) (patch string, binary bool, truncated bool) {
	var out string
	if f.Status == StatusAdded && f.OldPath == "" && !isTrackedOrStagedPath(ctx, root, f.Path) {
		abs := filepath.Join(root, filepath.FromSlash(f.Path))
		out = mustRunGit(ctx, root, "diff", "--no-ext-diff", "--no-index", "--", os.DevNull, abs)
	} else {
		path := f.Path
		if f.Status == StatusDeleted && f.OldPath != "" {
			path = f.OldPath
		}
		out = mustRunGit(ctx, root, "diff", "--no-ext-diff", "HEAD", "--", path)
	}
	if strings.Contains(out, "Binary files ") || strings.Contains(out, "GIT binary patch") {
		binary = true
	}
	out, truncated = truncateRunes(out, maxPatchRunes)
	return out, binary, truncated
}

func isTrackedOrStagedPath(ctx context.Context, root, path string) bool {
	if _, err := runGit(ctx, root, "ls-files", "--error-unmatch", "--", path); err == nil {
		return true
	}
	return false
}

func parseStatus(out string) []ChangedFile {
	var files []ChangedFile
	for _, raw := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if len(raw) < 4 {
			continue
		}
		code := raw[:2]
		pathPart := strings.TrimSpace(raw[3:])
		if pathPart == "" {
			continue
		}
		f := ChangedFile{Path: pathPart, Status: statusFromCode(code)}
		if strings.Contains(pathPart, " -> ") {
			parts := strings.SplitN(pathPart, " -> ", 2)
			f.OldPath = strings.TrimSpace(parts[0])
			f.Path = strings.TrimSpace(parts[1])
			f.Status = StatusRenamed
		}
		files = append(files, f)
	}
	return files
}

func statusFromCode(code string) ChangeStatus {
	if strings.Contains(code, "R") {
		return StatusRenamed
	}
	if strings.Contains(code, "D") {
		return StatusDeleted
	}
	if strings.Contains(code, "A") || code == "??" {
		return StatusAdded
	}
	return StatusModified
}

type lineCounts struct {
	additions int
	deletions int
}

func parseNumstat(out string) map[string]lineCounts {
	counts := map[string]lineCounts{}
	for _, raw := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		fields := strings.Split(raw, "\t")
		if len(fields) < 3 {
			continue
		}
		add, addOK := parseCount(fields[0])
		del, delOK := parseCount(fields[1])
		if !addOK || !delOK {
			continue
		}
		path := fields[2]
		if strings.Contains(path, " => ") {
			path = normalizeRenamedNumstatPath(path)
		}
		counts[path] = lineCounts{additions: add, deletions: del}
	}
	return counts
}

func parseCount(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func normalizeRenamedNumstatPath(path string) string {
	if !strings.Contains(path, "{") || !strings.Contains(path, "}") {
		parts := strings.Split(path, " => ")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	start := strings.Index(path, "{")
	end := strings.Index(path, "}")
	if start < 0 || end < start {
		return path
	}
	inside := path[start+1 : end]
	parts := strings.Split(inside, " => ")
	if len(parts) != 2 {
		return path
	}
	return path[:start] + parts[1] + path[end+1:]
}

func runGit(ctx context.Context, root string, args ...string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "git", append([]string{"-C", root}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if runCtx.Err() == context.DeadlineExceeded {
		return out.String(), fmt.Errorf("git timed out")
	}
	if err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

func mustRunGit(ctx context.Context, root string, args ...string) string {
	out, _ := runGit(ctx, root, args...)
	return out
}

func truncateRunes(s string, max int) (string, bool) {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s, false
	}
	r := []rune(s)
	return string(r[:max]) + "\n... diff truncated ...\n", true
}
