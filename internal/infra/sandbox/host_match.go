package sandbox

import (
	"net"
	"strings"
)

// HostMatcher decides whether a hostname is allowed by sandbox network
// policy. Patterns mirror Claude Code's `allowed_domains` semantics:
//
//   - exact host: "api.github.com"
//   - leading wildcard: "*.npmjs.org" matches every label-suffix.
//     "*.example.com" matches "registry.example.com" but **not**
//     "example.com" (matches Claude Code; one-or-more labels).
//   - port suffix: "api.example.com:443" matches that exact port; without
//     port, the matcher ignores the port on the input host.
//
// Deny patterns win over allow patterns. The empty allow list means
// "deny by default" — the secure default matching upstream's "no
// domains are pre-allowed."
type HostMatcher struct {
	allowed []hostPattern
	denied  []hostPattern
}

// NewHostMatcher compiles allow/deny string lists into a matcher.
func NewHostMatcher(allowed, denied []string) *HostMatcher {
	return &HostMatcher{
		allowed: compileHostPatterns(allowed),
		denied:  compileHostPatterns(denied),
	}
}

// Allowed reports whether host (with or without port) is reachable.
// reason carries a short, LLM-friendly explanation on deny.
func (m *HostMatcher) Allowed(host string) (ok bool, reason string) {
	if m == nil {
		return true, ""
	}
	h, port := splitHostPort(host)
	if h == "" {
		return false, "sandbox: empty host"
	}
	h = strings.ToLower(h)
	for _, p := range m.denied {
		if p.match(h, port) {
			return false, "sandbox: host \"" + host + "\" matches denied_domains pattern \"" + p.raw + "\""
		}
	}
	if len(m.allowed) == 0 {
		return false, "sandbox: host \"" + host + "\" is not in allowed_domains (allow-list is empty)"
	}
	for _, p := range m.allowed {
		if p.match(h, port) {
			return true, ""
		}
	}
	return false, "sandbox: host \"" + host + "\" is not in allowed_domains"
}

// AllowAll reports whether every host would be allowed. Used by tests
// and by the proxy fast-path. Returns true when allow is non-empty and
// contains a bare "*".
func (m *HostMatcher) AllowAll() bool {
	if m == nil {
		return true
	}
	for _, p := range m.allowed {
		if p.raw == "*" {
			return true
		}
	}
	return false
}

// hostPattern is one compiled allow/deny entry.
type hostPattern struct {
	raw    string
	host   string // lowercase
	port   string // empty = match any port on input
	suffix bool   // host begins with "*."; match the suffix
}

func compileHostPatterns(in []string) []hostPattern {
	out := make([]hostPattern, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		host, port := splitHostPort(s)
		p := hostPattern{raw: s, host: strings.ToLower(host), port: port}
		if strings.HasPrefix(p.host, "*.") {
			p.suffix = true
			p.host = strings.TrimPrefix(p.host, "*.")
		}
		out = append(out, p)
	}
	return out
}

func (p hostPattern) match(host, port string) bool {
	if p.host == "*" {
		return p.port == "" || p.port == port
	}
	if p.suffix {
		// "*.example.com" matches one or more leading labels: a.example.com,
		// a.b.example.com — but not example.com itself. Matches upstream.
		if !strings.HasSuffix(host, "."+p.host) {
			return false
		}
	} else if p.host != host {
		return false
	}
	if p.port != "" && p.port != port {
		return false
	}
	return true
}

// splitHostPort splits "host" or "host:port" into its parts. Square-bracketed
// IPv6 ("[::1]:8080") is handled. Returns ("", "") for inputs with no host.
func splitHostPort(s string) (host string, port string) {
	if s == "" {
		return "", ""
	}
	// IPv6 in brackets: [::1]:8080
	if strings.HasPrefix(s, "[") {
		if h, p, err := net.SplitHostPort(s); err == nil {
			return h, p
		}
		end := strings.LastIndex(s, "]")
		if end > 0 {
			return s[1:end], ""
		}
		return "", ""
	}
	if h, p, err := net.SplitHostPort(s); err == nil {
		return h, p
	}
	return s, ""
}
