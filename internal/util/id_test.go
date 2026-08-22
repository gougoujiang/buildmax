package util

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestNewPublicID(t *testing.T) {
	const allowed = "abcdefghijklmnopqrstuvwxyz234567"

	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id, err := NewPublicID()
		if err != nil {
			t.Fatalf("NewPublicID() error = %v", err)
		}
		if len(id) != PublicIDLen {
			t.Fatalf("NewPublicID() = %q, length %d, want %d", id, len(id), PublicIDLen)
		}
		for _, c := range id {
			if !strings.ContainsRune(allowed, c) {
				t.Fatalf("NewPublicID() = %q contains %q, outside the base32 alphabet", id, c)
			}
		}
		if seen[id] {
			t.Fatalf("collision at iteration %d: %q", i, id)
		}
		seen[id] = true
	}
}

func TestNewPublicIDRoundTrips(t *testing.T) {
	id, err := NewPublicID()
	if err != nil {
		t.Fatalf("NewPublicID() error = %v", err)
	}
	b, ok := ParsePublicID(id)
	if !ok {
		t.Fatalf("ParsePublicID(%q) rejected its own output", id)
	}
	if len(b) != PublicIDBytes {
		t.Fatalf("ParsePublicID(%q) decoded %d bytes, want %d", id, len(b), PublicIDBytes)
	}
	if got := FormatPublicID(b); got != id {
		t.Fatalf("FormatPublicID(ParsePublicID(%q)) = %q", id, got)
	}
}

// TestNewPublicIDEntropyFailure pins the contract that matters most when the
// kernel refuses entropy: a failed create, not a predictable ID and not a
// panic inside a request.
func TestNewPublicIDEntropyFailure(t *testing.T) {
	orig := randRead
	t.Cleanup(func() { randRead = orig })
	randRead = func([]byte) (int, error) { return 0, errors.New("no entropy") }

	id, err := NewPublicID()
	if err == nil {
		t.Fatalf("NewPublicID() = %q, nil; want an error", id)
	}
	if id != "" {
		t.Fatalf("NewPublicID() = %q on failure, want empty", id)
	}
}

func TestFormatPublicID(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"zero_value", make([]byte, PublicIDBytes), "aaaaaaaaaaaaaaaaaaaa"},
		{"all_ones", bytes.Repeat([]byte{0xff}, PublicIDBytes), "7777777777777777777q"},
		{"too_short", make([]byte, PublicIDBytes-1), ""},
		{"too_long", make([]byte, PublicIDBytes+1), ""},
		{"nil", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatPublicID(tt.in); got != tt.want {
				t.Fatalf("FormatPublicID(%x) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParsePublicID(t *testing.T) {
	// A real value from NewPublicID. The last character carries one bit of the
	// value and four bits of slack, so a canonical ID always ends in "a" or "q"
	// — which is why an invented example is usually not one.
	const canonical = "ivyoh5qcfu6ypfkhyedq"

	tests := []struct {
		name string
		in   string
		ok   bool
	}{
		{"canonical", canonical, true},
		{"uppercase", strings.ToUpper(canonical), true},
		{"mixed_case", "IvYoH5qCfU6yPfKhYeDq", true},
		{"empty", "", false},
		{"too_short", canonical[:PublicIDLen-1], false},
		{"too_long", canonical + "a", false},
		{"padding", "ivyoh5qcfu6ypfkhyed=", false},
		{"base64url_hyphen", "ivyoh5qcfu6ypfkhye-q", false},
		{"base64url_underscore", "ivyoh5qcfu6ypfkhye_q", false},
		{"digit_outside_alphabet", "ivyoh5qcfu6ypfkhyed1", false},
		{"prefixed_legacy_id", "t_9f3k2m8x1qwe7rt4zy", false},
		{"whitespace", " vyoh5qcfu6ypfkhyedq", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, ok := ParsePublicID(tt.in)
			if ok != tt.ok {
				t.Fatalf("ParsePublicID(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			}
			if !ok {
				if b != nil {
					t.Fatalf("ParsePublicID(%q) returned %x with ok=false", tt.in, b)
				}
				return
			}
			if got := FormatPublicID(b); got != strings.ToLower(tt.in) {
				t.Fatalf("ParsePublicID(%q) canonicalized to %q", tt.in, got)
			}
		})
	}
}

// TestParsePublicIDRejectsNonCanonicalTrailingBits is the reason ParsePublicID
// re-encodes. 20 base32 characters carry 100 bits and a public ID is 96, so
// without the check every value would have 16 accepted spellings, and two
// requests naming the same row would look like requests for different ones.
func TestParsePublicIDRejectsNonCanonicalTrailingBits(t *testing.T) {
	id, err := NewPublicID()
	if err != nil {
		t.Fatalf("NewPublicID() error = %v", err)
	}
	want, ok := ParsePublicID(id)
	if !ok {
		t.Fatalf("ParsePublicID(%q) rejected a fresh ID", id)
	}

	const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
	last := strings.IndexByte(alphabet, id[PublicIDLen-1])
	if last < 0 {
		t.Fatalf("last character of %q is outside the alphabet", id)
	}
	// The low 4 bits of the final character are slack. Setting any of them
	// leaves the decoded bytes identical but the text non-canonical.
	for bit := 0; bit < 4; bit++ {
		alt := last | (1 << bit)
		if alt == last {
			continue
		}
		spelling := id[:PublicIDLen-1] + string(alphabet[alt])
		got, ok := ParsePublicID(spelling)
		if ok {
			t.Fatalf("ParsePublicID(%q) accepted a non-canonical spelling of %q", spelling, id)
		}
		if got != nil {
			t.Fatalf("ParsePublicID(%q) returned %x with ok=false", spelling, got)
		}
		if decoded, err := publicIDEncoding.DecodeString(strings.ToUpper(spelling)); err != nil || !bytes.Equal(decoded, want) {
			t.Fatalf("test premise broken: %q does not decode to the same bytes as %q", spelling, id)
		}
	}
}
