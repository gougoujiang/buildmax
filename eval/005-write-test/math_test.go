package mathutil

import "testing"

func TestSubtract(t *testing.T) {
	if got := Subtract(10, 3); got != 7 {
		t.Fatalf("Subtract(10, 3) = %d, want 7", got)
	}
}
