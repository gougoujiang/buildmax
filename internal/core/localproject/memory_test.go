package localproject

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// One representation of "there is nothing here" across the render, the tool
// argument, and the stored metadata. A hash for the empty document would be a
// value every caller had to learn to recognize.
func TestMemoryDigestOfEmptyIsEmpty(t *testing.T) {
	if got := MemoryDigest(""); got != "" {
		t.Errorf("MemoryDigest(\"\") = %q, want empty", got)
	}
	full := MemoryDigest("# Project Memory\n")
	if !strings.HasPrefix(full, "sha256:") {
		t.Errorf("MemoryDigest = %q, want a sha256: prefix", full)
	}
	if MemoryDigest("# Project Memory\n") != full {
		t.Error("MemoryDigest is not stable for one input")
	}
	if MemoryDigest("# Project Memory") == full {
		t.Error("MemoryDigest ignores a trailing newline; it must identify exact content")
	}
}

func TestValidateMemory(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr error
	}{
		{name: "empty is the forget operation", content: ""},
		{name: "ordinary document", content: "# Project Memory\n\n- Prefer table-driven tests.\n"},
		{name: "at the limit", content: strings.Repeat("a", MaxMemoryChars)},
		{name: "over the limit", content: strings.Repeat("a", MaxMemoryChars+1), wantErr: ErrMemoryTooLarge},
		{
			// Counted in characters, not bytes: a document of multi-byte text
			// must not be refused for being written in another script.
			name:    "multi-byte at the limit",
			content: strings.Repeat("项", MaxMemoryChars),
		},
		{name: "not utf-8", content: string([]byte{0xff, 0xfe}), wantErr: ErrMemoryNotText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMemory(tt.content)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateMemory = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// The refusal has to be usable in a tool result and a log, so it names the
// shape and never the value.
func TestScanMemoryForSecretsNamesShapesNotValues(t *testing.T) {
	err := ScanMemoryForSecrets("# Project Memory\n\n- The deploy key is api_key=supersecretvalue\n")
	if !errors.Is(err, ErrMemorySecret) {
		t.Fatalf("ScanMemoryForSecrets = %v, want ErrMemorySecret", err)
	}
	if strings.Contains(err.Error(), "supersecretvalue") {
		t.Errorf("the refusal quotes the credential: %v", err)
	}

	if err := ScanMemoryForSecrets("# Project Memory\n\n- Prefer narrow table-driven tests.\n"); err != nil {
		t.Errorf("an ordinary document was refused: %v", err)
	}
}

func TestNextMemoryMetaAdvancesTheRevision(t *testing.T) {
	previous := MemoryMeta{Version: MemoryVersion, Revision: 6, Digest: MemoryDigest("old")}
	now := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)

	next := NextMemoryMeta(previous, MemoryWrite{
		Content:   "new",
		SessionID: "s-1",
		RunID:     "r-1",
	}, now)

	if next.Revision != 7 {
		t.Errorf("Revision = %d, want 7", next.Revision)
	}
	if next.Digest != MemoryDigest("new") {
		t.Errorf("Digest = %q, want the new content's", next.Digest)
	}
	if next.UpdatedBySessionID != "s-1" || next.UpdatedByRunID != "r-1" {
		t.Errorf("provenance = %s/%s, want s-1/r-1", next.UpdatedBySessionID, next.UpdatedByRunID)
	}
	if !next.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", next.UpdatedAt, now)
	}
}
