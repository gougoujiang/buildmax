package util

// PtrString returns a pointer to s. Useful for filling optional *string fields.
func PtrString(s string) *string {
	return &s
}
