//go:build !bindings

package main

// generatingBindings is false in every real build of the app. See bindings_on.go
// for what the other side of this flag is for.
const generatingBindings = false
