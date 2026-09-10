# Architecture Review

> **Audience:** contributors and code-changing agents · **Status:** current

A repeatable health check for the domain model, type system, contracts, and
package structure, meant to be run after a burst of growth or before a large
refactor. The coarse boundaries — the dependency direction and the layer rules
— are already guarded by tests under
[`internal/architecture`](../../internal/architecture); this review looks for
the decay that those tests do not catch, which is almost always *inside* a
valid layer: duplicated models, weakly expressed invariants, ambiguous
interfaces, functions with several reasons to change, ownership that has
drifted from what [`AGENTS.md`](../../AGENTS.md) claims, and concepts that could
be deleted rather than wrapped.

It produces a small set of review artifacts — a concept and type map, an
interface-contract table, a rule-ownership table, a package dependency graph,
and a ranked findings list — plus, for each finding, a proposed change that
names the concrete requirement it serves. It is not a rewrite. Read
[repo-layout.md](repo-layout.md) and the relevant
[architecture/](architecture/README.md) document before acting on anything it
surfaces.

## When To Run It

- After merging a run of feature work that added many types at once.
- Before a refactor large enough that you need a map to scope it.
- When a package has become the one everything imports, or a file has grown past
  the point where you can hold it in your head.
- On a cadence, if the codebase is growing fast — a quarterly pass keeps the map
  honest and the findings small.

Do not run it to re-check what the architecture tests already prove. If you find
yourself verifying the dependency direction or hunting for exported mutable
package state, stop: `TestInternalLayerImports` and
`TestNoExportedMutablePackageState` own those. Spend the review on what no test
asserts. Go also rejects package import cycles at compile time; dependency
analysis should therefore look for hubs, long lateral chains, and misplaced
dependencies rather than rediscovering cycles the compiler cannot admit.

## Establish The Baseline First

Before looking for problems, write down what is already guaranteed, so the review
does not rediscover it.

- **Dependency direction and layer rules.** `bootstrap → interface / server /
  service / agentapp / infra → core`, with `core` pure. Enforced by
  `TestInternalLayerImports`; described in [repo-layout.md](repo-layout.md).
- **Ownership model.** [`AGENTS.md`](../../AGENTS.md) names the owner of each
  business capability — Task versus TaskRun, Space as the authorization boundary,
  Conversation as a non-parent creator of Tasks, `localproject` as the owner of a
  local Project. This is the specification the review measures the code against.
- **Authoritative files.** Tool names in `internal/tool/names.go`; routes in each
  handler's `Register`, mirrored by `internal/server/static/openapi.json`; schema
  in the `xxxRow` structs applied by `AutoMigrate`; bootstrap environment in
  `internal/config/env_spec.go`. A duplicate of any of these is a finding by
  definition.
- **Identity and persistence conventions.** `snake_case` JSON tags, singular
  table names, `NewPublicID` for server entities. Partly guarded by
  `entity_identity_test.go`.

The baseline is the ruler. Everything below measures against it.

## The Dimensions

Run them roughly in this order: the first two are the map the rest reads from.
Inventory commands identify candidates; none of their counts or matches is a
finding by itself. The semantic passes need a person or agent to trace behavior
and judge it against the baseline.

### 1. Type Inventory And Domain Map

Group every declared type by package and by role: domain entity, transport or
DTO, configuration, wire/serialization, or persistence row. The goal is a table
of *concepts*, not of structs. Record every concept with more than one
representation, but do not classify that fact as duplication yet. Domain,
transport, and persistence types may correctly differ because they have
different invariants, trust boundaries, serialization contracts, or rates of
change.

```bash
# Counts by package, structs and interfaces.
rg -g '*.go' -g '!**/*_test.go' '^\s*type\s+\w+\s+struct' internal | wc -l
rg -g '*.go' -g '!**/*_test.go' '^\s*type\s+\w+\s+interface' internal | wc -l
# Every type declaration with its file, to bucket by package and role.
rg -n -g '*.go' -g '!**/*_test.go' \
  '^\s*type\s+\w+\s+(struct|interface)' internal
```

For each authoritative domain type, inspect whether constructors and methods
make its invariants visible. Look for strings standing in for closed value sets,
several booleans encoding one state, zero values with ambiguous meaning, and
callers that must remember validation the type itself does not express. A type
should make valid states easy to construct and invalid states difficult to
represent without hiding necessary wire or storage validation.

Healthy: one package owns the concept and every other representation has a
named boundary reason; conversion and validation happen in one place. Smell:
two representations enforce the same invariant independently, mappings are
scattered among callers, or a `struct` was copied merely to avoid an import.

Produce: the domain map (concept → representations → owning package) and a
shortlist of redundant representations or weakly expressed invariants.

### 2. Interface Ownership And Abstraction Fit

For each interface, decide whether it is defined at the *consumer* or the
*producer*, and whether it earns its keep. Go's grain is usually a narrow
interface declared where it is used. One implementation is not evidence that
an interface is redundant: dependency inversion, an external-system boundary,
or isolation of a capability may justify it. Conversely, several implementations
do not rescue an interface that groups unrelated operations.

```bash
rg -n -g '*.go' -g '!**/*_test.go' '^\s*type\s+\w+\s+interface' internal
# For a suspect interface, find its implementations and callers.
rg -n -g '*.go' 'InterfaceName' internal
```

Read more than the method set. Record input and output ownership, validation,
error categories, `context.Context` cancellation, idempotency, concurrency
safety, and whether returned values may be retained or mutated. If callers or
implementations disagree about one of these, the interface is not an accurate
contract even when its Go signature compiles.

Healthy: the interface represents one consumer capability and its behavioral
contract is asserted at the boundary. Smell: a producer-owned interface mirrors
all methods of one concrete implementation, an interface widens whenever that
implementation grows, or each caller guesses differently about errors and
retries.

Produce: the interface-contract table (interface → owner → consumer capability
→ implementations → behavioral contract → verdict: boundary / seam /
redundant).

### 3. Function Scope And State Transitions

Trace the functions that create or change important domain state. For each
rule, identify the single function that authoritatively enforces it and the
transaction or critical section in which it runs. Handlers, commands, workers,
and adapters should translate inputs and delegate; they should not each own a
variation of the rule.

Use function length or parameter count only to find candidates. The decisive
questions are how many independently changing policies the function knows,
whether its name describes all of its effects, and whether a caller can predict
partial-failure behavior. Pay particular attention to functions that validate,
authorize, persist, publish events, and retry in one body without an explicit
application-level operation owning that sequence.

Healthy: one function owns one cohesive operation, dependencies and side
effects are explicit, and tests assert the state transition rather than its
private steps. Smell: validation repeated before and inside the operation,
boolean parameters selecting unrelated modes, or an error leaving state whose
meaning only the implementation knows.

Produce: the rule-ownership table (rule or transition → authoritative function
→ callers → transaction and side effects → failure outcome) and a list of
functions with more than one rightful owner.

### 4. Assembler And God-Object Complexity

Find the files that assemble the world and check that they only assemble it. In
this codebase `internal/agentapp` is the runtime assembler — models, tools, MCP,
hooks, sandbox, traces, skills, sessions, workspace — and AGENTS.md says that is
all it does. An assembler that has also grown *behavior* is the classic place for
responsibilities to pile up.

```bash
# Largest production files — assemblers and god objects surface here.
find internal -name '*.go' -not -name '*_test.go' -exec wc -l {} + | sort -rn | head -15
```

File size is only a lead. Healthy: a large file is cohesive, or is large because
wiring is genuinely broad and each thing it wires is owned elsewhere. Smell:
construction logic interleaved with request handling, state transitions, or
validation that belongs to the capability being constructed.

Produce: for each oversized file, a one-line verdict — wiring-only (leave it), or
a named responsibility that should move, with its rightful owner.

### 5. Ownership Drift Against AGENTS.md

Take the ownership claims in AGENTS.md as a checklist and verify the code honors
them. This is the highest-value dimension because the claims are precise and the
violations are actionable: each state transition, validation rule, and
authorization decision is supposed to have exactly one authoritative
implementation, with handlers and workers delegating to it.

Check, for example, that Conversation creates a Task without reaching into
TaskRun's storage; that Space is the point where Portal authorization is decided,
not re-decided per handler; that a missing database row becomes
`apierr.ErrNotFound` above `internal/infra/db` and GORM stays inside it.

Healthy: one owner per rule, everyone else delegating. Smell: the same validation
in a handler and in the service; an authorization check duplicated because the
boundary was inconvenient to reach.

Produce: a list of drifts (claimed owner → where the logic actually lives → the
duplicate or leak), each tied to the AGENTS.md line it violates.

### 6. Dependency Shape And Change Coupling

The layer *direction* is tested and Go rejects import cycles. Build the package
dependency graph to find high fan-in or fan-out packages, long lateral chains,
and packages that pull unrelated capabilities into the same change surface.

```bash
# One line per package and its direct imports; retain module-local imports.
go list -f '{{.ImportPath}} {{join .Imports " "}}' ./internal/... | \
  rg 'github.com/gougoujiang/buildmax/internal/'
# Or, quick fan-in: who imports a suspected hub.
rg -l -g '*.go' 'buildmax/internal/<pkg>' internal | wc -l
```

Healthy: high fan-in packages are small, stable, and semantically cohesive;
orchestrators depend on capabilities without becoming their owner. Smell: a
package that everything imports because unrelated types were parked in it, or a
small business change that requires edits across several peer packages and
their adapters.

Produce: the intra-layer dependency graph, fan-in/fan-out ranking, and the
change paths that cross more ownership boundaries than the operation requires.

### 7. Persistence And External Contract Consistency

The `xxxRow` structs are the schema's source of truth. Verify the domain ↔ row
conversion is centralized, that no database concept leaks up into `core`, and
that the identity and naming conventions hold everywhere the tests do not yet
reach. Apply the same test to HTTP, OpenAPI, JSON, CLI, configuration, and tool
output: each is an external contract, not an incidental rendering of an
internal struct.

```bash
rg -n -g '*.go' '^\s*type\s+\w+Row\s+struct' internal/infra/db
# Any gorm import outside the db package is a leak.
rg -n -g '*.go' -g '!**/*_test.go' 'gorm\.io|gorm:"' internal | \
  rg -v '^internal/infra/db/'
```

Healthy: rows convert to domain types at the `db` boundary and nowhere else;
`core` never names a row; transport types expose only intended data and map
errors consistently. Smell: a `gorm` tag or import above `infra/db`, a domain
method that assumes a column, an HTTP response reusing a row, or different
surfaces assigning different meanings to the same state.

Produce: any schema-convention violations and any place a persistence concept has
escaped its layer. Remember that a real `db` change needs the MySQL scope
(`./make test mysql`) to prove; see [testing.md](testing.md).

### 8. Concurrency, Failure, And Resource Lifetime

Review the lifecycle of operations that outlive a request or cross a process:
Agent runs, TaskRuns, streams, leases, checkpoints, hooks, MCP child processes,
background jobs, and shutdown. Identify who creates each resource, who may use
it concurrently, who closes it, and which durable state remains after every
failure point.

Healthy: cancellation and deadlines propagate, ownership of cleanup is unique,
retries are idempotent or fenced, and partial success has an explicit durable
meaning. Smell: fire-and-forget goroutines, close responsibilities shared by
several layers, retry after an ambiguous write, or in-memory state treated as
authoritative across replicas.

Produce: a lifecycle table (resource or operation → creator → concurrent users
→ cancellation → cleanup → durable failure state) and any unowned interval.

### 9. Duplication And Deletable Surface

A burst of new code usually leaves parallel implementations behind. Look for
near-synonym type and function names, copy-pasted conversion logic, and — because
the project is Alpha with no compatibility contract — shims and dead paths that
can simply be deleted rather than maintained.

```bash
# Near-duplicate type names across the tree.
rg -o --no-filename -g '*.go' -g '!**/*_test.go' \
  '^\s*type\s+\w+' internal | awk '{print $2}' | sort | uniq -d
```

Repeated syntax is not necessarily duplicated knowledge, and superficially
different code can encode the same rule. Prefer a little local repetition when
the callers have different invariants or change for different reasons. Extract
shared code only when it gives one stable business rule or protocol one owner.

Apply Occam's razor: for every field, abstraction, or entity, name the concrete
requirement that fails without it. If none does, the finding is "delete," not
"refactor." Removing a wrong concept everywhere is usually simpler than wrapping
it — and Alpha means you may do so without a migration path.

Produce: the duplication list and a deletable-surface list, each item paired with
the requirement it does or does not serve.

## Turning Findings Into Change

Rank findings by the cost of leaving them, not by how easy they are to fix. For
each one that survives, write the proposed change as a first-principles statement:
the user outcome it serves, the concept it adds or removes, and why the result
has fewer independent concepts than before. Fix wrong shapes coherently across
code, tests, and documentation in one change — the project's Alpha status means
you correct the design rather than add a compatibility layer around it.

A finding that cannot be stated as a concrete requirement is not a finding; drop
it. A change that adds a concept must justify the concept against a demonstrated
need, or it is the same decay this review exists to catch.

## What A Completed Review Leaves Behind

- The concept and type map and the interface-contract table (dimensions 1–2).
- The rule-ownership and resource-lifecycle tables (dimensions 3 and 8).
- The intra-layer dependency graph and fan-in/fan-out ranking (dimension 6).
- A ranked findings list, each with a proposed change and the requirement it
  serves.
- Where a finding points at an authoritative file or an AGENTS.md ownership
  claim, a note of which one, so the fix updates the source of truth rather than
  working around it.

Keep the artifacts with the change that acts on them, not as a standing document
— the map is only true for the commit that produced it, and the next review
regenerates it from the code.
