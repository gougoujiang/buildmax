---
name: commit-and-push
description: Commits current changes with an AI-generated commit message and pushes to the remote branch. Use when the user wants to commit and push, save changes to git, generate a commit message from the diff, or push local commits to remote.
---

# Commit and Push

Helps commit current changes, generate a commit message from the diff, and push to the remote branch.

## Workflow

1. **Inspect changes** – Run `git status` and `git diff` (or `git diff --staged` if already staged) to see what will be committed.
2. **Generate commit message** – From the diff, write a single commit message in conventional format (see below).
IMPORTANT:
do NOT add "co-authored-by " or similar messages
3. **Stage and commit** – Run `git add` as needed, then `git commit -m "<message>"`. Request **git_write** when running these commands.
4. **Push** – Run `git push` (or `git push origin <branch>`). Request **git_write** and **network** if the tool requires it.

## Commit message format

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <short description>

[optional body]
```

**Types:** `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `perf`, `ci`, `build`.

**Examples:**

- `feat(agent): add tool-call handling`
- `fix(llm): correct API error mapping`
- `docs: update README setup steps`
- `chore(deps): bump go version`

Keep the subject line under ~72 characters; start with a verb in imperative mood.

## Permissions

- **git_write** – Required for `git add`, `git commit`, and `git push`.
- **network** – Required for `git push` when the execution environment restricts network access.

## Checklist before committing

- [ ] No unintended files (e.g. secrets, build artifacts) in the diff; adjust `.gitignore` or unstage if needed.
- [ ] One logical change per commit; split into multiple commits if the diff mixes unrelated changes.
- [ ] Message matches the actual change set.
