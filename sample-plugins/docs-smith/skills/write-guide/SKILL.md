---
name: write-guide
description: "Draft or revise task-oriented user documentation. Use when writing a how-to, guide, or README section for end users."
---

# Write Guide

Write documentation that helps a reader accomplish a task, not documentation that
describes the software.

## Principles

- **Lead with the task.** A section title is what the reader wants to do
  ("Install a plugin"), not the feature's name.
- **Show the shortest path first.** Give the command or steps that work for the
  common case before the options and caveats.
- **One idea per paragraph.** If a paragraph needs "and also", split it.
- **Prefer the imperative.** "Run `buildmax plugin list`" beats "You can run…".
- **Show, then explain.** A code block, then one line on what it does and why.

## Shape Of A How-To

1. A one-line statement of what the reader will have when they finish.
2. Prerequisites, only if there are real ones.
3. Numbered steps, each a single action.
4. How to verify it worked.
5. What to do when it did not.

## Revising

When editing existing docs, preserve the author's voice, cut words that carry no
information, and never invent behavior the code does not have — if a claim cannot
be verified, flag it rather than polishing it.
