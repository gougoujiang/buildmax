package calc

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(3, 4); got != 7 {
		t.Fatalf("Add(3, 4) = %d, want 7", got)
	}
}

func TestComputeProduct(t *testing.T) {
	if got := ComputeProduct(3, 4); got != 12 {
		t.Fatalf("ComputeProduct(3, 4) = %d, want 12", got)
	}
}
