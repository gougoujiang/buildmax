---
name: smoke
description: "Smoke test for BuildMax agent tools. Levels: 0 (read-only + session state), 1 (file I/O), 2 (edge cases, permissions, network), 3 (delegation, background jobs, MCP)."
---

# Smoke Test

Run a structured smoke test to verify the agent's built-in tools work correctly.

## Usage

```
/smoke [level]
```

Default level is **0** if omitted. Each level is cumulative: level 1 includes all
level 0 checks, and so on.

| Level | Scope | Needs |
|-------|-------|-------|
| 0 | Read-only and session state: Bash, Read, Glob, Grep, TodoWrite, NoteWrite, Skill | nothing |
| 1 | Level 0 + file mutation: Write, Edit | nothing |
| 2 | Level 1 + edge cases, the permission boundary, WebFetch | network for one check |
| 3 | Level 2 + delegation and background work: Task, background Bash, JobList, JobOutput, JobStop, Monitor, MCP gateway | TUI or Desktop, and model calls |

`./make agent-smoke` drives **level 0** in headless `-p` mode, so level 0 must
stay free of anything that needs an interactive surface, background jobs, or a
sub-agent turn. Put those in level 3.

## Reading The Result

A real model writes this report, so the report is a claim, not evidence. Check
the claim against the `Tool calls:` line the CLI prints when the run ends:

- Level 0 is 22 checks and costs **at least 23 tool calls** — one to load this
  skill, then one or more per check.
- A run that reports 22 passes off 1 tool call did not run anything. It read
  this file and wrote a plausible table from it.

That is not hypothetical. Small models do exactly this: `gemini-2.5-flash-lite`
produces a clean, correctly formatted, entirely fabricated PASS in one call.
Point `agent-smoke` at a model strong enough to execute a 22-step tool
sequence, and read the call count before believing the table.

## Which Tools Exist Where

Not every tool is registered on every surface, and a missing tool is a fact
about the surface, not a failure:

- `JobList`, `JobOutput`, `JobStop`, `Monitor`, and the `run_in_background`
  parameter on `Bash` and `Task` exist only where local background jobs are
  enabled — the TUI and Desktop — and never inside a sub-agent.
- `UploadArtifact` exists only where the surface has an artifact service. The
  CLI and TUI have none, so it is out of scope here.
- `LoadMcpTools` and `CallMcpTool` exist only when MCP servers are configured.
  This repository configures one in `.buildmax/mcp.json`.

If a level 3 check's tool is absent on the surface you are running, mark it
**SKIP** with the surface as the note. Do not mark it FAIL.

---

## Execution Instructions

Work through every check for the requested level in order. For each check, call
the tool, observe the result, and record PASS or FAIL with a one-line note. Do
not stop on failure — run all checks.

Never report a check you did not actually call. This file describes what each
tool should do, so a plausible report can be written from it without touching a
single tool — and a smoke test that reports a result it did not observe is worse
than no smoke test, because it retires the suspicion that would have caught the
regression. A level 0 run makes at least one tool call per check; if your report
claims 22 passes off fewer calls than that, it is fabricated.

Use `smoke-test-tmp/` (relative to workspace root) as the scratch directory for
file tests. Clean it up at the end of every level that created it.

Cleanup uses `find`, not `rm`: `rm` is on the risky-command list and would ask
for approval, which stalls a headless run.

At the end, print the summary table (see "Report Format" below).

---

## Level 0 — Read-Only And Session State

### B0: Bash basic output
- Call `Bash` with command `echo smoke-ok`
- **Pass** if output contains `smoke-ok`

### B1: Bash exit code on success
- Call `Bash` with command `true`
- **Pass** if result is empty string or has no "failed" text

### B2: Bash non-zero exit
- Call `Bash` with command `exit 42`
- **Pass** if output contains `Command failed with exit code 42.`

### B3: Bash runs in the workspace root
- Call `Bash` with command `ls go.mod AGENTS.md`
- **Pass** if output lists both files

### R0: Read known file
- Call `Read` with `file_path=go.mod`
- **Pass** if output contains `module github.com/gougoujiang/buildmax`

### R1: Read with offset and limit
- Call `Read` with `file_path=go.mod`, `offset=1`, `limit=2`
- **Pass** if the first line contains `module`, exactly 2 file lines are
  returned, and the result ends with a `(showing lines 1-2 of N; use
  offset/limit to read more)` note

### R2: Read offset past end of file
- Call `Read` with `file_path=go.mod`, `offset=100000`
- **Pass** if output says the requested offset is past the end and reports the
  file's line count — not an error, and not file content

### G0: Glob finds files
- Call `Glob` with `pattern=*.mod`
- **Pass** if output is one absolute path per line and one of them ends in
  `/go.mod`

### G1: Glob no match
- Call `Glob` with `pattern=*.nonexistent_xyz`
- **Pass** if output is `No files matched the pattern.`

### P0: Grep content mode
- Call `Grep` with `pattern=module github.com/gougoujiang/buildmax`,
  `path=go.mod`, `output_mode=content`
- **Pass** if output names the file on its own line, then the matching line
  prefixed with its line number and `:` — line numbers are on by default

### P1: Grep files_with_matches mode
- Call `Grep` with `pattern=module`, `path=go.mod`,
  `output_mode=files_with_matches`
- **Pass** if output is the absolute path of `go.mod` and shows no line content

### P2: Grep count mode
- Call `Grep` with `pattern=require`, `path=go.mod`, `output_mode=count`
- **Pass** if output is `<path>: <N>` with N ≥ 1

### P3: Grep can suppress line numbers
- Call `Grep` with `pattern=^module`, `path=go.mod`, `output_mode=content`,
  `line_numbers=false`
- **Pass** if the matching line is prefixed with a bare `:` and no line number
- Passing `line_numbers=true` here would prove nothing: that is the default,
  so P0 already covers it. This check is the one that exercises the option.

### P4: Grep head_limit caps results
- Call `Grep` with `pattern=.`, `path=go.mod`, `output_mode=content`,
  `head_limit=2`
- **Pass** if at most 2 matching lines are returned

### P5: Grep no match
- Call `Grep` with `pattern=XYZZY_NO_MATCH_9999`, `path=go.mod`
- **Pass** if output is `No matches found.`

### T0: TodoWrite formats list
- Call `TodoWrite` with todos:
  - `{content: "Step one", status: "completed"}`
  - `{content: "Step two", status: "in_progress", active_form: "Running"}`
  - `{content: "Step three", status: "pending"}`
- **Pass** if output starts `Todo list (3 items):` and contains `completed`,
  `in_progress`, `pending`, and all three content strings

### T1: TodoWrite rejects two in_progress
- Call `TodoWrite` with two todos both `status: "in_progress"`
- **Pass** if the call fails with a message about only one task being in progress

### T2: TodoWrite empty list clears it
- Call `TodoWrite` with `todos=[]`
- **Pass** if output starts with `Todo list (0 items).`
- A run with no durable task list appends a note saying the list was not
  stored. That is still a PASS; record which one you saw.

### D0: NoteWrite stores notes
- Call `NoteWrite` with `notes=["smoke check: notes are reachable"]`
- **Pass** if output is `Stored 1 note:` followed by the note text
- A run with no note store answers that nothing was stored instead. That is
  still a PASS; record which one you saw, because it tells you whether this
  surface keeps durable session state.

### D1: NoteWrite rejects an over-long note
- Call `NoteWrite` with one note of more than 200 characters
- **Pass** if the call fails with a message naming the 200-character limit

### D2: NoteWrite clears notes
- Call `NoteWrite` with `notes=[]`
- **Pass** if output says notes were cleared, or that the run stores none
- Leave the session's notes cleared: this check exists so the smoke run does
  not leave its own bookkeeping in the session state.

### K0: Skill reports the workspace skills
- Call `Skill` with `skill=no-such-skill-xyz`
- **Pass** if the call fails with `unknown skill` and the message lists the
  available skills, including `smoke` and `vibe`
- Do not invoke a real skill as a positive check. Loading another skill's
  instructions changes what the agent does for the rest of the run, and
  invoking `smoke` re-enters this file.

---

## Level 1 — File Mutation

Run all Level 0 checks first, then:

### W0: Write creates file
- Call `Write` with `file_path=smoke-test-tmp/hello.txt`, `content=smoke test line one\nline two`
- **Pass** if output is `File written successfully.`
- Writing into a directory that does not exist yet is part of the check: the
  tool creates the parent directories.

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

### E2: Edit deletes content with an empty new_string
- Call `Write` with `file_path=smoke-test-tmp/del.txt`, `content=keep DROP keep`
- Call `Edit` with `file_path=smoke-test-tmp/del.txt`, `old_string=DROP `, `new_string=`
- Call `Read` with `file_path=smoke-test-tmp/del.txt`
- **Pass** if output is `keep keep`

### G2: Glob finds written file
- Call `Glob` with `pattern=smoke-test-tmp/*.txt`
- **Pass** if output lists absolute paths under `smoke-test-tmp/` and includes
  at least one `.txt` file

### P6: Grep finds content in written file
- Call `Grep` with `pattern=BETA`, `path=smoke-test-tmp/edit-target.txt`, `output_mode=content`
- **Pass** if output contains `BETA`

### B4: Bash sees the written files
- Call `Bash` with command `ls smoke-test-tmp/`
- **Pass** if output lists at least one `.txt` file

### Cleanup
- Call `Bash` with command `find ./smoke-test-tmp -depth -delete`
- **Pass** if command exits without error (output is empty or success)

---

## Level 2 — Edge Cases, Permissions, Network

Run all Level 0 and Level 1 checks first (including cleanup), then:

### S0: Read path outside root is rejected
- Call `Read` with `file_path=../../etc/passwd` (or any `../..` escape)
- **Pass** if the call fails with `path outside allowed root` — an error, not
  file contents

### S1: Read non-existent file
- Call `Read` with `file_path=smoke-test-tmp/does-not-exist.txt`
- **Pass** if the call fails with `file not found`

### S2: Edit on non-existent file
- Call `Edit` with `file_path=smoke-test-tmp/ghost.txt`, `old_string=x`, `new_string=y`
- **Pass** if the call fails with `file not found`

### S3: Edit with old_string not found
- Call `Write` with `file_path=smoke-test-tmp/s3.txt`, `content=hello world`
- Call `Edit` with `file_path=smoke-test-tmp/s3.txt`, `old_string=NOPE`, `new_string=x`
- **Pass** if the failure says `old_string not found`

### S4: Edit with duplicate old_string (no replace_all)
- Call `Write` with `file_path=smoke-test-tmp/s4.txt`, `content=a a`
- Call `Edit` with `file_path=smoke-test-tmp/s4.txt`, `old_string=a`, `new_string=b`, `replace_all=false`
- **Pass** if the failure contains `not unique`

### S5: Glob invalid pattern
- Call `Glob` with `pattern=[invalid`
- **Pass** if the failure is `invalid glob pattern` (pattern rejected, not a crash)

### S6: Reading a credential-shaped file needs approval
- Call `Bash` with command
  `mkdir -p smoke-test-tmp && printf 'SMOKE_FAKE_TOKEN=not-a-real-secret\n' > smoke-test-tmp/.env`
- Call `Read` with `file_path=smoke-test-tmp/.env`
- **Pass** if the read asked for approval (interactive surface) or was refused
  with `denied by policy` (headless surface)
- **Fail** if the contents came back with no gate at all
- On an interactive surface this prompts. Deny it, and record that it prompted.
- The file is created with `Bash`, not `Write`: `Write` gates credential-shaped
  paths on the same rule, so setting the check up with it would trip the gate
  before the check runs.

### S7: A risky Bash command passes through the permission gate
- Call `Bash` with command `chmod 644 smoke-test-tmp/s3.txt`
- **Pass** if the call asked for approval (interactive surface) or was refused
  with `denied by policy` (headless surface, where an unanswerable ask
  collapses to a deny)
- **Fail** if it simply ran with no gate at all
- `chmod` is on the risky-command list, and the command itself is harmless
  whichever way you answer. Do not smoke-test the catastrophic tier by typing a
  destructive command: those are denied before a process is spawned, but an
  agent following written instructions should never be handed one, and a docs
  test enforces that this file contains none.

### F0: WebFetch returns content
- Call `WebFetch` with `url=https://example.com`
- **Pass** if the response is non-empty and contains `Example Domain`
- **Skip** (mark as SKIP, not FAIL) if network is unavailable or the request
  times out

### F1: WebFetch with a prompt summarizes
- Call `WebFetch` with `url=https://example.com`,
  `prompt=Reply with only the page's main heading.`
- **Pass** if the reply is a short answer naming the heading rather than the
  whole page
- **Skip** if F0 was skipped, or if the surface has no LLM client for WebFetch
- The URL is cached for 15 minutes, so this reuses F0's fetch rather than
  hitting the network again.

### Cleanup
- Call `Bash` with command `find ./smoke-test-tmp -depth -delete`

---

## Level 3 — Delegation, Background Work, MCP

Run all Level 0 to Level 2 checks first (including cleanup), then:

These checks need a surface with background jobs (TUI or Desktop) and they
spend real model calls on sub-agent turns. Mark **SKIP** with the reason for any
tool the surface does not register — see "Which Tools Exist Where" above.

### X0: Task delegates to a sub-agent
- Call `Task` with `subagent_type=explore`, `description=smoke delegation check`,
  `prompt=Report the module path declared in go.mod at the workspace root. Answer with the module path and nothing else.`
- **Pass** if the reply contains `github.com/gougoujiang/buildmax`

### X1: Task rejects an unknown sub-agent type
- Call `Task` with `subagent_type=no-such-agent-xyz`, `description=bad type`,
  `prompt=noop`
- **Pass** if the call fails and the message lists the valid types, which
  include the built-ins `general`, `explore`, and `shell` plus this
  workspace's `sample-researcher`

### J0: Bash starts a background job
- Call `Bash` with command `sleep 5; echo background-done`,
  `run_in_background=true`
- **Pass** if the result reports `Started background job jb_...` and returns
  immediately rather than after 5 seconds
- Keep the returned job ID for J1 and J2.

### J1: JobList shows the running job
- Call `JobList`
- **Pass** if the output row for J0's job ID shows a running state and its
  command

### J2: JobOutput reads the job's output
- Call `JobOutput` with `job_id=` the ID from J0
- **Pass** if the output reports `job:`, `state:`, and a `--- stdout ---`
  section, and returns a `--- next_cursor: N ---` line
- Read it again after the job finishes and confirm it reports
  `background-done` and a finished state.

### J3: JobStop stops a job
- Call `Bash` with command `sleep 120`, `run_in_background=true`
- Call `JobStop` with that job ID
- **Pass** if the result says a stop was requested, and a later `JobOutput`
  reports a canceled state

### J4: JobOutput on an unknown ID answers rather than errors
- Call `JobOutput` with `job_id=jb_nosuchjob`
- **Pass** if the result is `No such job: jb_nosuchjob.` plus the pointer to
  `JobList` — a plain answer, not a tool error

### J5: Monitor starts a watcher
- Call `Monitor` with `command=tail -F smoke-test-tmp/watch.log`,
  `description=smoke watcher`, after creating that file with `Write`
- **Pass** if the result reports `Started monitor job jb_...` and names its
  delivery mode
- Stop it with `JobStop` and clean up `smoke-test-tmp/`.

### M0: LoadMcpTools lists the workspace server's tools
- Call `LoadMcpTools` with `server=mcp`
- **Pass** if the result names the `echo`, `text_stats`, and `sum_numbers`
  tools
- The server is started with `go run`, so the first call may take several
  seconds.

### M1: CallMcpTool calls through the gateway
- Call `CallMcpTool` with `server=mcp`, `tool_name=echo`,
  `arguments={"message": "smoke-mcp-ok"}`
- **Pass** if the result contains `smoke-mcp-ok`
- The `echo` tool declares no `readOnlyHint`, so it counts as a write:
  interactive surfaces ask before it runs, and autonomous ones refuse it.
  Being asked is a PASS — approve it and record that it prompted. A refusal on
  an autonomous surface is a SKIP, not a FAIL.

### Cleanup
- Call `JobStop` on any job this level started that is still running
- Call `Bash` with command `find ./smoke-test-tmp -depth -delete`

---

## Report Format

After all checks, print this table. One row per check ID — do not collapse
`B0`–`B3` into a single range row, because a range hides which member of it
failed:

```
SMOKE <level>: <PASS|FAIL>

Check  | Result | Note
-------|--------|-----
B0     | PASS   |
B1     | PASS   |
...
S5     | FAIL   | returned "foo" instead of an error containing "invalid glob pattern"
M1     | SKIP   | autonomous surface refuses MCP write tools

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

A skipped check does not fail the run, but the report must say which tool was
skipped and on what surface — a level 3 run on the CLI that silently reported
PASS would be claiming coverage it never had.
