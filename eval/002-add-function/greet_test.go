package greet

import (
	"strings"
	"testing"
)

func TestGreet(t *testing.T) {
	if got := Greet("World"); got != "Hello, World!" {
		t.Fatalf("Greet(World) = %q, want %q", got, "Hello, World!")
	}
}

func TestGreetAll(t *testing.T) {
	names := []string{"Alice", "Bob", "Charlie"}
	result := GreetAll(names)
	for _, name := range names {
		want := Greet(name)
		if !strings.Contains(result, want) {
			t.Errorf("GreetAll result missing greeting for %q\nresult: %q", name, result)
		}
	}
}
