<!--
This PR will be squash-merged, so the TITLE above becomes the commit subject on
main: one imperative line that reads on its own in `git log --oneline`.

CONTRIBUTING.md has the full guidance. The short version: one concern per PR,
tests for behavioral changes, and documentation updated alongside the code.
-->

## What and why

<!-- The problem this solves, and the approach taken. -->

## Verification

<!--
What you actually ran, and what it showed. `./make test` and `./make build`
cover nearly everything CI checks. Say so explicitly if a change is untested
and why.
-->

## Notes for the reviewer

<!--
Remaining limitations, follow-up work, anything you are unsure about, and any
behavior change a user would notice. Delete this section if there is nothing.
-->

---

- [ ] Tests added or updated for behavioral changes
- [ ] Relevant local checks pass (`./make test`, frontend tests/builds, or both)
- [ ] Documentation updated: `docs/contribute/architecture/` for behavior or
      boundary changes, `docs/contribute/repo-layout.md` if a package moved,
      `docs/design/` for direction changes, `docs/guide/` + `docs/reference/` +
      `config-examples/` for user-facing configuration
- [ ] `CHANGELOG.md` entry added under `## [Unreleased]` if a user or operator
      would notice this change
- [ ] Dependency and license changes are explained above
- [ ] No credentials, customer data, or private deployment details in the diff
- [ ] Breaking changes to existing public behavior are called out above
