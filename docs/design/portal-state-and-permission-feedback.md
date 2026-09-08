# Portal State and Permission Feedback

> **简体中文：** [阅读中文镜像](../zh-CN/design/Portal状态与权限反馈.md)
> **Audience:** Portal and API contributors · **Status:** planned

This record defines how Portal distinguishes loading, absence, failure, and
authorization. It can be implemented page by page as an R3 operator-journey
improvement without changing domain behavior or roadmap priority.

## Contents

- [Outcome](#outcome)
- [Evidence and constraints](#evidence-and-constraints)
- [State model](#state-model)
- [Permission model](#permission-model)
- [Mutation feedback](#mutation-feedback)
- [Implementation slices](#implementation-slices)
- [Acceptance criteria](#acceptance-criteria)
- [Alternatives rejected](#alternatives-rejected)
- [Related records](#related-records)

## Outcome

Portal never tells a user that data is empty when it failed to load, never
pretends a permission lookup succeeded when it did not, and never leaves stale
data looking current after navigation or refresh fails. Every recoverable
failure offers a relevant next action.

## Evidence and constraints

- Several list pages render the same neutral empty treatment for a failed
  request and a genuinely empty collection; some do not offer Retry.
- Space membership failure can collapse into an empty member list or a missing
  role. Controls then appear read-only as if the server denied an action.
- The shell can display a synthetic “My Space” label while Space loading failed,
  obscuring the ownership context.
- Some detail pages retain the previous object while a new request fails, so a
  stale object can appear under a new route.
- Other pages already distinguish not found from transient error and provide a
  retry action. The design standardizes that stronger pattern rather than
  introducing a second notification system.
- Server authorization remains authoritative. Portal feedback mirrors known
  policy for comprehension and does not replace server enforcement.

## State model

Every remote resource boundary has one explicit state at a time:

| State | Meaning | Required presentation |
|---|---|---|
| Loading | No authoritative response yet | Skeleton or progress with a stable page frame |
| Ready with data | Current data is available | Normal content |
| Ready empty | Request succeeded with no objects | Specific explanation and valid creation or navigation action |
| Refreshing | Current data is visible while revalidation runs | Non-blocking progress that preserves context |
| Stale | Current data remains visible after refresh failed | Persistent warning, timestamp where useful, and Retry |
| Error | No usable data could be loaded | Alert, concise cause, Retry or safe navigation |
| Forbidden | Identity is known but action or resource is denied | Scope-specific explanation and safe navigation |
| Not found | The addressed object does not exist in the resolved scope | Object-specific message and collection link |

Pages must not render Ready empty and Error simultaneously. Data from a prior
route is cleared before the new route is presented unless the state is
explicitly Refreshing or Stale for the same resource key.

The application shell treats authenticated account and current Space resolution
as blocking state boundaries. It keeps the stable frame visible but does not
render Space pages, invented Space names, or permission-sensitive controls until
resolution succeeds. Account-with-no-Spaces is a successful empty state with a
create/join path, not a bootstrap error.

State transitions live in shared request hooks or pure reducers where possible.
Shared visual components receive a resolved state and actions; they do not own
API policy or silently convert exceptions to empty arrays.

## Permission model

Permission state is separate from resource state:

```text
unknown -> allowed | denied | failed
```

`unknown` and `failed` are not equivalent to `denied`. While membership or
capability data is unknown, Portal avoids offering an action it cannot yet
authorize but explains that access is being checked. On lookup failure it shows
an error and Retry instead of a read-only page that implies a real role.

Known restrictions remain visible when that helps the user understand the
product. A disabled control or restricted tab includes the required role or
next action. Highly sensitive capabilities may remain undiscoverable when the
security design requires it, but that exception must be defined by the owning
authorization record rather than page-local guesswork.

Forbidden responses are not rewritten as not found in Portal. If the API
deliberately uses not-found semantics to avoid disclosure, Portal follows that
API contract and does not infer existence.

## Mutation feedback

Every mutation has a local lifecycle: idle, submitting, succeeded, or failed.
The initiating control shows progress, prevents duplicate submission where the
operation is not idempotent, and remains associated with its result.

Success feedback states the object and effect, not only “Success.” Creation and
scheduling success links to the resulting object. Failure preserves recoverable
input, explains whether anything changed, and offers Retry when safe. Background
completion is shown on the affected object; unrelated global toasts are not the
only durable evidence.

Optimistic updates are used only when rollback is complete and unambiguous.
Authorization, quota, execution scheduling, and destructive operations wait for
an authoritative response.

## Implementation slices

1. **Shared vocabulary.** Define the state union, alert/empty/retry presenters,
   and pure transition tests in `portal/src`; reuse `@buildmax/gui` only for
   presentation that Desktop also needs.
2. **Space bootstrap.** Remove synthetic Space fallback, model Space and role
   lookup errors, and gate Space routes on successful resolution.
3. **Collections.** Migrate Issues, Workflows, Agents, Conversations, Artifacts,
   and Files one collection at a time.
4. **Details.** Key detail state by route identity, clear unrelated stale data,
   and distinguish not found, forbidden, and transient error.
5. **Mutations.** Standardize Save, Run, Retry, install/activate, and membership
   feedback in bounded feature changes.

Each slice is independently releasable. A page is considered migrated only when
all states it can receive have tests; adding the shared component alone does not
claim coverage.

## Acceptance criteria

- Every Portal request-driven page has testable loading, data, empty, error,
  forbidden, and not-found behavior where the API can produce those states.
- No catch handler converts a failed collection request into a successful empty
  collection without retaining the error.
- Navigating between two detail IDs cannot show the first object under the
  second URL after failure.
- A failed Space or role lookup is visibly different from no Spaces or a
  read-only role.
- Every recoverable error offers Retry; every non-recoverable error offers a
  safe navigation action.
- Mutation tests cover duplicate submission, preserved input, success target,
  and server rejection.
- Browser tests exercise one collection failure, one detail 404, one 403, and
  one failed mutation in addition to success paths.

## Alternatives rejected

- **Treat errors as empty data.** It produces a calm screen at the cost of false
  information and prevents meaningful recovery.
- **Hide every denied feature.** Users cannot distinguish product scope from a
  temporary failure or understand which role is required.
- **Put all feedback in global toasts.** Transient messages lose the relationship
  to the object and cannot represent stale or blocking states.
- **Keep old detail data on every error.** Preserving context is useful only for
  same-resource refresh and must be explicitly labeled stale.

## Related records

- [Space governance](space-governance.md)
- [Space membership lifecycle](space-membership-lifecycle.md)
- [Portal navigation and Space context](portal-navigation-and-space-context.md)
- [Portal work and execution experience](portal-work-and-execution-experience.md)

