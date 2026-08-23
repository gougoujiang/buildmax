# Client Modes: Local And Managed

> **Audience:** contributors · **Status:** decided, not implemented
>
> This record revises decisions already shipped in
> [llm-gateway.md](llm-gateway.md). Where the two disagree, this one is the
> direction and the gateway record describes what the code does today. Sections
> 7, 12, and parts of 1, 4.2, and 10 of that record are superseded; see
> [§10](#10-what-this-supersedes).

CLI and Desktop run in one of two modes. Which one is in effect is decided by a
single fact — whether a login exists — and everything else follows from it.

## 1. Decision

| | Local mode | Managed mode |
|---|---|---|
| Trigger | no `auth.json` | `auth.json` present |
| Model list | `settings.yaml` `models[]` | fetched from the server |
| Provider credential | on this machine | never leaves the server |
| LLM call | this machine calls the provider | this machine calls the server |

The two are **mutually exclusive**. A managed session does not see local model
entries, and a local session does not see server models. Leaving managed mode
is `buildmax logout`; entering it is `buildmax login`.

Three properties this buys, in order of importance:

- **One fact decides the mode.** No mode flag, no remembered preference, no
  precedence rule between them. A user asking "which mode am I in" has one
  place to look, and so does the code.
- **No merged list.** A union of local and server models would need conflict
  rules for duplicate names, a precedence order, and a story for what `--model`
  means when both match. None of that has to be designed if the list has one
  source.
- **No implicit fallback survives.** A server outage cannot redirect a prompt
  to a personal provider key, because in managed mode there is no local entry
  to fall back to. This was already the gateway's rule; a single-source list
  makes it structural rather than enforced.

The cost is that working offline requires `buildmax logout`. That is accepted:
it is one explicit command, and it is honest about what changed — prompts now
go somewhere else.

## 2. What This Fixes

Managed mode today is configured by hand-editing `settings.yaml`:

```yaml
- name: Team default
  model: default
  transport: buildmax
  server_url: https://buildmax.example.com
  team_id: tm_example
```

Every field except `model` is something the login already established.
`buildmax models --team <team_id>` prints this block for the user to copy
(`internal/interface/cli/models.go:253`), which is the design gap stated out
loud.

The root cause is structural: **`settings.yaml` is load-only.** The only writer
in the repository is `buildmax init`, which renders the whole file from a
template (`internal/interface/cli/init.go:188`); `internal/config` has no save
function. So a stateful action (login) and a configuration edit (using a team
model) were never connected, and could not be.

It also produces dangling state. After `buildmax logout`, the managed entries
stay in `settings.yaml` and `buildmax doctor` reports them as missing a login.
Configuration held something that depended on session state, and the two went
out of step exactly as that always does.

## 3. Mode Resolution

`auth.json` is the only input. It is present or it is not.

Desktop's third mode flag is removed: `internal/interface/desktop/mode.go`,
`desktopState`, and the `UseLocalMode` / `ConnectToServer` bindings all go. The
comment at `internal/interface/desktop/app.go:466` — "a usable login wins over a
remembered local choice" — is a patch over having two sources of truth, and it
disappears with the second source. Desktop opens straight into the agent, as the
CLI always has, and signing in becomes an action in settings rather than a gate
on startup.

Startup therefore has no mode question to ask. A stored login means managed;
nothing means local.

## 4. Model List

The server returns **a `ModelEntry` list with the credential fields removed**.
Same shape as `settings.yaml` `models[]`: `name`, `model`, `context_window`,
`vision`, `reasoning`, `max_tokens`, `pricing`, plus `is_default`. No `api_url`,
no `api_key`, no `transport`, no `server_url`, no `team_id`.

This is the point of the design rather than a convenience. One shape means the
model panel, the default-model rule, the status line, and `buildmax models`
have one code path, and the modes differ only in where the list came from.

`transport` on a model entry goes away with it. Transport was a per-entry
property because one list held both kinds; the mode now says which it is.

## 5. Models Are Global To The Deployment

A team is a collaboration boundary — shared issues, agents, workflows. It is
**not** a model authorization boundary. Every model in the catalog is available
to every user of the deployment.

This is a deliberate narrowing of [llm-gateway.md §4.2](llm-gateway.md), which
planned per-team model policy in the database. That capability is withdrawn,
not deferred. Gating models per team would mean a user who belongs to several
teams has to declare which team a prompt is for — a workspace-selection concept
this product does not have and is not adding.

The current code is already deployment-wide: `llm.aliases` in `server.yaml`
exposes the same set to every team, and the per-team database policy was never
built. So this decision deletes an unbuilt abstraction rather than changing
behavior.

## 6. The Alias Layer Is Removed

`llm.aliases` and `llm.default_alias` are deleted from `server.yaml`. Clients
name a model by `llm_model.name`. The `llm:` block itself stays, holding the one
key that survives: `default_model` ([§7](#7-the-default-model)).

The alias layer existed to give teams stable names that survive provider
routing changes. `llm_model.Name` already does that job: it is
`uniqueIndex; not null` (`internal/infra/db/llm_model.go:22`), operator-facing,
and unique across the deployment. Repointing a name at a different upstream is
editing that row's `api_url` and `model` — the name never moves. An
alias-to-ID map on top of a unique name is indirection with no second job, and
`resolve.go:127` already concedes that aliases are the team-facing half of a
resolver that can address catalog entries directly.

With teams out of the resolution path, that half has nothing left to be.
Resolution goes from:

```text
(team_id, alias) -> catalog ID -> LLMClient
```

to:

```text
name -> llm_model row -> LLMClient
```

## 7. The Default Model

Both modes need to answer "which model does a new session start with", and both
fall back to the first entry when nothing says otherwise.

- **Local:** a new top-level `default_model` key in `settings.yaml`, naming an
  entry in `models[]`.
- **Managed:** `llm.default_model` in `server.yaml`, naming a row in
  `llm_model`. The server resolves it and marks that entry in the model list it
  returns, so the client still just reads a flag.

The default is configuration, not catalog state. [§7 of
llm-gateway.md](llm-gateway.md) argues the catalog is not configuration, but its
reason is that the catalog holds provider credentials, which must not travel in
a file a ConfigMap carries. A model name is not a credential, so that reason
does not reach this field.

Two things do reach it. First, `server.yaml` already holds the deployment's
other model choices — `conversation.model` for Tier 1 and `worker.llm.model`
for task runs. Putting the client default anywhere else splits one question,
"what does this deployment default to", across two places. Second,
`worker.llm.model` falls back to this value when empty; if the default lived in
the database, `server.yaml` could no longer explain its own behavior without a
query.

Restarting to change it is not a new cost: `conversation.model` already
requires one, and promoting a new default model is a deliberate,
everyone-sees-it change rather than a routine operation.

The resolution chain is therefore: an explicit name, else `llm.default_model`,
else the first catalog row. A `default_model` naming a row that does not exist
**stops the server at startup**, for the same reason the empty-catalog check
does — it parses cleanly and would otherwise fail every session at its first
call.

`/model` stays session-scoped and is not persisted, which is what the code
already does (`internal/interface/cli/chat_models.go:112`). A model choice is a
property of the conversation you are having, not a setting; the next session
starts from the default again. Nothing needs to be written to disk to switch
models, which is what makes switching cheap.

## 8. An Expired Login Does Not Silently Fall Back

A login can be present but dead: the refresh token expired, or the session was
revoked. Managed mode must not quietly become local mode when that happens —
that is precisely the redirection of governed traffic that the no-fallback rule
exists to prevent.

The surface says the login expired and offers one action: return to local mode.
Taking it **deletes `auth.json`**, which is what actually returns the client to
local mode, since the file's presence is the mode. Until the user takes it, the
session does not run.

Deleting the file rather than marking it stale is what keeps §3 true. A retained
but invalid `auth.json` would be a third state, and every reader would need to
know the difference between "has a login" and "has a working login".

## 9. Usage Is Attributed To A Person

The `llm_call` ledger records the user. `team_id` is **dropped from the row**,
not made nullable: the ledger already carries `task_run_id`, and run → task →
team reconstructs the team whenever the question is asked. A team column on
every call would be a denormalization that only a foreground call — which
belongs to no team — could fail to fill.

The composite unique index `idx_llm_call_client` leads with `team_id`
(`internal/infra/db/llm_call.go:21`) and must be rebuilt to lead with the user
key.

Quota per team is out of scope here. It returns only if team workspace
selection is ever added, because a per-team ceiling needs each call to belong to
exactly one team, and today a foreground call does not.

## 10. What This Supersedes

In [llm-gateway.md](llm-gateway.md):

| Section | Was | Becomes |
|---|---|---|
| §1 Decision | transport is a per-model-entry property | mode is per client, decided by `auth.json` |
| §4.2 Team Model Policy | per-team model authorization in the database | withdrawn; models are deployment-global |
| §7 Model Catalog And Resolution | `(team_id, alias)` resolution, alias map in `server.yaml` | `name` resolution against `llm_model` |
| §10 Call Ledger | team-scoped ledger rows | user-scoped; team reached through the run |
| §12 Client Configuration | hand-written `transport: buildmax` entries | server-supplied list, no client-side model config |

Once this lands, fold the surviving parts of those sections into
`llm-gateway.md` and delete this record, per
[documentation.md](../contribute/documentation.md).

## 11. Delivery Order

Server first, because the client cannot fetch a list that does not exist yet.
Each step leaves the tree building and testable.

### S1. Remove the alias layer

- In `ServerLLMConfig` (`internal/config/server_config.go`), replace `aliases`
  and `default_alias` with a single `default_model` naming a catalog row.
- Rewrite `internal/service/llmgateway/resolve.go` to resolve by name. Delete
  `ErrNoDefaultAlias` and `ErrUnknownAlias`; add a not-found and a disabled
  error. `AvailableModel.Alias` becomes `Name`.
- Replace `worker.llm.alias` with `worker.llm.model`; empty means the default.
- Startup checks: the one that rejects `transport: buildmax` with an empty alias
  map becomes an empty-catalog check, and a `default_model` that names no row is
  rejected the same way.
- No schema change in this step — `llm_model` is unchanged.

### S2. Global model list endpoint

- `GET /api/llm/models` and `POST /api/llm/completions`, authenticated by the
  login alone. The team-scoped routes at `routes.go:37-38` are deleted outright,
  with no transitional aliases: BuildMax is alpha and owes no API compatibility.
- The response is the credential-free `ModelEntry` list from §4.

### S3. Ledger

- Drop `team_id` from `llmCallRow`; rebuild `idx_llm_call_client` on the user
  key.
- Update `GetLLMCallByClientID` and `GetLLMCallsByTaskRun`, and any usage
  aggregation that grouped by team.

### C1. Mode resolution

- One resolver, `auth.json` present or absent, used by CLI, TUI, and Desktop.
- Delete `internal/interface/desktop/mode.go`, `desktopState`, `UseLocalMode`,
  and `ConnectToServer`. Desktop opens into the agent.

### C2. Model source

- `agentapp` takes its model list from the resolver: `settings.yaml` in local
  mode, the server in managed mode.
- Delete `transport`, `server_url`, and `team_id` from `ModelEntry`. Delete
  `IsManaged`, and the `--team` flag on `buildmax models`.
- Add `default_model` to `settings.yaml`; first entry when unset.

### C3. Expired login

- The confirm-and-delete flow from §8, on both surfaces.

### C4. Mode is visible

- A persistent indicator in the TUI status line and the Desktop chrome: local
  mode names nothing, managed mode names the server. `managedModelLabel`
  (`internal/interface/cli/chat_models.go:56`) is the starting point, promoted
  out of the model panel.

### D1. Documentation

- `reference/configuration.md`: delete the managed-models section's client
  configuration, the `models[].transport` rows, `llm.aliases`, and
  `worker.llm.alias`; add `settings.yaml` `default_model`, `llm.default_model`,
  and `worker.llm.model`.
- `deploy/overview.md`: the operator flow is `model add`, then name one of them
  in `llm.default_model`. No alias step.
- `config-examples/server.example.yaml` and the two `deployment/smoke/`
  managed configs carry the alias block and must be updated with it.
- `guide/`: how a user switches modes, which is now two commands.
- Fold this record into `llm-gateway.md` and delete it.
- Changelog entries under `changed` for the settings shape, the removed alias
  configuration, and Desktop no longer opening on a login form.

## 12. Not In Scope

- **Team workspace selection.** Without it, per-team quota and per-team model
  policy cannot be built, and both stay out.
- **Migration.** BuildMax is alpha; `AutoMigrate` applies the new row shapes and
  no compatibility path is owed to an existing deployment.
- **A merged model list.** Rejected in §1, not deferred.
