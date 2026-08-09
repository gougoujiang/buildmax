# Dependency Licenses

BuildMax ships under [Apache-2.0](../../LICENSE). This page records what its
dependencies are licensed under, and how to re-check.

Last audited 2026-08-09.

## Go

133 modules reachable from the three binaries in `cmd/`:

| License | Count |
|---|---|
| MIT | 59 |
| Apache-2.0 | 43 |
| BSD-3-Clause | 27 |
| BSD-2-Clause | 2 |
| MPL-2.0 | 1 |
| ISC | 1 |

All are permissive and compatible with distributing BuildMax under Apache-2.0.

The single MPL-2.0 dependency is `github.com/go-sql-driver/mysql`. MPL-2.0 is
file-level copyleft: it obliges you to publish modifications **to that
library's own files**, and it places no condition on the rest of the project.
BuildMax uses it unmodified, so nothing further is required. If you ever fork
or patch it in-tree, that fork's sources have to stay available under MPL-2.0.

## npm

The frontend packages (`gui/`, `portal/`, `desktop/frontend/`) resolve to MIT
and ISC only across their production dependencies.

License tools report `@buildmax/gui`, `buildmax-portal`, and
`buildmax-desktop-frontend` as `UNLICENSED`. That is an artifact of their
`"private": true` field, which suppresses the `"license": "Apache-2.0"` they
declare. They are this repository's own packages, not third-party code.

## Re-running the audit

Go, including a gate that fails on copyleft that would conflict with
redistribution:

```bash
go install github.com/google/go-licenses@latest
go-licenses report ./cmd/...
go-licenses check ./cmd/... --disallowed_types=forbidden,restricted
```

The `check` command runs in CI on every pull request, so a dependency carrying
a forbidden or restricted license fails the build rather than being noticed
later.

npm, per package directory:

```bash
cd portal && npx license-checker-rseidelsohn --production --summary
```

`go-licenses` prints a warning for `github.com/modern-go/reflect2` because the
module contains assembly it cannot follow for transitive dependencies. The
module itself is Apache-2.0; the warning is not a finding.

## Attribution in releases

Apache-2.0 §4(d) requires carrying forward the attribution notices of
Apache-2.0 dependencies when you redistribute them, and compiled BuildMax
binaries contain that dependency code.

[`scripts/gen-third-party-notices.sh`](../../scripts/gen-third-party-notices.sh)
collects the full license text of every module linked into the binaries into a
single `NOTICE-THIRD-PARTY` file — 135 modules at the last run. GoReleaser runs
it as a pre-build hook, so every release archive and the container image ship
it. The file is generated, not committed.

To produce it locally:

```bash
go install github.com/google/go-licenses@latest
./scripts/gen-third-party-notices.sh
```
