# Contributing to BuildMax

Thanks for helping improve BuildMax. Small, focused contributions are the
easiest to review and keep the shared runtime dependable across CLI, Desktop,
Portal, and worker execution.

## Before You Start

- Search existing issues and [design documents](docs/design/) before proposing a
  large change. [docs/README.md](docs/README.md) explains how the documentation
  is organized.
- Open an issue or discussion first for product changes, new runtime providers,
  new tools, or changes that affect security, persistence, or public APIs.
- Never include credentials, customer data, or private deployment details in an
  issue, pull request, test fixture, or commit.
- Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).

## Development

The primary local checks are:

```bash
./make test
./make build
```

`./make test` runs Go tests with `BUILDMAX_HOME=./testing-sandbox`. Frontend
packages can be built from their own directories; see [README.md](README.md),
[portal/README.md](portal/README.md), and [gui/README.md](gui/README.md).

Keep changes aligned with the existing boundaries:

- shared agent behavior belongs in `internal/core/agent` or `internal/agentapp`
- infrastructure adapters belong in `internal/infra`
- user-facing orchestration belongs in `internal/service` or `internal/interface`
- `internal/core` must not import application, infrastructure, or interface layers
- persisted JSON uses explicit `snake_case` field names

## Pull Requests

- Keep each pull request focused on one user-visible outcome or engineering concern.
- Add or update focused tests for behavioral changes.
- Update documentation alongside the code:
  - behavior or package boundaries change → update the matching document in
    [docs/architecture/](docs/architecture/)
  - direction changes → add a numbered document to [docs/design/](docs/design/),
    or move the superseded one to [docs/archive/](docs/archive/)
  - user-facing behavior or configuration changes → update the README and
    `config-examples/`
- Explain the problem, the approach, verification performed, and any remaining
  limitations in the pull request description.
- Preserve existing public behavior unless the pull request explicitly documents
  a breaking change.

## Contribution License

By submitting a contribution, you agree that it is your original work and that
you license it under the [Apache License 2.0](LICENSE). This is the default
contribution grant described in Section 5 of that license; no separate CLA is
required at this stage.
