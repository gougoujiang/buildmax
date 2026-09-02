package sandbox

import (
	"slices"
	"strings"
)

// alwaysDenyExact lists BuildMax's own process credentials, which must never
// reach a sandboxed child and are never allow-listable. A Team Secret grant can
// re-admit a secret-shaped name (see ScrubEnvList's allowed set), but not one
// of these: the run token, the JWT signing secret, and the deployment provider
// key are the deployment's own authority, not a value a run is ever granted.
// See docs/design/team-secrets.md §13.1.
var alwaysDenyExact = []string{
	"BUILDMAX_API_KEY",
	"BUILDMAX_RUN_TOKEN",
	"BUILDMAX_JWT_SECRET",
}

// secretEnvDenyExact lists env var names that carry agent-process secrets and
// must not reach a sandboxed child unless this run explicitly declared one as a
// grant. Mirrors the design in docs/design/sandbox-boundaries.md §6.4.
var secretEnvDenyExact = []string{
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

// isAlwaysDenyEnvName reports whether name is one of BuildMax's own
// credentials, which an allow-list can never re-admit.
func isAlwaysDenyEnvName(name string) bool {
	return slices.Contains(alwaysDenyExact, strings.ToUpper(name))
}

// isSecretEnvName reports whether name matches the secret-shaped denylist.
// Returns false for short generic names (e.g. "KEY=…" alone or "PATH").
func isSecretEnvName(name string) bool {
	if name == "" {
		return false
	}
	upper := strings.ToUpper(name)
	if isAlwaysDenyEnvName(name) || slices.Contains(secretEnvDenyExact, upper) {
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

// scrubEnvName reports whether an entry named name should be dropped, given the
// set of names this run declared as Secret grants. A declared name passes even
// when it is secret-shaped -- that is the whole point of a grant -- except
// BuildMax's own credentials, which pass never.
func scrubEnvName(name string, allowed map[string]bool) bool {
	if isAlwaysDenyEnvName(name) {
		return true
	}
	if allowed[name] {
		return false
	}
	return isSecretEnvName(name)
}

// ScrubEnvList drops entries whose name is secret-shaped, keeping the names in
// allowed (this run's declared Secret grants) and always dropping BuildMax's
// own credentials. Entries that are not "KEY=VALUE" pairs are kept as-is.
// Exported so agentapp / non-sandbox callers can use the same filter.
func ScrubEnvList(env []string, allowed map[string]bool) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		if k, _, ok := strings.Cut(e, "="); ok && scrubEnvName(k, allowed) {
			continue
		}
		out = append(out, e)
	}
	return out
}
