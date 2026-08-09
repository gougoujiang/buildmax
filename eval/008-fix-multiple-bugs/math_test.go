package mathutil

import (
	"math"
	"testing"
)

func TestDivide_Normal(t *testing.T) {
	got, err := Divide(10, 4)
	if err != nil || math.Abs(got-2.5) > 1e-9 {
		t.Errorf("Divide(10,4) = %v, %v; want 2.5, nil", got, err)
	}
}

func TestDivide_ByZero(t *testing.T) {
	_, err := Divide(5, 0)
	if err != ErrDivisionByZero {
		t.Errorf("Divide(5,0) err = %v; want ErrDivisionByZero", err)
	}
}

func TestPow_Zero(t *testing.T) {
	if got := Pow(7, 0); got != 1 {
		t.Errorf("Pow(7,0) = %d; want 1", got)
	}
}

func TestPow_Positive(t *testing.T) {
	cases := []struct{ base, exp, want int }{
		{2, 1, 2}, {2, 3, 8}, {3, 4, 81}, {10, 2, 100},
	}
	for _, c := range cases {
		if got := Pow(c.base, c.exp); got != c.want {
			t.Errorf("Pow(%d,%d) = %d; want %d", c.base, c.exp, got, c.want)
		}
	}
}

func TestClamp_Below(t *testing.T) {
	if got := Clamp(-5, 0, 10); got != 0 {
		t.Errorf("Clamp(-5,0,10) = %d; want 0", got)
	}
}

func TestClamp_Above(t *testing.T) {
	if got := Clamp(15, 0, 10); got != 10 {
		t.Errorf("Clamp(15,0,10) = %d; want 10", got)
	}
}

func TestClamp_Inside(t *testing.T) {
	if got := Clamp(5, 0, 10); got != 5 {
		t.Errorf("Clamp(5,0,10) = %d; want 5", got)
	}
}
