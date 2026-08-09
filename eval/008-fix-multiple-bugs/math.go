package mathutil

import "errors"

// ErrDivisionByZero is returned when dividing by zero.
var ErrDivisionByZero = errors.New("division by zero")

// Divide returns a / b.
// Bug 1: missing zero check — should return ErrDivisionByZero when b == 0.
func Divide(a, b float64) (float64, error) {
	return a / b, nil
}

// Pow returns base raised to the power of exp (exp must be >= 0).
// Bug 2: base case returns base instead of 1, so Pow(x, 0) = x not 1.
func Pow(base, exp int) int {
	if exp == 0 {
		return base
	}
	return base * Pow(base, exp-1)
}

// Clamp returns n restricted to the range [lo, hi].
// Bug 3: the two branch conditions are swapped, so values are clamped to the wrong bound.
func Clamp(n, lo, hi int) int {
	if n < lo {
		return hi
	}
	if n > hi {
		return lo
	}
	return n
}
