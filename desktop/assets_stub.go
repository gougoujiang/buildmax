//go:build !desktop

// Package desktop holds the desktop app frontend assets. This file is the
// default build: it declares the same API as assets_embed.go but embeds nothing,
// so the package compiles without desktop/dist present.
//
// That matters because desktop/dist is a Vite build artifact that is not checked
// in. If //go:embed referenced it unconditionally, every `go build ./...`,
// `go vet ./...`, and `go test ./...` would fail on a fresh clone until someone
// built the frontend.
//
// Real desktop binaries are built with `-tags desktop`, which selects
// assets_embed.go instead.
package desktop

import "embed"

// Assets is empty in this build. See assets_embed.go for the real asset bundle.
var Assets embed.FS

// Embedded reports whether this binary carries the frontend assets. Callers
// should refuse to start a UI when this is false rather than serve a blank window.
const Embedded = false
