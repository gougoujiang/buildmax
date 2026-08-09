package trace

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   string // substring that must appear
		absent string // substring that must NOT appear
	}{
		{"bearer", "token is Bearer abc123XYZ_-.tok now", "Bearer [redacted]", "abc123XYZ"},
		{"sk key", "key is sk-abcdefABCDEF0123456789", "[redacted]", "abcdefABCDEF0123456789"},
		{"keyword equals", `api_key=supersecretvalue`, "api_key=[redacted]", "supersecretvalue"},
		{"keyword json", `{"password": "hunter2"}`, "[redacted]", "hunter2"},
		{"plain text untouched", "the quick brown fox", "the quick brown fox", "[redacted]"},
		{"empty", "", "", "[redacted]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Redact(c.in)
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Errorf("Redact(%q) = %q, want substring %q", c.in, got, c.want)
			}
			if c.absent != "" && strings.Contains(got, c.absent) {
				t.Errorf("Redact(%q) = %q, must not contain %q", c.in, got, c.absent)
			}
		})
	}
}
