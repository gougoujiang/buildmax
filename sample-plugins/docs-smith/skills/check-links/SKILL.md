---
name: check-links
description: "Find broken relative links in Markdown files. Use when reviewing docs or before publishing a docs change."
---

# Check Links

Find relative Markdown links that point at files which do not exist.

## Steps

1. Collect the Markdown files in scope with Glob (for example `docs/**/*.md`).
2. In each file, find inline links of the form `[text](path)` and reference-style
   link definitions.
3. Ignore absolute URLs (`http://`, `https://`, `mailto:`) and in-page anchors
   that start with `#` — this skill checks *relative file* links only.
4. For a link with an anchor (`guide/x.md#section`), resolve the file part.
5. Resolve each remaining path relative to the file it appears in and confirm the
   target exists with Read or Glob.

## Report

List each broken link as `source-file:line -> target`, grouped by source file.
If every link resolves, say so plainly. Do not rewrite links — report them so the
author decides the fix.
