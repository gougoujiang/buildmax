# Changelog

All notable changes to BuildMax are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
BuildMax is currently in alpha, so incompatible changes may still occur between
pre-releases and must be called out in release notes.

## [Unreleased]

### Added

- Open source governance, support, maintainer, and conduct policies.
- Markdown, npm production license, configuration example, Git history secret
  scanning, and release snapshot checks in CI.
- SPDX SBOM generation, GitHub artifact attestations, and release image
  vulnerability scanning.
- Native Linux, macOS, and Windows release archive smoke checks, including
  checksum and required-content validation.
- A public launch checklist and documented issue triage policy.

### Changed

- Release tools are pinned to exact versions for reproducible builds.
- Release setup instructions now use the current YAML configuration flow.

## [0.1.0-alpha] - 2026-08-09

### Added

- Initial alpha release of the BuildMax CLI/TUI, server, and worker binaries.
- Linux, macOS, and Windows archives with checksums and third-party notices.
- Multi-architecture Linux container image published to GHCR.

[Unreleased]: https://github.com/gougoujiang/buildmax/compare/v0.1.0-alpha...HEAD
[0.1.0-alpha]: https://github.com/gougoujiang/buildmax/releases/tag/v0.1.0-alpha
