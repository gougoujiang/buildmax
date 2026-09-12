# Agent Exploratory Testing

> **简体中文：** [阅读中文镜像](../zh-CN/contribute/exploratory-testing.md)
> **Audience:** contributors and testing agents · **Status:** current

Use a real user journey to discover failures and usability obstacles that fixed
test cases do not yet describe. Leave enough evidence for another contributor
to reproduce each finding and decide what to change.

The repository already has scripted verification and interactive Portal and
Desktop drivers. This guide supplies the exploration method: choose an outcome,
operate the product, interpret what happened, and use that observation to choose
the next action. It adds no runner or automatic gate. Exploration can cross
end-to-end boundaries; the distinction is that its branches evolve during use.
Required suites remain in [testing.md](testing.md).

## Choose One Journey

Use this path when asked to explore the product, when a changed flow needs
hands-on review, or when the right regression assertion is still unclear.
Routine mechanical edits do not require an exploratory session.

Start from the user outcome, evidence that it matters, and today's constraints.
Read the relevant [manual](../../manual/introduction.md),
[current state](../current-state.md), and [roadmap](../ROADMAP.md) to distinguish
available behavior from plans. A reported obstacle or the journey affected by a
change is a useful starting point. Use code to locate risks and explain results;
do not let the current implementation be the only definition of correctness.

Write a short charter before operating the product:

```text
Journey and why it matters:
User role, starting data, and observable success:
Surface and environment; source/build being tested:
Scope, allowed mutations, and model mode/cost boundary:
Time budget, including setup, reproduction, reporting, and cleanup:
Initial question or suspected risk:
```

When the request leaves routine choices open, choose one relevant journey and
a 30-minute budget, state those assumptions, and proceed within the existing
authorization. Reserve the final part of that budget for evidence and cleanup.
Ask only for missing access or a consequential scope decision that cannot be
inferred; repeated confirmation is not part of the method.

These are starting charters, not a mandatory checklist or claims of coverage:

| Journey | Observable outcome | Possible branches after the first attempt |
|---|---|---|
| Portal: submit work, leave the page, return to the result | The same Task can be found, its TaskRun state is understandable, and available results are reachable | Refresh during progress; navigate away and back; inspect a failed run; cancel and retry when offered |
| Desktop: complete a first local task | A user can discover required configuration, submit work, and find its Session and output | Start without a model; change Project; interrupt work; reopen the Session |
| CLI/TUI: interrupt work and continue later | The user understands what stopped, what was saved, and how to continue | Cancel while waiting; exit and reopen; inspect history and workspace changes |

## Prepare An Environment You Can Account For

Follow [testing.md](testing.md) for environment selection and lifecycle, and
inspect `./make help` and the relevant command's help before starting
infrastructure or installing prerequisites. Use the smallest environment that
can support the claim; Portal/worker deployment claims still require the kind
boundary described there.

- Record the commit, dirty-tree status, actual target URL or binary, and how
  the running build was obtained. Reload changed images before using them as
  evidence. An interactive driver does not perform the scripted suite's
  source-to-deployment check for you; if identity is uncertain, say so.
- Establish whether the environment is owned or attached. Use the documented
  ephemeral kind lifecycle for a task-owned cluster; the resident
  `buildmaxdev` cluster belongs to a person. Never reset shared state for a
  clean start. Keep test resources uniquely named and track their IDs.
- Use an isolated runtime home and disposable workspace for local journeys.
  Development commands use the persistent `testing-sandbox`, not a fresh home
  per run. Check existing state before calling a journey "first use", and do
  not erase that state if another task or contributor owns it.
- Record whether model responses are scripted or real. Fixed E2E scenarios
  are useful for boundary checks but may not support arbitrary exploratory
  prompts. Do not interpret a scenario mismatch as a product failure, or a
  scripted response as evidence of real-model quality. Ad hoc development
  sessions are not automatically backed by the E2E mock. Use paid inference
  only when the task authorizes it; otherwise explore supported no-cost paths
  and report the blocked portion.
- Check that the chosen account, Space role, services, and seed data support
  the starting journey. Record fixture creation and privileged login setup as
  preparation, not proof that a new user could perform those steps.

Choose the available interaction surface and preserve its limitations:

| Surface | Entry point | Evidence boundary |
|---|---|---|
| Portal | [Portal driver](../../.buildmax/skills/drive-portal/SKILL.md) against an existing deployment, or an available browser tool | The driver's `login` helper mints a code through kind; it is not a Compose login helper. Its `ss` command temporarily expands the scrolling shell, so that image cannot prove the original viewport layout. Capture the unmodified viewport with a capable browser tool for clipping or overlap findings. |
| Desktop | [Desktop driver](../../.buildmax/skills/drive-desktop/SKILL.md) against the development browser bridge, or an available native UI tool | The driver uses DOM `el.click()` and does not drive the native window. Confirm visibility and hit testing through real pointer/keyboard interaction before claiming a control is usable. Report native-window behavior as untested without native evidence. |
| CLI/TUI | Built CLI with an isolated home/workspace; see [CLI reference](../../manual/cli.md) and `./make help run` | Use a PTY-capable terminal for interactive behavior. Captured command output alone does not prove focus, key handling, or screen layout. |

Use the linked setup instructions for commands, not an invented exploration
command. Missing interaction capability is a limitation to report, not a reason
to silently substitute a backend call for the user action.

## Explore By Observation

1. **Attempt the ordinary journey through user-facing controls.** Use visible
   labels, accessible names, or terminal help to discover actions. Observe what
   a user can know at each step: what happened, whether work is continuing, and
   what they can do next. Record friction even when the operation succeeds.
2. **Observe before choosing the next action.** Inspect the visible state and
   relevant output after a meaningful transition. Read console errors or logs
   where useful; a rendered shell or a driver command returning `OK` does not
   establish the user's outcome. Keep a brief action/observation/next-question
   log so the branch has an explanation.
3. **Follow the strongest unanswered question.** An ambiguous progress indicator
   suggests leaving and returning; a confusing failure suggests recovery; an
   unexpected duplicate suggests checking repeated submission. Pick branches
   from evidence instead of executing every idea below.
4. **Preserve a surprising result before diagnosing it.** Capture its original
   state, inputs, resource IDs, and relevant evidence. Then narrow the sequence
   and retry from a known starting state within the budget. Keep the first
   failure even if a retry succeeds; report intermittent behavior explicitly.
5. **Close the journey.** Check the result and final state, including available
   history or outputs. If scope includes fixing the issue, preserve the
   pre-fix finding first, then identify the new build and verify the fix in a
   separate attempt.

Useful prompts for choosing a branch:

- **Continuity:** What happens after refresh, back navigation, leaving and
  returning, or reopening a Session? Is the same work still identifiable?
- **Timing:** What happens if an offered action is repeated, cancelled, or
  retried while the state is changing? Does the final state agree across views?
- **Input and presentation:** Can the user understand empty state, invalid
  input, long content, keyboard focus, scrolling, and narrow layouts?
- **Failure and recovery:** Does an error explain the consequence and an
  available next step? Does retry preserve useful work?
- **Ownership:** With explicitly prepared test accounts and roles, does
  switching Space or Project keep context and access understandable?

Backend calls and code inspection can prepare fixtures or diagnose a failure.
Keep those steps separate from the user journey. If a UI obstacle forces a
bypass, record that obstacle and mark the later path as assisted. For injected
network or service failures, record the injection and limit the conclusion to
what it exercised; a browser response override does not prove backend recovery.
Disruptive fault injection belongs only in an owned environment and within the
task's scope.

## Judge Findings Against Evidence

State the expectation and its basis: a current product promise, an explicit task
requirement, or a concrete consistency rule such as "the same TaskRun retains
the same terminal result after refresh". When documentation and behavior differ,
verify which is wrong; an old design record is not an acceptance contract.

Distinguish the following in the report:

| Finding | What to record |
|---|---|
| Confirmed defect | The violated expectation, actual behavior, affected user outcome, and reproduction evidence |
| Usability obstacle | The confusing label, hidden action, missing feedback, or extra work observed; explain its impact without presenting taste as a broken contract |
| Suspected defect or product question | Observation, possible explanation, and the missing evidence or decision |
| Environment or tooling blocker | The prerequisite or interaction capability that failed, and which part of the journey remains untested |

Prioritize by impact: lost work or incorrect access, inability to finish or
recover, avoidable confusion, then cosmetic friction. Keep impact separate from
confidence and repeatability. A plausible cause is a hypothesis until checked.
An Agent's self-reported PASS, an empty console, or one successful retry is not
independent evidence that the product works.

## Report, Clean Up, And Preserve The Discovery

Stop when the charter's main questions have evidence, the time budget is reached,
or an access, cost, safety, or tooling boundary prevents useful progress. A
blocked branch need not stop independent branches within scope. Record what was
actually exercised, what was skipped, and what remains uncertain; do not turn
"no finding in this session" into a product-wide pass or coverage percentage.

Keep a run-specific directory under the gitignored `.artifacts/` for the report,
screenshots, terminal output, and relevant logs or traces. Do not reuse the E2E
output directory, which suites clear. Preserve evidence before cleanup and
redact credentials, login codes, and unrelated private data before sharing.

Use this compact report shape; omit inapplicable fields rather than invent data:

```text
Charter: journey, rationale, role, success condition, scope, time spent
Environment: date, commit/dirty state, build identity, surface, target,
             account role, starting data, model mode, owned/attached resources
Explored: action -> observation -> next question; outcome of each branch
Findings (one entry each):
  Title, classification, user impact, confidence/repeatability
  Preconditions and shortest known reproduction
  Expected behavior and its basis; actual behavior
  Evidence paths and relevant Task/TaskRun/Session IDs
  Workaround if any; remaining uncertainty
Not exercised or blocked: scope and reason
Cleanup: removed resources, retained evidence, leftovers and their owner
Follow-up: proposed fix or decision, appropriate regression scope
```

Stop only the processes you started and remove only resources this session owns.
Follow the documented teardown for owned infrastructure even after setup fails;
inspect partial resources rather than assuming failure left nothing behind.
For attached environments, record any resources that cannot be removed through
supported operations. Never drop shared data or delete someone else's cluster.

Include the findings and evidence locations in the handoff. When a fix follows,
add a focused regression at the lowest layer that proves the failure; use a
browser regression when the defect depends on interaction or presentation, and
the required deployment scope when it crosses that boundary. Run the scopes in
[testing.md](testing.md) and update documentation to match the corrected behavior.
Preserve the discovery in the fix or an authorized work item, with evidence
accessible to its reviewer; a local artifact path alone is not durable shared
evidence. Follow [AGENTS.md](../../AGENTS.md) for backlog/issue ownership and
avoid duplicate work items. Exploration findings do not automatically authorize
publishing an issue, broadening the implementation, or claiming Beta readiness.
