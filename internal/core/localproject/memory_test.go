package localproject

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func sampleMemory() Memory {
	return Memory{
		Name:        "rejected-sse-transport",
		Description: "SSE was rejected for the event stream; it cannot resume mid-turn",
		Type:        MemoryTypeProject,
		SessionID:   "b0a1c2d3-0000-0000-0000-000000000000",
		UpdatedAt:   time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC),
		Body:        "The event stream uses WebSocket, not SSE.\n\n**Why:** a reconnect cannot resume a turn in flight.",
	}
}

func TestValidMemoryName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"merge-commit", true},
		{"a", true},
		{"fixture-layout-2", true},
		{"", false},
		{"Merge-Commit", false},
		{"merge_commit", false},
		{"-leading", false},
		{"trailing-", false},
		{"double--hyphen", false},
		// The slug is a file name, so the character set is a containment guard
		// as much as a convention.
		{"../escape", false},
		{"a/b", false},
		{strings.Repeat("a", MaxMemoryNameChars+1), false},
	}
	for _, tt := range tests {
		if got := ValidMemoryName(tt.name); got != tt.want {
			t.Errorf("ValidMemoryName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestMemoryValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(m *Memory)
		ok     bool
	}{
		{name: "as constructed", mutate: func(*Memory) {}, ok: true},
		{name: "bad slug", mutate: func(m *Memory) { m.Name = "Not A Slug" }},
		{name: "unknown type", mutate: func(m *Memory) { m.Type = "user" }},
		{name: "no description", mutate: func(m *Memory) { m.Description = "  " }},
		{
			// An index line is one line; a description with a newline would
			// break the block it is rendered into.
			name:   "multi-line description",
			mutate: func(m *Memory) { m.Description = "one\ntwo" },
		},
		{
			name:   "description over budget",
			mutate: func(m *Memory) { m.Description = strings.Repeat("d", MaxDescriptionChars+1) },
		},
		{
			name:   "description at budget",
			mutate: func(m *Memory) { m.Description = strings.Repeat("d", MaxDescriptionChars) },
			ok:     true,
		},
		{
			// Counted in characters, not bytes: a memory written in another
			// script must not be refused for that.
			name:   "multi-byte description at budget",
			mutate: func(m *Memory) { m.Description = strings.Repeat("项", MaxDescriptionChars) },
			ok:     true,
		},
		{name: "empty body", mutate: func(m *Memory) { m.Body = "\n\t " }},
		{name: "body over budget", mutate: func(m *Memory) { m.Body = strings.Repeat("b", MaxBodyChars+1) }},
		{name: "body at budget", mutate: func(m *Memory) { m.Body = strings.Repeat("b", MaxBodyChars) }, ok: true},
		{name: "body not utf-8", mutate: func(m *Memory) { m.Body = string([]byte{0xff, 0xfe}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sampleMemory()
			tt.mutate(&m)
			err := m.Validate()
			if tt.ok && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if !tt.ok {
				if err == nil {
					t.Fatal("Validate() = nil, want an error")
				}
				if !errors.Is(err, ErrMemoryInvalid) {
					t.Errorf("Validate() = %v, want ErrMemoryInvalid", err)
				}
			}
		})
	}
}

// The refusal has to be usable in a tool result and a log, so it names the
// shape and never the value.
func TestScanMemoryForSecretsNamesShapesNotValues(t *testing.T) {
	err := ScanMemoryForSecrets("d", "The deploy key is api_key=supersecretvalue")
	if !errors.Is(err, ErrMemorySecret) {
		t.Fatalf("ScanMemoryForSecrets = %v, want ErrMemorySecret", err)
	}
	if strings.Contains(err.Error(), "supersecretvalue") {
		t.Errorf("the refusal quotes the credential: %v", err)
	}
	if err := ScanMemoryForSecrets("how we test", "Prefer narrow table-driven tests."); err != nil {
		t.Errorf("an ordinary memory was refused: %v", err)
	}
	// The description is the part that goes to the model on every call, so a
	// token pasted there is the most exposed place in the store, not the least.
	if err := ScanMemoryForSecrets("api_key=supersecretvalue", "an ordinary body"); !errors.Is(err, ErrMemorySecret) {
		t.Errorf("a credential in the description was accepted: %v", err)
	}
}

// The digest is what the read-then-replace rule compares, so it has to be
// stable for one body and different for any change.
func TestBodyDigest(t *testing.T) {
	body := "The event stream uses WebSocket."
	same := "The event stream uses WebSocket."
	if BodyDigest(body) != BodyDigest(same) {
		t.Error("BodyDigest is not stable for one body")
	}
	if BodyDigest(body) == BodyDigest(body+" ") {
		t.Error("BodyDigest ignores trailing whitespace; it must identify exact content")
	}
	if !strings.HasPrefix(BodyDigest(body), "sha256:") {
		t.Errorf("BodyDigest = %q, want a sha256: prefix", BodyDigest(body))
	}
}

func TestSortMemoriesOrdersByName(t *testing.T) {
	memories := []Memory{{Name: "c"}, {Name: "a"}, {Name: "b"}}
	SortMemories(memories)
	for i, want := range []string{"a", "b", "c"} {
		if memories[i].Name != want {
			t.Fatalf("order = %v, want a, b, c", memories)
		}
	}
}
