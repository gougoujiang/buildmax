# Governance

BuildMax uses a maintainer-led governance model while the project is in alpha.
The current maintainers are listed in [MAINTAINERS.md](MAINTAINERS.md).

## Roles

- **Contributors** open issues, propose changes, review code, improve docs, and
  participate in technical discussions.
- **Reviewers** are contributors trusted to provide informed review in an area.
  Review does not by itself grant merge or release access.
- **Maintainers** triage work, merge pull requests, manage releases, coordinate
  security response, and administer the repository.

## Decisions

Routine implementation decisions are made in the pull request that introduces
the change. Significant changes to public behavior, architecture, persistence,
security boundaries, or project direction should begin with an issue or design
record so alternatives and compatibility impact are visible before code lands.

Maintainers seek consensus and explain decisions in the relevant issue or pull
request. If consensus cannot be reached, the project owner makes the final
decision and records the reasoning. Security reports follow
[SECURITY.md](../SECURITY.md) and are never decided in a public issue before a
coordinated disclosure.

## Merging Changes

A pull request may merge when:

- its scope and user impact are clear
- required CI checks pass
- tests and documentation match the change
- unresolved review comments are addressed
- a maintainer approves the result

Maintainer-authored changes should receive independent review when another
qualified reviewer is available. Urgent security fixes may merge before public
review; the maintainer must document the exception when disclosure is safe.

Merges are squashes: one commit on `main` per pull request, its subject taken
from the pull request title. A maintainer merging a pull request is responsible
for that subject reading well in `git log` — edit it at merge time rather than
letting a vague title land. Keep the reviewer-facing half of the description out
of the commit body.

Direct pushes to `main` are reserved for repository administration or recovery.
Normal code and documentation changes should use pull requests.

## Issue Triage

Maintainers triage issues on a best-effort basis. Triage confirms that the
report has enough information to act on, applies an appropriate type and area
label, links duplicates, and records whether the issue fits the active roadmap.
Security reports never enter public triage; they follow [SECURITY.md](../SECURITY.md).

Issues are not closed merely because they are old. A maintainer may close an
issue when it is a duplicate, cannot be reproduced, falls outside project
scope, or is waiting on requested reporter information for at least 30 days.
The closing comment must explain why. An issue closed for missing information
may be reopened when that information is supplied.

Accepted work may remain open without an assigned milestone. Labels such as
`good first issue` and `help wanted` mean that the scope is understood and
outside contributions are welcome; they are not promises of a response time.

## Releases

Only maintainers may create release tags or publish project artifacts. A
release must come from `main`, pass CI, follow the documented release process,
and describe known limitations appropriate to its stability level.

## Changes to Governance

Governance changes use the same pull request process as code changes. Material
changes should be announced in the pull request and reflected in
[MAINTAINERS.md](MAINTAINERS.md) or [SUPPORT.md](SUPPORT.md) where applicable.
