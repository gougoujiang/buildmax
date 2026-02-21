package util

import (
	"strings"
	"testing"
)

func TestNewPrefixedID(t *testing.T) {
	const bodyLen = 20
	const allowedChars = "0123456789abcdefghijklmnopqrstuvwxyz"

	for _, prefix := range []string{PrefixUser, PrefixWorkspace, "ar"} {
		t.Run("prefix_"+prefix, func(t *testing.T) {
			seen := make(map[string]bool)
			for i := 0; i < 100; i++ {
				id := NewPrefixedID(prefix)
				parts := strings.SplitN(id, "_", 2)
				if len(parts) != 2 || parts[0] != prefix {
					t.Errorf("NewPrefixedID(%q) = %q, want prefix %q_<body>", prefix, id, prefix)
				}
				body := parts[1]
				if len(body) != bodyLen {
					t.Errorf("body length = %d, want %d", len(body), bodyLen)
				}
				for _, c := range body {
					if !strings.ContainsRune(allowedChars, c) {
						t.Errorf("body contains invalid char %q", c)
					}
				}
				if seen[id] {
					t.Errorf("duplicate ID %q", id)
				}
				seen[id] = true
			}
		})
	}
}

func TestNewPrefixedID_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := NewPrefixedID(PrefixChat)
		if seen[id] {
			t.Fatalf("collision at iteration %d: %q", i, id)
		}
		seen[id] = true
	}
}
