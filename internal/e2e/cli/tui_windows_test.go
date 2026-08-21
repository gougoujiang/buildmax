//go:build windows

package clie2e

import "testing"

// The approval path needs a pseudo-terminal, which this suite drives with a
// package that has no Windows implementation. Skipping loudly beats leaving
// the platform with a silently smaller suite.
func TestApprovalPathIsUnsupportedOnWindows(t *testing.T) {
	t.Skip("the terminal approval path is not covered on Windows: no pty support")
}
