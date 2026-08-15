# Compose Quickstart

> **Audience:** operators · **Status:** current
>
> A team deployment on one machine, in about five minutes. For what the pieces
> are and how to run them anywhere else, read [overview.md](overview.md) first
> or after — this page is the short path.

## What You Get

Three containers: MySQL, the server (which spawns workers itself), and the
Portal. No MinIO — the server and its workers share one container and one
volume, so the local filesystem backend is enough here. A deployment where
workers run elsewhere needs blob storage both can reach.

## Run It

```bash
cd deployment/compose
./generate-env.sh          # writes .env with generated secrets
docker compose up -d
```

The first `up` builds the images from this checkout, which takes a few minutes.
Once a release publishes them, `docker compose pull` fetches them instead.

Wait for the server to report healthy:

```bash
docker compose ps
```

## Create Your Account

Nobody can register themselves — [signup is closed by
default](authentication.md). Create the first account from inside the server
container:

```bash
docker compose exec server buildmax-server user create you@example.com
docker compose exec server buildmax-server user login-code you@example.com
```

The second command prints a single-use code. Open <http://localhost:8080>,
enter that email and code, and you are in.

## Add a Model

The Portal runs without one, but conversations cannot reach a model until you
set a key. Put it in `.env`:

```bash
BUILDMAX_CONVERSATION_MODEL_API_KEY=sk-your-key-here
```

then `docker compose up -d` to apply it. The endpoint and model id are in
`server.yaml` next to the compose file — any OpenAI-compatible provider works.

## What To Change Before Trusting It

This stack is shaped for a laptop. Before it faces anyone else:

- **Ports are published to the host.** `5678` and `8080` are reachable from
  wherever the machine is reachable.
- **No TLS.** Both services speak plain HTTP. Put a reverse proxy in front, and
  then set `BUILDMAX_API_BASE=/` on the Portal so it calls its own origin —
  which also removes the CORS pairing below.
- **The agent runs shell commands.** The worker executes what the model asks
  for, inside the server container, with the sandbox
  [off by default](../guide/sandbox.md).
- **Storage is a Docker volume.** `docker compose down -v` deletes every
  workspace, artifact, and account with it.

## Two Settings That Must Agree

`BUILDMAX_API_BASE` on the Portal is what the **browser** calls, so it is a host
address, not a container name. `cors_origin` in `server.yaml` must name the
origin the Portal is served from. Change a port in `.env` and the second one has
to follow, or the browser blocks every request and the Portal looks broken while
both containers report healthy.

## Common Problems

| Symptom | Cause |
|---|---|
| `run ./generate-env.sh first` | `.env` is missing; compose refuses rather than starting with empty secrets |
| Portal loads, every request fails in the browser console | `cors_origin` and the Portal's origin disagree |
| `signup is disabled on this server` | Expected. Create the account with `user create` |
| `invalid otp` | The code is single-use and expires in an hour; issue another |
| Server restarts in a loop | Usually MySQL: `docker compose logs mysql` |

## Teardown

```bash
docker compose down       # keep the data
docker compose down -v    # delete it
```

## Related

- [overview.md](overview.md) — the topology, and configuration field by field
- [authentication.md](authentication.md) — accounts, login codes, what is missing
- [local-kind.md](local-kind.md) — the Kubernetes path instead of this one
