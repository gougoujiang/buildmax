package trace

import "regexp"

// Redaction scrubs common secret shapes from trace text before it is written to
// disk. It is intentionally conservative — keyword/shape based — so it does not
// mangle ordinary tool output. Grow this set here; the recorder calls Redact
// only.

var redactors = []struct {
	re   *regexp.Regexp
	repl string
}{
	// "Bearer <token>"
	{regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._\-]+`), "Bearer [redacted]"},
	// OpenAI-style "sk-..." keys (and sk-proj- etc.)
	{regexp.MustCompile(`sk-[A-Za-z0-9._\-]{16,}`), "[redacted]"},
	// key=value / key: value where the key looks sensitive. The value run stops
	// at whitespace, quotes, commas, or closing brackets so JSON stays parseable.
	{regexp.MustCompile(`(?i)\b(authorization|api[_\-]?key|access[_\-]?token|token|secret|password|passwd|pwd)\b(\s*["']?\s*[:=]\s*["']?)([^\s"',}\]]+)`), "$1$2[redacted]"},
}

// Redact returns s with recognized secret values replaced by a redaction
// marker. Empty input is returned unchanged.
func Redact(s string) string {
	if s == "" {
		return s
	}
	for _, r := range redactors {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}
