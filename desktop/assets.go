// Package desktop holds the desktop app frontend assets (embedded for production build).
// The UI is a React app in desktop/frontend/; Vite builds to frontend/dist/. This
// package embeds that dist so the Wails binary can serve it. Build order: run
// "npm run build" in desktop/frontend (or use "wails build", which does that first).
package desktop

import "embed"

// Assets is the embedded frontend build output (Vite dist). Used by the Wails desktop app.
// Requires desktop/frontend/dist to exist (create with: cd desktop/frontend && npm run build).
//
//go:embed all:frontend/dist
var Assets embed.FS
