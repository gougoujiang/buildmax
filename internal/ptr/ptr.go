// Package ptr provides pointer helpers for tests and optional fields.
package ptr

// PtrString returns a pointer to s. Useful for filling optional *string fields in tests.
func PtrString(s string) *string {
	return &s
}
