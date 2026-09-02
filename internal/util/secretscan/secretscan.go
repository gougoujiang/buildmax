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

import (
	"regexp"
	"strings"
)

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

const (
	// minExactValue is the shortest exact value worth redacting. Below it, a
	// value is as likely to be an ordinary word as a credential, and replacing
	// every "abc" in output would mangle more than it protects. See
	// docs/design/team-secrets.md §12.
	minExactValue = 6
	// maxExactValue bounds a single exact value so one very large credential
	// (a certificate, a key file) cannot make every redaction pass unbounded.
	maxExactValue = 4096
)

// Redactor redacts both recognized secret shapes and a fixed set of exact
// values. The exact set is a run's materialized Team Secret values, registered
// before the Agent starts so they do not drift into a durable trace, a log, or
// a tool result. It is defense in depth, not a boundary: a value can be encoded
// or transformed past it, which is why the primary control is withholding the
// value from the general environment. See docs/design/team-secrets.md §12.
type Redactor struct {
	exact []string
}

// NewRedactor builds a Redactor over the given exact values, dropping empty,
// very short, and oversized ones. A Redactor with no usable values redacts by
// shape only, exactly like the package Redact.
func NewRedactor(values []string) *Redactor {
	var exact []string
	for _, v := range values {
		if len(v) >= minExactValue && len(v) <= maxExactValue {
			exact = append(exact, v)
		}
	}
	return &Redactor{exact: exact}
}

// Redact replaces exact registered values first, then recognized shapes. A nil
// Redactor redacts by shape only, so a caller never needs to nil-check.
func (r *Redactor) Redact(s string) string {
	if s == "" {
		return s
	}
	if r != nil {
		for _, v := range r.exact {
			s = strings.ReplaceAll(s, v, "[redacted]")
		}
	}
	return Redact(s)
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
