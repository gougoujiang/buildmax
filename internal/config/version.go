package config

// Version is the BuildMax application version.
var Version = "0.0.7"

// Commit is the short git commit SHA the binary was built from.
// Injected at build time via -ldflags "-X buildmax/internal/config.Commit=<sha>".
// Defaults to "dev" for non-build-script builds (e.g. plain `go build`, `go run`, `go test`).
var Commit = "dev"

// VersionString returns the human-readable version string, e.g. "0.0.7 (abc1234)".
func VersionString() string {
	if Commit == "" {
		return Version
	}
	return Version + " (" + Commit + ")"
}
