// Package testutil provides test-only helpers for use from _test.go files:
// ptr helpers (e.g. PtrString), JWT (SignJWT), and in-memory mocks for entity stores and quota (MockUserStore, MockChatStore, etc.).
package testutil

import "buildmax/internal/ptr"

// PtrString returns a pointer to s. Useful for filling optional *string fields in tests.
func PtrString(s string) *string {
	return ptr.PtrString(s)
}
