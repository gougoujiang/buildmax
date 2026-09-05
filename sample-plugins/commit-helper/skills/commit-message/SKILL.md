---
name: commit-message
description: "Draft a Conventional Commits message from the staged diff. Use when the user is about to commit and wants a clear subject and body."
---

# Commit Message

Draft a commit message for the currently staged change, following the
[Conventional Commits](https://www.conventionalcommits.org) format.

## Steps

1. Read the staged change with `git diff --cached`. If nothing is staged, say so
   and stop — there is nothing to describe.
2. Decide the type from what changed: `feat`, `fix`, `docs`, `refactor`,
   `test`, `perf`, `build`, `ci`, or `chore`.
3. Add a scope in parentheses when one package or area clearly owns the change.
4. Write the subject: imperative mood, no trailing period, under ~72 characters.

## Format

```text
<type>(<scope>): <subject>

<body: what changed and why, wrapped at ~72 columns>
```

## Rules

- The subject says what the change *does*, not what you did ("Add retry", not
  "Added retry").
- Use the body to explain *why* when the reason is not obvious from the diff.
- Mark a breaking change with `!` after the type/scope and a `BREAKING CHANGE:`
  footer.
- Never invent a change the diff does not show.
