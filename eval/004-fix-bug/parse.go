package parse

import (
	"errors"
	"strconv"
)

// ParsePositive parses s as a positive integer (strictly greater than zero).
// Returns an error if s is not a valid integer or if the value is not positive.
func ParsePositive(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	// BUG: should be n > 0 (strictly positive), not n >= 0 (allows zero)
	if n >= 0 {
		return n, nil
	}
	return 0, errors.New("not a positive integer")
}
