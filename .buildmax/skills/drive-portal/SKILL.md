---
name: drive-portal
description: "Drive Portal's real browser UI ad hoc against a running kind or Compose deployment: sign in with a freshly minted login code, click, type, screenshot, read console errors. Use when asked to poke at, click through, or screenshot Portal while working on a feature — not for a pass/fail check, use `./make e2e {local,kind,compose}` for that."
---

# Drive Portal

Ad hoc, agent-driven exploration of Portal's real UI: the same login-code
flow and browser `./make e2e {local,kind,compose}` scripts as a fixed
Playwright suite (see docs/contribute/testing.md), but here as a REPL you can
send one command at a time and read the result of each before deciding the
next — sign in, open the view a change touched, click into it, screenshot it,
read whatever the console logged.

This is not a pass/fail suite. For "does Portal still work," run
`./make e2e kind` (or `local`/`compose`) instead — same login flow, but
scripted, asserted, and torn down automatically. Reach for this skill instead
when the question is "what does this actually look like" or "does this flow
work," and you do not yet know what to assert — typically right after
`./make kind reload` has picked up a code change and before writing the
Playwright spec for it.

## Setup — one process, against an already-running deployment

Unlike Desktop, Portal has no separate dev server this skill starts for you.
Bring up a deployment first, then point the driver at it:

```bash
./make kind up            # or: ./make compose smoke
./make kind reload server # after a code change, before driving it
```

Both serve Portal at `http://localhost:8080` by default — override with
`BUILDMAX_PORTAL_URL` if the cluster or Compose project used a different
port.

Start the driver:

```bash
node .buildmax/skills/drive-portal/driver.mjs
```

It resolves Playwright from `portal/node_modules`, so that has to be
installed first — `npm --prefix portal ci` if `./make e2e kind` or `./make
check portal` has not already put it there. If no Chromium is cached, `npm
--prefix portal exec -- playwright install chromium` once.

Type `launch`, then a command per line. Wrap the driver in tmux for an agent
to drive: send a line with `tmux send-keys`, wait for its output with
`tmux capture-pane`, then send the next — the driver serializes commands
internally, but you still want to see each result before deciding the next
one.

## Commands

| command | what it does |
|---|---|
| `launch` | open a browser page at `BUILDMAX_PORTAL_URL`, print its title |
| `login [email]` | mint a fresh code with `./make kind login`, sign in through the login-code form |
| `ss [name]` | screenshot → `/tmp/buildmax-portal-shots/<name>.png` (override: `SCREENSHOT_DIR`) |
| `click <css-sel>` | click an element (a real Playwright locator — waits for it to be actionable) |
| `click-text <text>` | click the first element whose text contains `<text>` |
| `label <text>` | focus the input behind an accessible label (then `type` into it) |
| `type <text>` | keyboard-type into whatever has focus |
| `press <key>` | press one key (`Enter`, `Escape`, ...) |
| `wait <css-sel>` | wait up to 10s for a selector to appear |
| `wait-text <text>` | wait up to 10s for text to appear anywhere on the page |
| `url` | print the current page URL |
| `eval <js>` | evaluate an expression in the page, print it as JSON |
| `text [css-sel]` | print `innerText` of a selector, or the whole body |
| `console [errors]` | print captured console messages; `console errors` filters to `console.error`/uncaught exceptions |
| `reload` | reload the page, clearing captured console messages |
| `quit` | close the browser, exit |

## A representative walkthrough

```
launch
login
wait-text Dashboard
ss after-login
click-text Issues
wait-text New issue
ss issues-list
console errors
```

## Gotchas

- **A login code is single-use and out of band on purpose.** `login` calls
  `./make kind login [email]` itself — you never need to run that separately
  or paste a code by hand. Default email is the deployment-smoke account;
  the account is created if it does not exist yet, so `login` works right
  after `kind up` with no prior `kind smoke` run.
- **React controlled inputs.** A raw `eval el.value = '...'` does not fire
  React's `onChange`. Use `label` to focus the field by its accessible label,
  then `type` — same reason `login` uses `getByLabel(...).fill(...)`-style
  interaction rather than `eval`.
- **Portal's own specs (`portal/e2e/*.spec.ts`) are the reference for
  selectors** — most views are driven by `getByRole`/`getByLabel`/`getByText`
  there, not CSS classes; skim the closest spec before guessing a selector.
- **Websockets / long-poll.** `wait` and `wait-text` target the element you
  actually need; there is no generic "network idle" wait, because a live
  conversation or task view never goes idle.
- **Check `console errors` before declaring success.** A page can render its
  shell while every data fetch fails.
