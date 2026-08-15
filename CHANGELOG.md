# Changelog

All notable changes to BuildMax are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
BuildMax is currently in alpha, so incompatible changes may still occur between
pre-releases and must be called out in release notes.

## [Unreleased]

### Added

- `buildmax init` writes a starter `settings.yaml`, optionally with the API key
  supplied on the command line.
- golangci-lint, govulncheck, and `-race` tests in CI, plus CodeQL analysis for
  Go and TypeScript that starts once the repository is public.
- ESLint for the Portal and desktop frontends, with `npm run lint` in each.
- Open source governance, support, maintainer, and conduct policies.
- Markdown, npm production license, configuration example, Git history secret
  scanning, and release snapshot checks in CI.
- SPDX SBOM generation, GitHub artifact attestations, and release image
  vulnerability scanning.
- Native Linux, macOS, and Windows release archive smoke checks, including
  checksum and required-content validation.
- A public launch checklist and documented issue triage policy.

### Changed

- `buildmax version` falls back to the Go build info, so a binary installed with
  `go install` reports its module version instead of `dev`.
- Startup tells a first-time user what to do: a missing configuration file, a
  file without models, and an unedited placeholder key are now three distinct
  messages instead of one.
- Go 1.26.6, `golang.org/x/net` 0.55.0, `golang.org/x/text` 0.39.0, and
  `goldmark` 1.7.17 — closing 23 vulnerabilities reachable from this code.
- Release tools are pinned to exact versions for reproducible builds.
- Release setup instructions now use the current YAML configuration flow.

### Removed

- Dead code across the agent runtime, storage, handlers, and CLI that the new
  linter surfaced.

## [0.1.0-alpha] - 2026-08-09

### Added

- Initial alpha release of the BuildMax CLI/TUI, server, and worker binaries.
- Linux, macOS, and Windows archives with checksums and third-party notices.
- Multi-architecture Linux container image published to GHCR.

[Unreleased]: https://github.com/gougoujiang/buildmax/compare/v0.1.0-alpha...HEAD
[0.1.0-alpha]: https://github.com/gougoujiang/buildmax/releases/tag/v0.1.0-alpha
