package slashcmd

import (
	"slices"
	"testing"
)

func TestBuiltinsAreSortedUniqueAndDescribed(t *testing.T) {
	seen := map[string]bool{}
	var names []string
	for _, c := range builtins {
		if c.Name == "" {
			t.Errorf("a command has an empty name")
		}
		if c.Description == "" {
			t.Errorf("%q has no description", c.Name)
		}
		if c.Surfaces == 0 {
			t.Errorf("%q is offered on no surface", c.Name)
		}
		if seen[c.Name] {
			t.Errorf("%q is listed twice", c.Name)
		}
		seen[c.Name] = true
		names = append(names, c.Name)
	}
	if !slices.IsSorted(names) {
		t.Errorf("builtins are not sorted by name: %v", names)
	}
}

func TestForFiltersBySurface(t *testing.T) {
	cli := For(CLI)
	desktop := For(Desktop)
	if len(cli) <= len(desktop) {
		t.Fatalf("CLI offers %d commands, Desktop %d; CLI should offer more (it has /sessions)", len(cli), len(desktop))
	}
	if IsCommand(Desktop, "sessions") {
		t.Errorf("sessions must not be a Desktop command")
	}
	if !IsCommand(CLI, "sessions") {
		t.Errorf("sessions must be a CLI command")
	}
	// The leading slash is accepted either way.
	if !IsCommand(CLI, "/info") || !IsCommand(Desktop, "info") {
		t.Errorf("IsCommand should accept a name with or without its slash")
	}
}

func TestNamesCarryTheSlash(t *testing.T) {
	for _, n := range Names(Desktop) {
		if n == "" || n[0] != '/' {
			t.Fatalf("name %q is missing its leading slash", n)
		}
	}
}
