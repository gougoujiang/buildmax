# Testing

> **Audience:** contributors and code-changing agents · **Status:** current

What to run after a change, what it needs, and where to look when it fails.
The reasoning behind this split — why end-to-end suites are a local feedback
loop rather than a pull-request gate — is in
[../design/end-to-end-testing.md](../design/end-to-end-testing.md).

## The Short Version

```bash
./make test           # everything below the deployment: unit, integration, CLI, Desktop bridge
./make test ./internal/tool -run TestX   # one package or one test, same isolated home
./make e2e cli        # just the CLI and TUI suite
./make e2e desktop    # just the Desktop bridge suite
./make e2e local      # Portal in a browser, against a Compose stack this command owns
./make e2e all        # cli, desktop, then local — the release-time matrix
```

`./make test` is the loop. The CLI and Desktop suites live inside it because
they run in seconds and need nothing but a temporary directory; a suite you
have to remember to run is one that stops being run.

Narrow it with `./make test`, not with a bare `go test`. Only the task runner
sets `BUILDMAX_HOME`, and a test that reaches the real `~/.buildmax` reads the
contributor's own sessions, settings, and credentials — so its result depends on
whose machine it runs on. `config.DataDir` panics rather than fall back, and a
package whose code reads those paths gives itself a `TestMain` calling
`testsupport.RunWithIsolatedHome`.

## Which Suite For What You Changed

| You changed | Run |
|---|---|
| Anything in `internal/`, `cmd/` | `./make test` |
| The agent loop, tools, permissions, sessions, the TUI | `./make test`, then `./make e2e cli` |
| Plugins, packaging, or the Marketplace routes | `./make test`, then `./make e2e cli` |
| The Desktop bridge, its events, approvals, or session history | `./make e2e desktop` |
| A shared component in `gui/` | `./make check gui` |
| Portal, `gui`, or a route Portal calls | `./make e2e local` |
| Server, worker, scheduler, storage, or the model gateway | `./make compose smoke`, and `./make compose smoke managed` if the change touches the gateway |
| Deployment manifests, the Dockerfiles, ingress, or the worker's Kubernetes path | `./make kind up`, then `./make e2e` |
| Documentation | `./make check docs` |

Start with the narrowest suite the change touches. Run a broader one when the
narrow one passes and you still do not believe it — not instead of reading the
failure you already have.

## What Each Suite Needs, And How Long It Takes

| Suite | Needs | Normal | Owns |
|---|---|---|---|
| `./make e2e cli` | Go | under 60 s | a temporary `BUILDMAX_HOME`, a workspace, and a Marketplace server it starts in process |
| `./make e2e desktop` | Go | under 60 s | the same |
| `./make e2e local` | Docker, Node, Chromium | under 10 min | a Compose stack it starts and stops |
| `./make e2e compose` | a Compose stack already running | under 2 min | nothing — it is a guest |
| `./make e2e kind` | a kind cluster already running | under 2 min | nothing — it is a guest |
| `./make compose smoke` | Docker | under 5 min | a Compose stack it leaves running |
| `./make kind up` | Docker, kubectl | under 20 min | a cluster it leaves running |

No suite needs a provider API key. Every one of them answers the model from
`internal/testsupport/mockllm`, which replays a committed scenario.

`./make agent-smoke` is the exception, and it is not a test: it drives the
agent's tools with a real model, needs a key, and reports a PASS/FAIL table the
model wrote about itself. Read its output; its exit code says only that the
process finished.

`./make cache-qualify` is the second exception, for the same reason and a
sharper one. Every cache test in the tree runs against a fake upstream, which
proves what BuildMax sends and nothing about what a provider does with it — and
a request can be perfectly shaped while the provider declines to cache it, for a
minimum prefix length, an unsupported model, or an expired retention window.
The suite runs the scenarios
[prompt-cache-control.md](../design/prompt-cache-control.md) gates on against a
real provider named by `BUILDMAX_CACHE_QUALIFY_*`, and no provider or gateway is
described as cache-capable until it passes. Unset, it skips like the
MySQL-backed store tests do.

`./make eval` is the third, and it measures something the suites deliberately do
not. It builds the CLI and evaluates it as a black box against the tasks in
`evaluation/suite/`: a real model, repeated trials, graders that read the final
workspace and the run's trace, and a report of pass rate with its uncertainty.
That is agent quality, not boundary verification — a suite above proves a
behavior is wired, and evaluation asks how reliably a model drives it. It needs
a key and spends tokens. Everything it can check without one — task validity,
oracles, graders, and the adapter — runs in `./make test` instead, so a task
that measures nothing is caught before it costs anything. See
[design/evaluation-system.md](../design/evaluation-system.md).

If a prerequisite is missing, the suite says which one before it starts. The two
that catch people out:

```bash
npm --prefix portal ci                                  # Portal test dependencies
npm --prefix portal exec -- playwright install chromium # the browser itself
```

## Attached Or Owned

A Portal suite either attaches to a deployment or owns one, and it says which
before it starts.

- **Owned** (`./make e2e local`) starts a Compose stack, tests it, and takes it
  down. It refuses to adopt a stack that is already running, because taking one
  down that someone else started is not its call.
- **Attached** (`./make e2e compose`, `./make e2e kind`) is a guest. It uses the
  fixed diagnostic account `deployment-smoke@buildmax.local`, creates only
  uniquely tagged resources, and prints what it left behind — most of them have
  no delete route, so the line naming them is the cleanup instruction.

## When A Suite Fails

Everything a browser suite produces goes to one place, cleared at the start of
every run:

```text
.artifacts/e2e/portal/
├── run.txt      the deployment, the run id, and the command that reproduces it
└── results/     Playwright traces, screenshots, and error context per failed test
```

Open a trace with `npx playwright show-trace <path>`. For the Go suites the
failure is the test output: each one prints what it expected, what it saw, and —
for the terminal and Desktop suites — the whole screen or event stream it had
at that point.

Read the artifact before changing code. A failing end-to-end test names a
boundary; guessing at the boundary from the test name is how a real defect
becomes a weakened assertion.

## What CI Runs

| Trigger | What runs |
|---|---|
| Pull request | The fast checks in `ci.yml`: build, vet, lint, tests, frontend builds, licenses |
| Merge to `main` | The deployment smoke and the Portal browser suites, on Compose and kind |
| Daily schedule | The same, so a regression no merge caused is still found |
| Manual dispatch | Either, for release preparation or a suspected environment regression |

End-to-end verification is deliberately not a pull-request gate. A post-merge
failure is triaged by the author of the merge that broke it, and that merge is
reverted if it is not fixed within one working day. A test that fails
intermittently is quarantined the same day rather than retried.

Every post-merge run reports each suite as passed, failed, cancelled, or skipped
by policy with the reason. A skipped suite is never evidence that a journey
passed.

## Before A Release

```bash
./make check ci   # everything a pull request runs, except the Windows job
./make e2e all    # cli, desktop, then a browser run against a stack it owns
```

`./make e2e all` deliberately leaves out kind: that suite needs a cluster, and a
release check that quietly builds one is a surprise. Run `./make kind up`
followed by `./make e2e` when the change is worth proving on Kubernetes.

## Frontend Component Tests

`gui/` and `desktop/frontend` run their component tests under vitest with a
jsdom document, so a test asserts what a viewer can see and which key does what
rather than what a function returned. `gui` is where a shared component's own
behaviour is proved once, for both surfaces; a surface's test covers only its
own wiring to that component. Portal keeps browser-level assertions in
Playwright, where a real engine is the point.

```bash
./make check gui               # build the package, type-check, run its tests
npm --prefix gui test          # the tests alone, while iterating
```

`npm test` in `gui` type-checks before it runs: `tsconfig.json` excludes test
files so no `.d.ts` for one reaches `dist/`, and `tsconfig.test.json` puts them
back for the check.

## Adding A Test

Put it at the lowest level that can prove the claim. A display decision belongs
in a frontend unit test, a service contract in a Go test, a handler rule in an
HTTP test. Reach for an end-to-end suite only when the outcome depends on
several of those cooperating — a real binary, a real deployment, a browser — and
say in the test what that boundary is. The design record's §6 lists the paths
that qualify and, for the deployment ones, the exact fact no handler test can
reach.
