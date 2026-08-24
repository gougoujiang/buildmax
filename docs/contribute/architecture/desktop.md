# Desktop

> **Audience:** contributors · **Status:** current
>
> Build and usage notes: [`cmd/buildmax-desktop/README.md`](../../../cmd/buildmax-desktop/README.md)

## Purpose

The Desktop app is the native local surface for the same agent runtime used by
the CLI. Wails hosts a React frontend and binds it to
`internal/interface/desktop.App`; it does not call the Portal backend for local
chat execution.

The desktop `Project` type is a named local folder used to group sessions. It
is not the server's team, issue, or project domain model.

## Modes

Desktop runs in one of two modes, the same two the CLI has:

| Mode | What it is |
|---|---|
| `local` | The agent runs here against the models in `settings.yaml`. No server. |
| `server` | The same local agent, plus a signed-in BuildMax account — managed models, and the bridge to a team's work. |

Neither mode changes where the agent runs: chat is always executed locally by
`agentapp`. A server adds identity and the models it manages, which is why
Portal login is a connector rather than a gate — see
[surface positioning](../../design/surface-positioning.md).

`GetAuthStatus` returns the signed-in account, and there is no mode field: the
credentials are the mode. A stored login in `<BUILDMAX_HOME>/auth.json` reports
`server`, no login reports `local`, and nothing is remembered alongside them,
because a second record of one fact is a second source of truth for it. A login
the server no longer honours reports expired: the app stays in managed mode and
refuses to run rather than quietly using local models, leaving signing in again
and signing out as the two ways forward. `Logout` revokes the session and
removes the credentials, and that removal is the whole switch back to local. See
[client modes](../../design/client-modes.md) sections 3 and 8.

## Layers

| Path | Responsibility |
|---|---|
| `cmd/buildmax-desktop/main.go` | Thin process entry point, logging, embedded-asset guard |
| `internal/interface/desktop` | Wails lifecycle, Go bindings, project/session state, streaming and approvals |
| `desktop/frontend` | React UI and generated Wails bindings |
| `desktop/assets_embed.go` | Production frontend embed under the `desktop` build tag |
| `cmd/buildmax-desktop/wails.json` | Wails build configuration |

`App` creates one `agentapp.AgentApp` lazily per project folder and caches it
for the process lifetime. Each project also has one interactive approval
handler and at most one in-flight run. Shutdown cancels active runs and closes
all cached runtimes.

## Data And Runtime Flow

1. The frontend calls a generated Wails binding such as `SendMessageStream`.
2. `App` resolves the local project and creates its shared `AgentApp` when
   needed.
3. The core run emits LLM, tool, usage, and stream events.
4. The bridge forwards those events through Wails (`desktop/*` event names).
5. The React frontend renders deltas and returns approval decisions through
   `RespondApproval`.
6. Session persistence and durable traces are handled by `agentapp`, exactly as
   for the CLI.

At most one run per project is in flight. A prompt submitted while one is running
is queued: `SendMessageStream` returns its 1-based queue position (0 means it
started a run), and `QueuedMessages` re-reads a project's queue. The queue is
passed to the run as `RunPromptOpts.Pending`, so a queued prompt usually joins the
turn in progress at its next iteration boundary; the run goroutine's turn loop
picks up anything queued after that. Either way the frontend hears
`desktop/message-dequeued`, and `desktop/message-blocked` when a hook refuses one
— a refusal that leaves the run itself going. `CancelRun` discards the queue
before cancelling. See
[Queued messages](../../design/queued-messages.md).

## Session Ownership

Desktop holds no session between calls. A run opens one, owns it for its whole
life including queued prompts, and releases it before it emits
`desktop/stream-done` — so a frontend acting on that event finds the session
free. Everything else either reads without the writer lock (`GetSession`,
`GetRunStatus`, `GetHistoryPoints`) or opens transiently and closes again
(`RewindSession`, `ForkSession`).

That is what makes "not while a run is in flight" enforce itself rather than
needing a flag: a history move takes the writer lock, and a run holding it is
how the move discovers it cannot proceed. The bindings translate that into a
message naming the session as busy. Neither fires a session lifecycle hook —
nothing is starting or ending when a user edits history, and the transient open
is an artifact of Desktop's ownership model, not an event a hook should see.

Project metadata is stored in
`<BUILDMAX_HOME>/projects/projects.json`. Sessions, settings, traces, auth, and
logs use the regular paths under `BUILDMAX_HOME`; project source files stay in
the user-selected folder.

## Build Boundary

`desktop/frontend` is a nested Go module so root Go commands do not traverse
npm packages. Vite outputs to `desktop/dist`, outside that nested module, which
allows `desktop/assets_embed.go` to embed it.

`desktop/frontend` must dedupe react, react-dom, and react/jsx-runtime in its
Vite config. `@buildmax/gui` is a symlinked `file:` dependency that externalises
react and has react installed as a peer, so without dedupe the bundle carries
two React instances, the hook dispatcher of the one that rendered is null, and
the window opens blank. `internal/architecture` has a test for it.

The embed file is guarded by the `desktop` build tag. Without the tag, a stub
keeps `go build ./...`, `go vet ./...`, and `go test ./...` valid on a fresh
clone. A runnable native app must be built with the tag; `./make build` performs
the frontend build and invokes the Wails version pinned by `go.mod`.

## Change Checklist

- Keep Wails bindings in `internal/interface/desktop`; do not assemble another
  agent runtime in the frontend or command package.
- Keep shared presentation in `gui` and Desktop-specific state in
  `desktop/frontend`.
- Preserve JSON field names and the project file format when changing persisted
  desktop state.
- Test Go bridge changes with `./make test`; test frontend changes with
  `./make check desktop`; use `./make build` for the native packaging boundary.

## Related

- [Overview](overview.md)
- [Agent loop](agent-loop.md)
- [Session](session.md)
- [CLI](cli.md)
- [Shared GUI and frontends](../repo-layout.md#frontends)
