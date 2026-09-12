# CLI first-use and continuity — 2026-09-13

> **简体中文：** [阅读中文镜像](../../zh-CN/contribute/exploratory-runs/2026-09-13-cli-first-use-continuity.md)

**Charter.** Journey: CLI first-use plus "interrupt work and continue later".
Rationale: single-binary CLI first experience and recoverability are core
promises. Role: new user, isolated `BUILDMAX_HOME=./testing-sandbox`, disposable
workspace. Success: a user can discover required configuration, submit work, and
continue a prior session understanding what was saved. Scope: local only; allowed
mutations were the gitignored `.local/`, `testing-sandbox/`, and `.artifacts/`.
Time spent: ~30 min.

**Environment.** Commit `9298f240` (worktree clean). Build: `./make build cli`
→ `bin/buildmax`, `v0.2.0-alpha.10-38-g9298f240`. Surface: built CLI run with
`BUILDMAX_HOME=./testing-sandbox`. Local mode, not signed in. Starting data:
fresh sandbox. Owned resources: `.local/`, `testing-sandbox/`, `.artifacts/`
(all gitignored, left in place).

**Model.** Real. `./make run cli` copied the contributor's real
`~/.buildmax/settings.yaml` into the sandbox on first use (documented behavior),
providing OpenRouter-backed models; default `openai/gpt-5.6-luna`. Trials kept
tiny. Total model time ~3.3s, 7,135 in / 26 out tokens, cost 0.000997 USD.
Prompts contained only a test codeword; no private data.

**Explored** (action → observation → next question):

1. `buildmax doctor` → all OK, one expected warning (sandbox disabled for local),
   lists configured models, prints the exact next command. Config is discoverable.
2. `buildmax -p "Remember this codeword: ARTICHOKE-42. Reply with only ACK."` →
   `ACK`; session `576bd48b` saved; cost/token footer shown. Can it be continued?
3. `buildmax -c -p "What was the codeword? Reply with only the codeword."` →
   `ARTICHOKE-42`; same session id; cumulative spend and cache savings shown.
   Continuity works with a real model, and prompt caching engaged.
4. `buildmax info` (no args) → rich returning-user summary: first message,
   session id, workspace, spend, context-window use, work counts, project memory
   path. Satisfies "what stopped / what was saved".
5. `buildmax usage` → per-day rollup, consistent with `info` totals.
6. Empty home, `buildmax -c -p hi` → exit 1, message `no sessions yet …; start
   one with -p PROMPT or the TUI`.
7. Configured home, `buildmax -r 00000000-0000-0000-0000-000000000000 -p hi` →
   exit 4, `error: session not found: 0000…`.
8. Configured home, `buildmax -r not-a-uuid -p hi` → exit 4, `error: session not
   found: not-a-uuid`.

**Findings.**

- **Malformed resume id reported as "not found"** — usability, low impact, high
  confidence. `-r not-a-uuid` returns the same `session not found: not-a-uuid`
  as a well-formed but missing id, while `--help` / `--session-id` state the
  value "must be a valid UUID". Expected (basis: consistency with that documented
  constraint): distinguish "not a valid session id" from "session not found".
  Repro: `BUILDMAX_HOME=<configured> buildmax -r not-a-uuid -p hi`. No functional
  loss; not filed as a work item pending maintainer triage.

**Positive observations** (not defects): exit codes are distinct and meaningful
(empty continue = 1, invalid resume = 4); prompt caching engaged on resume
(`info` reported ~50% served from cache); sessions are scoped by project/cwd
independent of `BUILDMAX_HOME` (the empty-state message showed the cwd project
path under a fresh home).

**Not exercised / blocked.** The interactive TUI could not be driven — the shell
had no PTY (`bubbletea: could not open TTY: /dev/tty`). So the live interrupt
path (Ctrl+C mid-generation, reopening a session in the TUI, in-TUI history
navigation) is untested; continuity was proven through `-p` print mode only.
Captured output would not prove TUI focus, key handling, or layout in any case,
so no weak substitute was recorded. A PTY-capable terminal is needed to close
this branch.

**Cleanup.** No long-running processes started; all commands were one-shot.
`testing-sandbox/` and `.local/` are persistent gitignored development state
left in place by design; the sandbox holds a copy of the user's real
`settings.yaml` (own machine, gitignored), not shared. Tracked worktree files
unchanged.

**Follow-up.** No product defect warranting a backlog item. Optional polish for
the malformed-resume-id message. No regression added; nothing here authorizes
broadening work or claiming readiness.
