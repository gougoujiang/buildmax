// Package secretscan recognizes common secret shapes in free text.
//
// It is deliberately conservative -- keyword and shape based -- so it does not
// mangle ordinary tool output or refuse ordinary prose. Nothing here proves
// text is safe: a scanner that found nothing has found nothing, not established
// that a string holds no credential. Both callers treat it that way. The run
// trace redacts what it recognizes and still bounds and scopes what it writes;
// project memory refuses a write it recognizes and still tells the agent that
// not persisting credentials is its own contract.
//
// Grow the pattern set here rather than in either caller, so what the two
// recognize cannot drift apart.
package secretscan

import "regexp"

// pattern is one recognizable secret shape and how a redaction replaces it.
type pattern struct {
	name string
	re   *regexp.Regexp
	repl string
}

var patterns = []pattern{
	// "Bearer <token>"
	{"bearer token", regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._\-]+`), "Bearer [redacted]"},
	// OpenAI-style "sk-..." keys (and sk-proj- etc.)
	{"api key", regexp.MustCompile(`sk-[A-Za-z0-9._\-]{16,}`), "[redacted]"},
	// key=value / key: value where the key looks sensitive. The value run stops
	// at whitespace, quotes, commas, or closing brackets so JSON stays parseable.
	{
		"credential assignment",
		regexp.MustCompile(`(?i)\b(authorization|api[_\-]?key|access[_\-]?token|token|secret|password|passwd|pwd)\b(\s*["']?\s*[:=]\s*["']?)([^\s"',}\]]+)`),
		"$1$2[redacted]",
	},
	// PEM private key blocks, whatever the algorithm prefix.
	{"private key block", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`), "[redacted]"},
}

// Redact returns s with recognized secret values replaced by a marker. Empty
// input is returned unchanged.
func Redact(s string) string {
	if s == "" {
		return s
	}
	for _, p := range patterns {
		s = p.re.ReplaceAllString(s, p.repl)
	}
	return s
}

// Findings names the secret shapes recognized in s, in the order the patterns
// are declared and without repeats. It returns the names rather than the
// matches so a caller can say what it refused without quoting the credential
// back into a log, an error, or a model's context.
func Findings(s string) []string {
	if s == "" {
		return nil
	}
	var found []string
	for _, p := range patterns {
		if p.re.MatchString(s) {
			found = append(found, p.name)
		}
	}
	return found
}
