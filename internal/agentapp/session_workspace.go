package agentapp

import "path/filepath"

// workspaceAliases returns the spellings of one workspace path that should be
// treated as the same directory. A session records whatever root it ran under,
// which may be written relative in one surface and absolute in another.
func workspaceAliases(workspace string) map[string]struct{} {
	out := make(map[string]struct{})
	if workspace == "" {
		return out
	}
	out[filepath.Clean(workspace)] = struct{}{}
	if abs, err := filepath.Abs(workspace); err == nil {
		out[filepath.Clean(abs)] = struct{}{}
	}
	return out
}

// matchesWorkspace reports whether a session's recorded workspace is one of
// aliases.
//
// An empty workspace never matches. A session that never recorded a root is
// not evidence that it belongs to the directory being cleared, and treating it
// as a match would delete unrelated sessions on the way past.
func matchesWorkspace(recorded string, aliases map[string]struct{}) bool {
	if recorded == "" {
		return false
	}
	if _, ok := aliases[filepath.Clean(recorded)]; ok {
		return true
	}
	if abs, err := filepath.Abs(recorded); err == nil {
		_, ok := aliases[filepath.Clean(abs)]
		return ok
	}
	return false
}
