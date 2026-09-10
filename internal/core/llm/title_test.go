package llm

import (
	"context"
	"strings"
	"testing"
)

func TestTrimTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`  "Refactor Login"  `, "Refactor Login"},
		{`'Task One'`, "Task One"},
		{"  Plain Title  ", "Plain Title"},
		{`""`, ""},
	}
	for _, tt := range tests {
		if got := TrimTitle(tt.input); got != tt.want {
			t.Errorf("TrimTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTrimTitle_NoLengthCap(t *testing.T) {
	long := strings.Repeat("x", 200)
	if got := TrimTitle(long); got != long {
		t.Errorf("TrimTitle should not cap length, got len=%d", len(got))
	}
}

func TestNewTitleGenerator_EmptyInput(t *testing.T) {
	g := NewTitleGenerator(nil)
	title, pt, ct, err := g.GenerateTitle(context.TODO(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "" || pt != 0 || ct != 0 {
		t.Errorf("empty input: got title=%q pt=%d ct=%d, want all zero", title, pt, ct)
	}
}
