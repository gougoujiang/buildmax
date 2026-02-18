package util

import (
	"strings"
	"testing"

	"github.com/oklog/ulid/v2"
)

func TestNewID(t *testing.T) {
	const wantLen = 25
	const allowedChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	for i := 0; i < 100; i++ {
		id := NewID()
		if len(id) != wantLen {
			t.Errorf("NewID() length = %d, want %d", len(id), wantLen)
		}
		for _, c := range id {
			if !strings.ContainsRune(allowedChars, c) {
				t.Errorf("NewID() contains invalid char %q", c)
			}
		}
	}
}

func TestNewULID(t *testing.T) {
	const wantLen = 26

	for i := 0; i < 100; i++ {
		s := NewULID()
		if len(s) != wantLen {
			t.Errorf("NewULID() length = %d, want %d", len(s), wantLen)
		}
		if _, err := ulid.Parse(s); err != nil {
			t.Errorf("NewULID() = %q: parse error: %v", s, err)
		}
	}
}
