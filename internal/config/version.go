package config

// Version and Commit identify the build. Both are injected at link time so the
// git tag stays the single source of truth for what a binary calls itself:
//
//	-ldflags "-X github.com/gougoujiang/buildmax/internal/config.Version=0.1.0 \
//	          -X github.com/gougoujiang/buildmax/internal/config.Commit=abc1234"
//
// ./make build and the release pipeline both set them. A plain `go build`,
// `go run`, or `go test` leaves the defaults, which is why they read "dev"
// rather than a version number that would be wrong the moment it is committed.
var (
	// Version is the BuildMax application version, without a leading "v".
	Version = "dev"

	// Commit is the short git commit SHA the binary was built from.
	Commit = "dev"
)

// VersionString returns the human-readable version string, e.g. "0.1.0 (abc1234)".
func VersionString() string {
	if Commit == "" {
		return Version
	}
	return Version + " (" + Commit + ")"
}
