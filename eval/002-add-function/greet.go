package greet

import "strings"

// Greet returns a personalised greeting for name.
func Greet(name string) string {
	return "Hello, " + strings.TrimSpace(name) + "!"
}
