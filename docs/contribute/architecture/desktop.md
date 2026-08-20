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
started a run), the run goroutine drains one queued prompt per turn and announces
each with `desktop/message-dequeued`, and `QueuedMessages` re-reads a project's
queue. `CancelRun` discards the queue before cancelling. See
[Queued messages](../../design/queued-messages.md).

Project metadata is stored in
`<BUILDMAX_HOME>/projects/projects.json`. Sessions, settings, traces, auth, and
logs use the regular paths under `BUILDMAX_HOME`; project source files stay in
the user-selected folder.

## Build Boundary

`desktop/frontend` is a nested Go module so root Go commands do not traverse
npm packages. Vite outputs to `desktop/dist`, outside that nested module, which
allows `desktop/assets_embed.go` to embed it.

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
