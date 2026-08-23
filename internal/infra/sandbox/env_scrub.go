package sandbox

import (
	"slices"
	"strings"
)

// secretEnvDenyExact lists env var names that always carry agent-process
// secrets and must never reach a sandboxed child. Mirrors the design in
// docs/design/sandbox-boundaries.md §6.4.
//
// Operators who explicitly need to pass one of these to a subprocess can
// re-introduce it via cmd.Env outside the sandbox path; the sandbox
// itself refuses to forward them.
var secretEnvDenyExact = []string{
	"BUILDMAX_API_KEY",
	"BUILDMAX_RUN_TOKEN",
	"BUILDMAX_JWT_SECRET",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"GITHUB_TOKEN",
	"GH_TOKEN",
	"OPENAI_API_KEY",
	"ANTHROPIC_API_KEY",
	"NPM_TOKEN",
}

// secretEnvSuffixes deny vars whose name *ends* with one of these
// substrings. Case-insensitive. These catch the long tail of provider-
// specific names we cannot list exhaustively (e.g. STRIPE_SECRET_KEY,
// SLACK_BOT_TOKEN, FOO_PASSWORD).
var secretEnvSuffixes = []string{
	"_SECRET",
	"_KEY",
	"_TOKEN",
	"_PASSWORD",
	"_PASSWD",
	"_PWD",
}

// isSecretEnvName reports whether name matches the secret-shaped denylist.
// Returns false for short generic names (e.g. "KEY=…" alone or "PATH").
func isSecretEnvName(name string) bool {
	if name == "" {
		return false
	}
	upper := strings.ToUpper(name)
	if slices.Contains(secretEnvDenyExact, upper) {
		return true
	}
	// Require a leading underscore so single-word names like "PATH" or
	// "KEY" (rare but possible) do not over-match. The suffix list
	// always includes a leading "_".
	for _, suf := range secretEnvSuffixes {
		if strings.HasSuffix(upper, suf) {
			return true
		}
	}
	return false
}

// ScrubEnvList drops entries whose name matches the secret pattern.
// Entries that are not "KEY=VALUE" pairs are kept as-is. Exported so
// agentapp / non-sandbox callers can use the same filter where useful.
func ScrubEnvList(env []string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		if k, _, ok := strings.Cut(e, "="); ok && isSecretEnvName(k) {
			continue
		}
		out = append(out, e)
	}
	return out
}
