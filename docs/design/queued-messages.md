# Queued Messages

> **Audience:** contributors · **Status:** phase 1 implemented on CLI/TUI,
> Desktop, and Portal; mid-run injection not started

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

A message that arrives during a run is **queued and runs as its own turn** once
the current turn finishes. Nothing is injected into a running turn — see
[Not done yet](#not-done-yet).

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
| CLI/TUI | `Model.queue` | Queues, prints a dim `⏸ queued #n` line | `agentDoneMsg` → `drainQueueMsg` → next run |
| Desktop | `App.queues`, per project | `SendMessageStream` returns the queue position | The run goroutine loops over turns |
| Portal | `Handler.turns`, per conversation | `conversation.message.queued` event | `turnRegistry` drain goroutine |

The TUI refuses to queue a slash command: those act on live UI state (model
switch, session load, panels), so running one a turn later would apply it to a
different world than the one the user was looking at.

Desktop reports the queue position as a **return value** of `SendMessageStream`
rather than an event, because it answers the caller's own call. `QueuedMessages`
lets the frontend re-read a queue it was not mounted for.

### Portal: the registry moved off the connection

`turnRegistry` (`internal/server/handlers/conversation_turns.go`) is server-scoped
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

## Not done yet

**Mid-run injection.** The turn boundary here is the whole run, so a message
queued behind a long tool-calling run waits for all of it. `RunLoop` has a clean
seam for the smaller boundary — the top of each iteration, where tool results are
complete and a `user` message can be appended without breaking the
`assistant(tool_calls)` ↔ `tool` pairing. That would let a queued message reach
the model after the current batch of tools instead of after the whole run, and it
would use the same `MessageQueue`. It is not implemented, and no surface offers
it.

**Persistence.** Nothing above survives a restart. Whether queued Portal turns
should be durable is open, and depends on whether a queued message should also be
visible to other sessions watching the conversation.
