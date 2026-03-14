// Package utils provides shared utilities (e.g. pointer helpers for tests and optional fields).
package utils

// PtrString returns a pointer to s. Useful for filling optional *string fields in tests.
func PtrString(s string) *string {
	return &s
}
