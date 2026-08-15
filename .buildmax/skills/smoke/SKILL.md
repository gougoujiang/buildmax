---
name: smoke
description: "Smoke test for BuildMax agent tools. Levels: 0 (read-only), 1 (file I/O), 2 (edge cases + network)."
---

# Smoke Test

Run a structured smoke test to verify the agent's built-in tools work correctly.

## Usage

```
/smoke [level]
```

Default level is **0** if omitted. Each level is cumulative: level 1 includes all level 0 checks, level 2 includes all level 1 checks.

| Level | Scope |
|-------|-------|
| 0 | Read-only: Bash, Read, Glob, Grep, TodoWrite |
| 1 | Level 0 + file mutation: Write, Edit |
| 2 | Level 1 + edge cases + WebFetch |

---

## Execution Instructions

Work through every check for the requested level in order. For each check, call the tool, observe the result, and record PASS or FAIL with a one-line note. Do not stop on failure — run all checks.

Use `smoke-test-tmp/` (relative to workspace root) as the scratch directory for file tests. Clean it up at the end of level 1 and level 2.

At the end, print the summary table (see "Report Format" below).

---

## Level 0 — Read-Only Tools

### B0: Bash basic output
- Call `Bash` with command `echo smoke-ok`
- **Pass** if output contains `smoke-ok`

### B1: Bash exit code on success
- Call `Bash` with command `true`
- **Pass** if result is empty string or has no "failed" text

### B2: Bash non-zero exit
- Call `Bash` with command `exit 42`
- **Pass** if output contains `exit code 42`

### R0: Read known file
- Call `Read` with `file_path=go.mod`
- **Pass** if output contains `module github.com/gougoujiang/buildmax`

### R1: Read with offset and limit
- Call `Read` with `file_path=go.mod`, `offset=1`, `limit=2`
- **Pass** if exactly 2 lines are returned (no more, no less) and first line contains `module`

### G0: Glob finds files
- Call `Glob` with `pattern=*.mod`
- **Pass** if output contains `go.mod`

### G1: Glob no match
- Call `Glob` with `pattern=*.nonexistent_xyz`
- **Pass** if output is `No files matched the pattern.`

### P0: Grep finds pattern
- Call `Grep` with `pattern=module github.com/gougoujiang/buildmax`, `path=go.mod`, `output_mode=content`
- **Pass** if output contains `module github.com/gougoujiang/buildmax`

### P1: Grep files_with_matches mode
- Call `Grep` with `pattern=module`, `path=go.mod`, `output_mode=files_with_matches`
- **Pass** if output contains `go.mod` and does not show line content

### P2: Grep no match
- Call `Grep` with `pattern=XYZZY_NO_MATCH_9999`, `path=go.mod`
- **Pass** if output is `No matches found.`

### T0: TodoWrite formats list
- Call `TodoWrite` with todos:
  - `{content: "Step one", status: "completed"}`
  - `{content: "Step two", status: "in_progress", active_form: "Running"}`
  - `{content: "Step three", status: "pending"}`
- **Pass** if output contains `completed`, `in_progress`, `pending`, and all three content strings

### T1: TodoWrite empty list
- Call `TodoWrite` with `todos=[]`
- **Pass** if output is `Todo list (0 items).`

---

## Level 1 — File Mutation

Run all Level 0 checks first, then:

### W0: Write creates file
- Call `Write` with `file_path=smoke-test-tmp/hello.txt`, `content=smoke test line one\nline two`
- **Pass** if output is `File written successfully.`

### W1: Read back written file
- Call `Read` with `file_path=smoke-test-tmp/hello.txt`
- **Pass** if output contains `smoke test line one` and `line two`

### W2: Write overwrites existing file
- Call `Write` with `file_path=smoke-test-tmp/hello.txt`, `content=overwritten content`
- Call `Read` with `file_path=smoke-test-tmp/hello.txt`
- **Pass** if read output is `overwritten content` and does not contain `smoke test line one`

### E0: Edit replaces string
- Call `Write` with `file_path=smoke-test-tmp/edit-target.txt`, `content=alpha beta gamma`
- Call `Edit` with `file_path=smoke-test-tmp/edit-target.txt`, `old_string=beta`, `new_string=BETA`
- Call `Read` with `file_path=smoke-test-tmp/edit-target.txt`
- **Pass** if read output is `alpha BETA gamma`

### E1: Edit replace_all
- Call `Write` with `file_path=smoke-test-tmp/multi.txt`, `content=x x x`
- Call `Edit` with `file_path=smoke-test-tmp/multi.txt`, `old_string=x`, `new_string=y`, `replace_all=true`
- Call `Read` with `file_path=smoke-test-tmp/multi.txt`
- **Pass** if output is `y y y`

### G2: Glob finds written file
- Call `Glob` with `pattern=smoke-test-tmp/*.txt`
- **Pass** if output contains `smoke-test-tmp/` and at least one `.txt` file

### P3: Grep finds content in written file
- Call `Grep` with `pattern=BETA`, `path=smoke-test-tmp/edit-target.txt`, `output_mode=content`
- **Pass** if output contains `BETA`

### B3: Bash operates in workspace
- Call `Bash` with command `ls smoke-test-tmp/`
- **Pass** if output lists at least one `.txt` file

### Cleanup
- Call `Bash` with command `find ./smoke-test-tmp -depth -delete`
- **Pass** if command exits without error (output is empty or success)

---

## Level 2 — Edge Cases + Network

Run all Level 0 and Level 1 checks first (including cleanup), then:

### S0: Read path outside root is rejected
- Call `Read` with `file_path=../../etc/passwd` (or any `../..` escape)
- **Pass** if result is an error containing `outside` or `not allowed` (tool returns an error, not file contents)

### S1: Read non-existent file
- Call `Read` with `file_path=smoke-test-tmp/does-not-exist.txt`
- **Pass** if result contains `not found` or `no such file`

### S2: Edit on non-existent file
- Call `Edit` with `file_path=smoke-test-tmp/ghost.txt`, `old_string=x`, `new_string=y`
- **Pass** if result contains `not found` or `no such file`

### S3: Edit with old_string not found
- Call `Write` with `file_path=smoke-test-tmp/s3.txt`, `content=hello world`
- Call `Edit` with `file_path=smoke-test-tmp/s3.txt`, `old_string=NOPE`, `new_string=x`
- **Pass** if result contains `not found`

### S4: Edit with duplicate old_string (no replace_all)
- Call `Write` with `file_path=smoke-test-tmp/s4.txt`, `content=a a`
- Call `Edit` with `file_path=smoke-test-tmp/s4.txt`, `old_string=a`, `new_string=b`, `replace_all=false`
- **Pass** if result contains `not unique`

### S5: Glob invalid pattern
- Call `Glob` with `pattern=[invalid`
- **Pass** if result contains `invalid` (pattern rejected, not a crash)

### N0: WebFetch returns content
- Call `WebFetch` with `url=https://httpbin.org/get`
- **Pass** if response body is non-empty and contains `"url"` (JSON field from httpbin)
- **Skip** (mark as SKIP, not FAIL) if network is unavailable or request times out

### Cleanup
- Call `Bash` with command `find ./smoke-test-tmp -depth -delete`

---

## Report Format

After all checks, print this table:

```
SMOKE <level>: <PASS|FAIL>

Check  | Result | Note
-------|--------|-----
B0     | PASS   |
B1     | PASS   |
...
S5     | FAIL   | returned "foo" instead of error containing "not unique"

Total: <N> checks, <P> passed, <F> failed, <S> skipped
```

If all checks pass, print:

```
SMOKE <level>: PASS — all <N> checks passed
```

If any check fails, print:

```
SMOKE <level>: FAIL — <F> of <N> checks failed
```
