<!--
This PR is merged with a merge commit: the TITLE above becomes the merge commit
subject, and every commit on the branch lands on main too. One imperative line
each, reading on its own in `git log --oneline`.

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
- [ ] Changelog entry added as a new file under `docs/changelog/` if a user or
      operator would notice this change
- [ ] Dependency and license changes are explained above
- [ ] No credentials, customer data, or private deployment details in the diff
- [ ] Breaking changes to existing public behavior are called out above
