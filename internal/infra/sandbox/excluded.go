// Package sandbox provides the OS-level bash subprocess sandbox.
//
// See docs/design/sandbox-boundaries.md. This file implements
// the excluded_commands matcher — commands the user has opted out of the
// sandbox. Per upstream's NOTE in src/tools/BashTool/shouldUseSandbox.ts:
// "excludedCommands is a user-facing convenience, not a security boundary."
package sandbox

import "strings"

// MatchesExcluded reports whether command matches any pattern in
// excluded. Matching mirrors Claude Code's bashPermissionRule semantics:
//
//   - Patterns ending with ":*" are *prefix* matches: "npm:*" matches
//     "npm", "npm install", "npm test".
//   - Patterns ending with " *" are *prefix-with-space* matches:
//     "docker *" matches "docker ps" but not "docker".
//   - Otherwise the pattern is an *exact* match against the command (or
//     against the command's first pipeline segment).
//
// The command is split on `&&`, `||`, `;`, `|` and each segment is
// checked individually so a compound command (`docker ps && curl x`)
// does not silently bypass the policy because its first subcommand
// matches.
func MatchesExcluded(command string, excluded []string) bool {
	if len(excluded) == 0 || command == "" {
		return false
	}
	for _, seg := range splitPipelineSegments(command) {
		trimmed := strings.TrimSpace(seg)
		if trimmed == "" {
			continue
		}
		for _, pattern := range excluded {
			if matchOnePattern(trimmed, pattern) {
				return true
			}
		}
	}
	return false
}

func matchOnePattern(cmd, pattern string) bool {
	switch {
	case strings.HasSuffix(pattern, ":*"):
		prefix := strings.TrimSuffix(pattern, ":*")
		if prefix == "" {
			return false
		}
		return cmd == prefix || strings.HasPrefix(cmd, prefix+" ")
	case strings.HasSuffix(pattern, " *"):
		prefix := strings.TrimSuffix(pattern, " *")
		if prefix == "" {
			return false
		}
		return strings.HasPrefix(cmd, prefix+" ")
	default:
		return cmd == pattern
	}
}

// splitPipelineSegments splits a shell command string on `&&`, `||`, `;`, `|`.
// Quoting is not honored — this is a conservative split (more segments
// rather than fewer) which is the safer direction for an opt-out list.
func splitPipelineSegments(command string) []string {
	var out []string
	start := 0
	i := 0
	for i < len(command) {
		c := command[i]
		switch c {
		case ';':
			out = append(out, command[start:i])
			start = i + 1
			i++
		case '&':
			if i+1 < len(command) && command[i+1] == '&' {
				out = append(out, command[start:i])
				start = i + 2
				i += 2
				continue
			}
			i++
		case '|':
			// `||` or single pipe
			out = append(out, command[start:i])
			if i+1 < len(command) && command[i+1] == '|' {
				start = i + 2
				i += 2
				continue
			}
			start = i + 1
			i++
		default:
			i++
		}
	}
	if start < len(command) {
		out = append(out, command[start:])
	}
	return out
}
