//go:build bindings

package main

// generatingBindings marks the throwaway binary that Wails compiles with
// `go build -tags bindings` and runs to dump the app's JS/TS bindings. That run
// never opens a window and never serves assets — see internal/app/app_bindings.go
// in the wails module, whose App.Run only walks the bound structs and exits.
//
// It matters here because Wails deletes the `desktop` tag from that build:
// pkg/commands/bindings/bindings.go does
//
//	genModuleTags := lo.Without(tags, "desktop", "production", "debug", "dev")
//
// treating `desktop` as one of its own reserved mode tags. This project happens
// to use the same name for the frontend-embed tag, so the bindings binary always
// compiles against desktop/assets_stub.go with Embedded == false. Without this
// flag, main's missing-frontend guard would exit 1 and fail every `wails build`.
const generatingBindings = true
