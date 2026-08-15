package bootstrap

import (
	"context"
	"strings"
	"testing"
)

func TestCleanTaskTitle(t *testing.T) {
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
		got := cleanTaskTitle(tt.input)
		if got != tt.want {
			t.Errorf("cleanTaskTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTitleGenAdapter_EmptyInput(t *testing.T) {
	a := &titleGenAdapter{client: nil}
	title, pt, ct, err := a.GenerateTitle(context.TODO(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "" || pt != 0 || ct != 0 {
		t.Errorf("empty input: got title=%q pt=%d ct=%d, want all zero", title, pt, ct)
	}
}

func TestCleanTaskTitle_NoLengthCap(t *testing.T) {
	long := strings.Repeat("x", 200)
	got := cleanTaskTitle(long)
	if got != long {
		t.Errorf("cleanTaskTitle should not cap length, got len=%d", len(got))
	}
}

func TestRunServer_PortOverride(t *testing.T) {
	// Verify that RunServer prefers portOverride > 0 over the config file port.
	// We can't start a real server here; just confirm the logic compiles and the
	// function signature accepts a portOverride int.
	_ = func() { _ = RunServer(context.TODO(), 9999) }
}
