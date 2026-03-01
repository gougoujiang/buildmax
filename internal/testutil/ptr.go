// Package testutil provides test-only helpers for use from _test.go files.
package testutil

// PtrString returns a pointer to s. Useful for filling optional *string fields in tests.
func PtrString(s string) *string {
	return &s
}
