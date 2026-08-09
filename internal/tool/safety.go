package tool

import (
	"path/filepath"
	"regexp"
	"strings"
)

// ── Bash: catastrophic (Deny) ────────────────────────────────────────────────

// catastrophicBashPatterns match commands that are never safe regardless of context.
// Targets: root filesystem deletion and raw block device writes.
var catastrophicBashPatterns = []*regexp.Regexp{
	// rm -rf / or rm -rf /* (root deletion); must not match rm -rf /tmp etc.
	regexp.MustCompile(`rm\s+-[a-z]*r[a-z]*f\s+/\s*$`),
	regexp.MustCompile(`rm\s+-[a-z]*r[a-z]*f\s+/\*`),
	regexp.MustCompile(`rm\s+-[a-z]*f[a-z]*r\s+/\s*$`),
	regexp.MustCompile(`rm\s+-[a-z]*f[a-z]*r\s+/\*`),
	// rm -rf ~ or rm -rf ~/
	regexp.MustCompile(`rm\s+-[a-z]*r[a-z]*f\s+~/?(\s+|$)`),
	regexp.MustCompile(`rm\s+-[a-z]*f[a-z]*r\s+~/?(\s+|$)`),
	// dd writing to raw block devices
	regexp.MustCompile(`of=/dev/(sd|nvme|disk|rd|vd|xvd)`),
	// mkfs on a device
	regexp.MustCompile(`mkfs\S*\s+/dev/(sd|nvme|disk|rd|vd|xvd)`),
}

// isCatastrophicBash returns true when the command matches a known catastrophic pattern.
func isCatastrophicBash(command string) bool {
	lower := strings.ToLower(command)
	for _, re := range catastrophicBashPatterns {
		if re.MatchString(lower) {
			return true
		}
	}
	return false
}

// ── Bash: risky (Ask) ────────────────────────────────────────────────────────

// riskyBashPrefixes are command names whose execution may have significant side effects
// (network, file destruction, package mutation, permission changes, process control).
// Commands matching these prefixes trigger ToolActionAsk so the user can review them
// in interactive sessions; without an ApprovalHandler they collapse to Deny.
var riskyBashPrefixes = []string{
	// File destruction / movement
	"rm", "mv", "shred", "truncate",
	// Network transfer
	"curl", "wget", "ssh", "scp", "sftp", "rsync", "nc", "ncat", "netcat",
	// Package management
	"pip", "pip3", "npm", "yarn", "pnpm", "npx",
	"apt", "apt-get", "apt-cache",
	"brew", "yum", "dnf", "pacman", "gem", "cargo install",
	// Permissions / identity
	"chmod", "chown", "sudo", "su", "doas", "chroot",
	// Process control
	"kill", "killall", "pkill",
	// Service management
	"systemctl", "service",
}

// commandTokens splits a shell command string into the first token of each
// pipeline segment (split on &&, ||, ;, |) so piped commands are checked individually.
func commandTokens(command string) []string {
	var tokens []string
	for _, segment := range strings.FieldsFunc(command, func(r rune) bool {
		return r == '|' || r == ';' || r == '&'
	}) {
		fields := strings.Fields(segment)
		if len(fields) > 0 {
			// Strip any leading variable assignments (FOO=bar cmd …).
			for _, f := range fields {
				if !strings.ContainsRune(f, '=') {
					tokens = append(tokens, f)
					break
				}
			}
		}
	}
	return tokens
}

// isRiskyBashCommand returns true when any pipeline segment's first token matches
// a known risky command prefix. Risky commands trigger ToolActionAsk (not Deny).
func isRiskyBashCommand(command string) bool {
	for _, token := range commandTokens(command) {
		lower := strings.ToLower(token)
		// Strip path prefix so "/usr/bin/curl" matches "curl".
		if idx := strings.LastIndexByte(lower, '/'); idx >= 0 {
			lower = lower[idx+1:]
		}
		for _, prefix := range riskyBashPrefixes {
			if lower == prefix || strings.HasPrefix(lower, prefix) {
				return true
			}
		}
	}
	return false
}

// ── Sensitive file paths (Ask) ───────────────────────────────────────────────

// sensitiveSuffixes are file extensions that typically hold private key material.
var sensitiveSuffixes = []string{
	".pem", ".key", ".p12", ".pfx", ".der", ".crt",
}

// sensitiveNames are exact filenames (lowercased) that hold credentials or secrets.
var sensitiveNames = []string{
	".env",
	".netrc",
	".htpasswd",
	"credentials",
	"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa",
}

// sensitiveNamePrefixes are filename prefixes (lowercased) that indicate a secret file.
var sensitiveNamePrefixes = []string{
	".env.", // .env.local, .env.production, .env.test …
}

// sensitivePathSegments are path segments whose presence in the full path indicates a secrets store.
var sensitivePathSegments = []string{
	"/.aws/credentials",
	"/.aws/config",
	"/.ssh/id_rsa",
	"/.ssh/id_ed25519",
	"/.ssh/id_ecdsa",
	"/.ssh/id_dsa",
}

// isSensitivePath returns true when path matches a known credential or private-key pattern.
// The check is case-insensitive and operates on both the filename and the full path.
func isSensitivePath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	base := strings.ToLower(filepath.Base(path))

	for _, name := range sensitiveNames {
		if base == name {
			return true
		}
	}

	for _, prefix := range sensitiveNamePrefixes {
		if strings.HasPrefix(base, prefix) {
			return true
		}
	}

	ext := strings.ToLower(filepath.Ext(base))
	for _, suf := range sensitiveSuffixes {
		if ext == suf {
			return true
		}
	}

	for _, seg := range sensitivePathSegments {
		if strings.Contains(lower, seg) {
			return true
		}
	}

	return false
}
