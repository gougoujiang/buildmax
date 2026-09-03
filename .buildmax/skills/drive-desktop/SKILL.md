---
name: drive-desktop
description: "Drive the running Desktop app's UI ad hoc: launch a browser against its live dev server, click, type, screenshot, read bound Go methods. Use when asked to poke at, click through, or screenshot the Desktop app while working on it — not for a pass/fail check, use `./make e2e desktop-ui` for that."
---

# Drive Desktop

Ad hoc, agent-driven exploration of desktop/frontend's real UI: the same
`wails dev` browser bridge `./make e2e desktop-ui` scripts a fixed check
against (see docs/contribute/testing.md), but here as a REPL you can send
one command at a time and read the result of each before deciding the next —
click into a view, type into a field, screenshot it, read what a bound Go
method returns.

This is not a pass/fail suite. For "does the Desktop UI still work," run
`./make e2e desktop-ui` instead — same dev server, but scripted and torn down
automatically. Reach for this skill instead when the question is "what does
this actually look like" or "does this flow work," and you do not yet know
what to assert.

## Setup — two processes

**1. Start the dev server** (foreground, own terminal or tmux pane):

```bash
./make run desktop-dev
```

Wait for `Using DevServer URL: http://localhost:34115` in its output. This
runs against `./testing-sandbox`, the same BUILDMAX_HOME every other `./make
run` target uses — never your real `~/.buildmax`. It also opens a native
window as a side effect of `wails dev` itself; ignore it, the driver below
talks to the HTTP bridge, not that window.

**2. Start the driver** (a second terminal or tmux pane):

```bash
node .buildmax/skills/drive-desktop/driver.mjs
```

It resolves Playwright from `desktop/frontend/node_modules`, so that has to be
installed first — `npm --prefix desktop/frontend ci` if `./make e2e
desktop-ui` or `./make check desktop` has not already put it there. If no
Chromium is cached, `npm --prefix desktop/frontend exec -- playwright install
chromium` once.

Type `launch`, then a command per line. Wrap the driver in tmux for an agent
to drive: send a line with `tmux send-keys`, wait for its output with
`tmux capture-pane`, then send the next — the driver serializes commands
internally, but you still want to see each result before deciding the next
one.

## Commands

| command | what it does |
|---|---|
| `launch` | open a browser page at the dev server, print its title |
| `ss [name]` | screenshot → `/tmp/buildmax-desktop-shots/<name>.png` (override: `SCREENSHOT_DIR`) |
| `click <css-sel>` | click an element via the DOM, not Playwright's coordinate hit-testing |
| `click-text <text>` | click the first button/link/`[role=button]` whose text matches |
| `type <text>` | keyboard-type into whatever has focus |
| `press <key>` | press one key (`Enter`, `Escape`, ...) |
| `wait <css-sel>` | wait up to 10s for a selector to appear |
| `eval <js>` | evaluate an expression in the page, print it as JSON |
| `text [css-sel]` | print `innerText` of a selector, or the whole body |
| `bindings` | list the bound Go methods `window.go.desktop.App` actually exposes |
| `reload` | reload the page (picks up a `wails dev` hot-reload without relaunching) |
| `quit` | close the browser, exit |

## Gotchas

- **Fresh sandbox has no models and nobody signed in.** The app boots straight
  into "local mode" (see desktop/frontend/src/LoginPage.jsx and the gating in
  App.jsx) rather than the login screen — that is not a bug to route around,
  it is the default state. Sign-in is reachable from the UI if you need it.
- **`window.go` is the tell for a broken bridge.** If `bindings` reports zero
  methods, or the page shows "Run this app with Wails", the dev server is not
  injecting the runtime — check `./make run desktop-dev`'s own output before
  suspecting the driver.
- **A native window also opens.** `wails dev` starts it as a side effect;
  closing that window does not stop the dev server, and leaving it open does
  not affect the driver. Stop the dev server itself (Ctrl-C) when done.
