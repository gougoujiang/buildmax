# Contributor Documentation

> **Audience:** contributors · **Status:** current

How the code is arranged and how to keep the documentation honest. For the
contribution process itself — prerequisites, build and test commands, code
boundaries, pull request expectations — start at
[CONTRIBUTING.md](../../CONTRIBUTING.md).

| Document | Covers |
|---|---|
| [first-pr.md](first-pr.md) | The whole path from clone to open pull request, for a first-time contributor |
| [conventions.md](conventions.md) | Persisted-data naming, table names, entity IDs, tool output, commit messages, changelog entries |
| [repo-layout.md](repo-layout.md) | The repository tree and dependency direction. **Single source of truth** — other docs link here instead of repeating it. |
| [testing.md](testing.md) | Which suite to run for a change, what each needs, where its artifacts land, and what CI runs when |
| [architecture/](architecture/README.md) | How each subsystem works today, one document per package or area |
| [documentation.md](documentation.md) | Documentation structure, conventions, and what to update when |
| [dependency-licenses.md](dependency-licenses.md) | Go and npm license audit, and how to re-run it |
| [releasing.md](releasing.md) | Versioning, release preparation, verification, and recovery |

Design records — the reasoning behind a decision, and plans not yet built —
live in [../design/](../design/README.md).
