# Commit Helper

A sample BuildMax plugin with a single skill that turns a staged diff into a
well-formed [Conventional Commits](https://www.conventionalcommits.org) message.

It contributes:

- `skills/commit-message/` — reads the staged change and drafts a commit subject
  and body.

Skill-only, so a Space can activate it for background runs.

## Try it

```bash
buildmax plugin validate ./sample-plugins/commit-helper
buildmax plugin publish ./sample-plugins/commit-helper
```
