# Queued Messages

> **Audience:** contributors · **Status:** implemented. Turn-boundary queueing on
> CLI/TUI, Desktop, and Portal; mid-run injection on CLI/TUI and Desktop.
> Persistence and Portal injection are decided against.

## Problem

Every surface refused a message typed while the agent was working, and each one
refused it differently:

- The TUI swallowed `Enter` while `busy`, but still fed the keystrokes to the
  hidden input. Text typed during a run was invisible until the run ended, then
  reappeared without explanation.
- Portal answered a second WebSocket message with
  `conversation.error: "a conversation turn is already in progress"` and dropped
  it. The composer was disabled during a turn, so the user could not even type.
- Desktop returned `"a run is already in progress for this project"`.
- The Portal HTTP path serialized nothing at all: two concurrent `POST
  .../messages` both read the message history, both appended to it, and the two
  turns interleaved their writes.

A user who thinks of one more thing mid-run has to hold it in their head, watch
for the run to end, and type it again. That is work the runtime should be doing.

## Approach

A message that arrives during a run is **queued**, and how it is delivered depends
on the surface:

- CLI/TUI and Desktop hand the queue to the run itself, which drains it at its
  next iteration boundary. The message reaches the model after the current batch
  of tool calls, not after the whole run.
- Portal queues the message as its own turn behind the running one. Its Tier 1
  turns are short, and its queue is shared server state — see
  [Why Portal does not inject](#why-portal-does-not-inject).

Either way the boundary is a turn boundary of some size, and no message ever
lands in the middle of a tool batch.

The queue itself is one shared primitive, `agent.MessageQueue` in
`internal/core/agent/queue.go`: enqueue with a position, dequeue oldest first,
drop the last (an undo), drop all, and a snapshot for display. Each surface owns
its queues and drains them at its own turn boundary; the primitive has no opinion
about when that is.

Capacity is `agent.DefaultMaxQueuedMessages` (10) per conversation. A full queue
**rejects** rather than evicting: dropping the oldest would silently lose a turn
the user already believes is scheduled.

### Cancel drops the queue

Stopping a run discards everything queued behind it. Those messages were written
for work the user has just called off, and running them afterwards would restart
it in their name. Desktop does this in `App.CancelRun`, before cancelling the
run's context.

A **failed** run does not drop the queue. Failure is not a decision the user
made, and stranding their messages with no run left to release them is worse than
letting each fail on its own turn.

### Where each surface queues

| Surface | Queue owner | Enter during a run | Drain point |
|---|---|---|---|
| CLI/TUI | `Model.queue` | Queues, prints a dim `⏸ queued #n` line | `RunLoop` at the next iteration; `agentDoneMsg` → `drainQueueMsg` as backstop |
| Desktop | `App.queues`, per project | `SendMessageStream` returns the queue position | `RunLoop` at the next iteration; the run goroutine's turn loop as backstop |
| Portal | `Handler.turns`, per conversation | `conversation.message.queued` event | `turnRegistry` drain goroutine |

The backstops still matter on the two local surfaces: a message queued after the
run stopped reading its queue — in the window between the last iteration and the
run returning — is picked up as a fresh run.

The TUI refuses to queue a slash command: those act on live UI state (model
switch, session load, panels), so running one a turn later would apply it to a
different world than the one the user was looking at.

Desktop reports the queue position as a **return value** of `SendMessageStream`
rather than an event, because it answers the caller's own call. `QueuedMessages`
lets the frontend re-read a queue it was not mounted for.

### Portal: the registry moved off the connection

`turnqueue.Registry` (`internal/server/turnqueue`) is server-scoped
and keyed by conversation. It replaced a `map[string]*sync.Mutex` that lived on
the WebSocket connection, which was the wrong owner: a conversation outlives any
one socket, and it is reachable from a reconnected socket, a second tab, the HTTP
API, and a system turn reporting a finished task. Anchoring serialization to a
connection let two of those interleave their reads and writes of one history.

The registry serves both shapes of caller:

- `Submit` is fire-and-forget, for WebSocket turns and system turns. System turns
  used to hold a goroutine on a blocking `Lock`; they now queue like everything
  else.
- `RunSync` submits and waits, for an HTTP request that has to stream its own turn
  back. A streaming request writes its SSE headers before waiting, so the client
  sees the stream open. A caller that disconnects before its turn starts marks the
  job dropped and the queue moves on.

Idle conversation queues are released, so a long-lived server does not accumulate
one entry per conversation it has ever served.

New WebSocket events: `conversation.message.queued` (with a 1-based position) and
`conversation.message.dequeued` (that message is starting now).
`conversation.message.completed` carries `queued_remaining`, so a client can tell
"idle" from "between turns" and keep the composer in its busy state across the
gap. A queue-full refusal is a `conversation.error` with `code: "queue_full"` —
the turn in flight is still running, and a client must not read that error as the
conversation going idle.

## Limits

The queue is in memory. A server restart loses queued turns that had not started,
and a queued message is not written to `conversation_message` until its own turn
begins — the running turn's history stays exactly what the model was answering.
Desktop and TUI queues are process state for the same reason.

A Portal message queued from one browser tab is announced to that connection.
Another tab watching the same conversation sees the turn once it starts, not
while it waits.

## Mid-Run Injection

`RunLoopOpts.PendingInput` is drained at the top of every iteration. Everything
waiting is appended to the history as user messages before the history is read and
before the compaction check, so the message is part of what the iteration reasons
about and part of what the context-pressure decision counts.

The iteration boundary is the only safe insertion point: the previous iteration's
tool results are complete there, so a user message does not come between an
`assistant(tool_calls)` message and its `tool` replies, which providers reject.

Injected messages go through the **`UserPromptSubmit` hook** like any other
prompt. A hook that inspects what the user sends must not be bypassed by the path
that happens to arrive mid-run; a deployment blocking secrets in prompts would
otherwise have a hole exactly the width of this feature. A blocked message is
dropped with `EventUserInputBlocked` rather than appended, leaving the run working
on what it already had.

`EventUserInput` announces an injected message so a surface can move it from
"queued" to "sent", and both events are written to the run trace: a message that
entered a run after it started is part of that run's instructions, and a trace
that omitted it would misreport them.

`*MessageQueue` satisfies `PendingInput`, so the same queue serves both
deliveries: whichever of the run and the surface reaches a waiting message first
owns it, and the mutex makes that race harmless.

### Why Portal does not inject

Portal keeps turn-boundary queueing:

- Tier 1 turns are short. They mostly dispatch work to Tier 2, where the long
  runs actually happen — and a task run takes no user input mid-flight either
  way. The wait injection would remove is small.
- Its queue is server state shared by every client watching the conversation.
  Injection would fold two user messages into one turn, so
  `conversation.message.completed` would no longer mark one message answered, and
  every client's view of "what is waiting" would have to be rebuilt around a
  different event.
- The conversation runtime wires no `EventSink` at all, so announcing an
  injection would mean threading one through `ConversationService` first.

This follows the repository's own rule: the capability lives in the shared
runtime, and each surface exposes it in the way that fits its job.

## Deliberately Not Done

**Persistence.** A queue stays process state. A restart loses turns that had not
begun, and the person who queued them is still there to send them again. Making
them durable would also decide a larger question by accident: a queued message
written before its own turn starts is a message every other session watching the
conversation can see, which is a different feature from holding one user's
next turn.

**Portal injection.** Not built, for the reasons in
[Why Portal does not inject](#why-portal-does-not-inject) — the wait it would
remove is small, and it would fold two user messages into one turn. The core
seam is there the day Tier 1 turns get long enough to need it.
