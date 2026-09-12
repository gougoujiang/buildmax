# Exploratory Run Records

> **简体中文：** [阅读中文镜像](../../zh-CN/contribute/exploratory-runs/README.md)
> **Audience:** contributors and testing agents · **Status:** current

Dated records of [exploratory testing](../exploratory-testing.md) sessions.
Each file is one session's report, written from that guide's compact report
shape, committed so a finding and the way it was reached outlive the session's
working directory.

## What Belongs Here

Commit a report when a session produced something worth keeping: a confirmed or
suspected defect, a usability obstacle, a reusable charter, or evidence another
contributor will want. A session that found nothing of lasting interest stays in
the gitignored `.artifacts/` working directory and need not be committed — do
not manufacture a coverage record.

Write the report from the shape in
[exploratory-testing.md](../exploratory-testing.md) and inline the shortest
reproduction and the key observations. The raw captures
under `.artifacts/` are ephemeral; the committed report must stand on its own
once they are gone. Redact credentials, login codes, and unrelated private data
before committing.

## This Is A Record, Not A Work Queue

An actionable finding still becomes a fix pull request, a
[backlog](../../backlog/README.md) task, or a GitHub issue, following
[AGENTS.md](../../../AGENTS.md) — one item, one place. The report links to that
item as evidence and history; it does not track the item's status, so a report
never becomes a second place the work lives.

A report is a point-in-time record tied to the commit it names. Do not edit it
to follow later behavior — link the work item instead. Prune a report once its
findings are resolved and it is no longer illustrative; this project keeps
nothing that must persist.

## Conventions

- File name: `YYYY-MM-DD-<surface>-<slug>.md`, e.g.
  `2026-09-13-cli-first-use-continuity.md`. `<surface>` is `portal`, `desktop`,
  `cli`, `worker`, or similar.
- Reports are written in English, the repository artifact language. This index
  is mirrored in zh-CN; individual reports are not translated.
- Add a row below when you commit a report; remove it when you prune the report.

## Records

| Report | Surface | Summary |
|---|---|---|
| [2026-09-13-cli-first-use-continuity.md](2026-09-13-cli-first-use-continuity.md) | CLI | First-use and resume-a-session journey with a real model; continuity, `info`/`usage`, and error/empty states behave well; one low-impact message nuance; live TUI interrupt blocked by no PTY |
