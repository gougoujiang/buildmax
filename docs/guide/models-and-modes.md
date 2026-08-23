# Models And Modes

> **Audience:** users · **Status:** current

BuildMax runs in one of two modes, and one command switches between them.

| | Local mode | Managed mode |
|---|---|---|
| You are | signed out | signed in |
| Models come from | `settings.yaml` on this machine | the deployment you signed in to |
| Provider credentials | yours, in `settings.yaml` | the deployment's; never sent to you |
| Prompts and tool results go | straight to each provider | to that deployment |

```bash
buildmax models       # which mode you are in, and what it offers
buildmax login        # switch to a deployment's models
buildmax logout       # switch back to settings.yaml
```

Nothing else configures this. There is no per-model setting for it, because a
session is in one mode or the other and every prompt in it goes to the same
place.

## Local mode

The default, and the whole product without a server: `settings.yaml` lists the
models, each with its own endpoint and API key, and the agent calls them from
this machine. Start one with
[`buildmax init`](../start/quickstart.md), and see
[reference/configuration.md](../reference/configuration.md) for the fields.

`default_model` names which entry a new session starts with:

```yaml
default_model: GPT-5.6 Luna
models:
  - model: openai/gpt-5.6-luna
    name: GPT-5.6 Luna
    # …
```

Leave it out and the first entry is the default. Inside a session, `/model`
switches for that conversation only — the next one starts from the default
again.

## Managed mode

Sign in and the models become the deployment's:

```console
$ buildmax login
Server URL [http://localhost:5678]: https://buildmax.example.com
Email: you@example.com
Password (leave blank to use a login code): ********
Logged in as you@example.com on https://buildmax.example.com
```

Every model that deployment offers is available to you — a team is who you
collaborate with, not what gates a model. `buildmax models` lists them and says
where prompts go:

```console
$ buildmax models
Signed in to https://buildmax.example.com. Prompts, tool schemas, and tool
results go there.

Models this deployment offers:
  NAME    CONTEXT   DEFAULT
  Fast    128000
  Deep    200000    yes
```

Your `settings.yaml` models are untouched and unused while you are signed in.
`buildmax logout` brings them back.

What you gain is that the deployment holds the provider credentials, so you
never have one on your machine, and it records what each call cost. What changes
is where your data goes: **prompts, tool schemas, and tool results pass through
that server.** That is the point of the mode, which is why every surface says
which one you are in — `buildmax models`, the `/model` panel, and the TUI footer.

## The two never mix

A signed-in session sees only the deployment's models. A signed-out one sees
only `settings.yaml`. Neither covers for the other:

- **A deployment that is down does not fall back to your local models.** The
  session refuses to start and says so. Falling back would send a prompt you
  wrote for a governed deployment to a provider on your own key instead.
- **An expired login does not become local mode on its own.** BuildMax says the
  session ended and waits: sign in again, or `buildmax logout` to work locally.

Working offline is therefore `buildmax logout` — one command, and an explicit
decision about where your prompts go.

## Checking it

`buildmax doctor` reports the mode as a check of its own, along with whether the
models behind it actually work:

```console
$ buildmax doctor
✓ mode         signed in to https://buildmax.example.com: its models serve every prompt
```

Signed out, it reports local mode and then checks each entry in `settings.yaml`
— an endpoint, a key, a local daemon that is not running.

## Related

- [reference/configuration.md](../reference/configuration.md) — every
  `settings.yaml` field
- [deploy/overview.md](../deploy/overview.md) — running a deployment for a team
- [design/client-modes.md](../design/client-modes.md) — why it works this way
