//go:build desktop

// Package desktop holds the desktop app frontend assets (embedded for production build).
// The UI is a React app in desktop/frontend/; Vite builds it to frontend/dist/. This
// file embeds that output so the Wails binary can serve it.
//
// Embedding is behind the `desktop` build tag because //go:embed fails at compile
// time when frontend/dist is missing, and that directory is a build artifact that
// is not checked in. Without the tag, assets_stub.go is compiled instead, which
// keeps `go build ./...`, `go vet ./...`, and `go test ./...` working on a fresh
// clone. See assets_stub.go.
package desktop

import "embed"

// Assets is the embedded frontend build output (Vite dist), served by the Wails app.
// Requires desktop/frontend/dist to exist; `wails build -tags desktop` produces it
// first, and ./make build runs the same sequence.
//
//go:embed all:frontend/dist
var Assets embed.FS

// Embedded reports whether this binary carries the frontend assets.
const Embedded = true
