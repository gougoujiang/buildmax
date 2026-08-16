# Public Launch Checklist

> **Audience:** maintainers · **Status:** current

Use this checklist once, immediately before and after changing the GitHub
repository visibility from Private to Public. Do not create the first release
tag until every post-switch setting has been verified.

## Before Changing Visibility

- [ ] Confirm the local `main` worktree is clean and matches `origin/main`.
- [ ] Confirm there are no unreviewed open pull requests intended for launch.
- [ ] Confirm the latest required CI run passes.
- [ ] Scan the complete Git history for credentials and inspect author emails,
      private filesystem paths, internal hostnames, and customer identifiers.
- [ ] Review `README.md`, `SECURITY.md`, `.github/SUPPORT.md`, and the installation guide
      for promises that are inappropriate for an alpha.
- [ ] Review `docs/start/support.md` so the support matrix and non-goals match
      the release being published.
- [ ] Run the non-publishing release snapshot and native archive smoke jobs.
- [ ] Confirm the owner accepts that all commits, tags, issues, and repository
      metadata become publicly visible immediately.

### Prepare The On-Ramp

An empty issue tracker tells a first-time visitor there is nothing to do. Do
this before the switch, not after:

- [ ] Create the labels [CONTRIBUTING.md](../../CONTRIBUTING.md) points people
      at: `good first issue`, `help wanted`, `documentation`. Add `bug`,
      `enhancement`, and `question` if they are not already there.
- [ ] Open five to ten genuinely small, independent issues and label them. Good
      sources: documentation that has drifted from the code, missing tests for
      behavior that already works, CLI or TUI rough edges found while running
      the quickstart, and error messages that do not say what to do next.
      Each one should name the file to change and how to verify the fix.
- [ ] Walk [first-pr.md](first-pr.md) once on a clean clone and fix whatever
      does not match reality.

## Change Visibility

In GitHub, open **Settings → General → Danger Zone → Change repository
visibility**, choose **Public**, and complete GitHub's confirmation flow. This is
an owner decision and should not be delegated to release automation.

## Immediately After The Switch

- [ ] Enable private vulnerability reporting.
- [ ] Enable secret scanning and push protection.
- [ ] Protect `main`: require the selected CI checks, block force pushes, and
      block branch deletion.
- [ ] Apply the project's chosen review policy. A single-maintainer project
      should not require an approval that no independent reviewer can provide.
- [ ] Enable required CODEOWNERS review only when an eligible second reviewer
      exists.
- [ ] Confirm Dependabot alerts and automatic security updates remain enabled.
- [ ] Confirm Discussions, issue templates, labels, topics, and the default
      branch are still correct.
- [ ] Confirm GitHub's Documentation link resolves from the default branch.
- [ ] Confirm the community health files render from `.github/` — the Code of
      Conduct and Support links should appear on the repository's Community
      Standards page and in the new-issue chooser.
- [ ] Confirm the private reporting form `SECURITY.md` points reporters to is
      actually reachable from the Security tab.
- [ ] Confirm the CodeQL workflow starts running; it skips itself while the
      repository is private.
- [ ] Clone the public repository without credentials and repeat the quickstart.

## First Alpha Release

- [ ] Choose and document the version, for example `v0.1.0-alpha.1`.
- [ ] Follow [releasing.md](releasing.md) to create and push the tag.
- [ ] Verify archives, checksums, SBOMs, attestations, and the GHCR image.
- [ ] Confirm the release is marked as a prerelease and does not move `latest`.
- [ ] Open the README, documentation, Discussions, and security reporting links
      from a signed-out browser session.
