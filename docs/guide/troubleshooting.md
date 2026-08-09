# Troubleshooting

> **Audience:** users and operators · **Status:** current

## `No model configured. Add a model to …`

There is no `models:` entry in `settings.yaml`. BuildMax does **not** read an
API key from the environment — earlier versions did, and some older
documentation still says so. Create the file as shown in
[start/quickstart.md](../start/quickstart.md).

The message prints the exact path it looked at; if that path is not what you
expect, `BUILDMAX_HOME` is set somewhere.

## Runs fail in bursts with HTTP 429

Free OpenRouter models rate-limit aggressively. This shows up as runs that work
individually but fail when you run several in a row, or when an agent makes many
tool-calling round trips. Switch to a paid model in `settings.yaml`, or slow
down.

## `POST /api/login` returns 503

Expected, and safe. There is no OTP delivery channel, so login is disabled
unless you set a development code. See
[deploy/authentication.md](../deploy/authentication.md) — and read what that
code does before enabling it.

## Tasks stay `PENDING` and never run

The scheduler claimed the run but could not start a worker. Check, in order:

1. `buildmax-worker` is on `PATH` or next to the server binary, matching
   `worker.binary` in `server.yaml`
2. `worker.server_url` is reachable **from the worker**, which is not always the
   same address the server binds
3. `worker.token` is set — without it the worker cannot call `/api/worker/*`
4. `workspaces_dir` exists and is writable by both processes
5. The `storage:` block is reachable from the worker, which talks to blob
   storage directly rather than through the server

The server log names the step that failed.

## Webhook returns 400

The prompt could not be extracted from the request body. `webhook.message_path`
in `server.yaml` must match your payload's shape — the default is `message`, and
a nested field is written `body.text`. See
[reference/webhook.md](../reference/webhook.md).

## The sandbox will not turn on

```bash
buildmax sandbox deps      # is bwrap / sandbox-exec / socat present?
buildmax sandbox status    # what is actually resolved, and from which layer
```

The sandbox needs Seatbelt on macOS or `bwrap` on Linux/WSL2. It is
**unavailable on native Windows**. If `status` shows a value you did not set,
check `<BUILDMAX_HOME>/policy.yaml` and `BUILDMAX_SANDBOX_ENABLED` — both
override `settings.yaml`. See [sandbox.md](sandbox.md).

## A hook never fires

Almost always the `matcher`. It is a regex against the **tool name**, and the
names are capitalized: `Bash`, `Write`, `Edit`, `Read`, `Grep` — not `bash` or
`writefile`. Check the current names with `/tools`, or in
[tools.md](tools.md).

Also: `matcher` only applies to `pre_tool_use`, `post_tool_use`, and
`post_tool_use_failure`. On other events it is ignored.

Remember hooks **fail open** — a hook that times out or errors allows the
action. Silence can mean "ran and failed", not "did not run".

## The desktop app refuses to start

It was built without the `desktop` build tag, so no frontend bundle is embedded.
The binary tells you this rather than opening a blank window. Build with
`./make build`, which builds the frontend and passes the tag.

## `go test ./...` behaves differently from `./make test`

`./make test` sets `BUILDMAX_HOME=./testing-sandbox` so tests never touch your
real data directory. Run it rather than bare `go test` — some tests assume that
isolation.

## Bash tool behaves oddly on Windows

Native Windows has no sandbox, and the bash tool falls back to `cmd /c`. Windows
is not covered end to end by CI: the Windows job builds and vets, but the test
suite there is advisory. WSL2 is the supported path for anything involving the
shell, `./make setup`, or deployment.

## Something else

The trace tells you what the run actually did — every LLM call, every tool call,
what was denied, and how it ended:

```bash
ls -t ~/.buildmax/traces/<session-id>/ | head -1
```

See [sessions-and-traces.md](sessions-and-traces.md). Logs are file-only, under
`<BUILDMAX_HOME>/logs/buildmax.log`; raise the detail with `log_level: debug` in
`settings.yaml`.
