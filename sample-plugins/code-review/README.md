# Code Review

A sample BuildMax plugin that packages a structured code-review workflow.

It contributes:

- `skills/review/` — a checklist-driven review skill covering correctness,
  readability, tests, and security.
- `agents/reviewer.md` — a read-only reviewer subagent that inspects a diff and
  reports findings without modifying files.

This plugin ships **instruction-only** content (skills and subagents), so a Space
can activate it for background runs.

## Try it

```bash
buildmax plugin validate ./sample-plugins/code-review
buildmax plugin publish ./sample-plugins/code-review
```
