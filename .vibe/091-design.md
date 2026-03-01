# Design 091 — Tier 1 interfaces and channel abstraction

## Goal

Define the Tier 1 conversation contract in a new package: types (`ConversationTurn`, `ConversationResult`), channel constants, and interfaces (`ChannelAdapter`, `ConversationEngine`) so Phase 2+ can implement adapters and engines without depending on server or storage from this package.

## Modules


| Module (package)          | Responsibility                                                                                         | Owns                                                                                  |
| ------------------------- | ------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------- |
| **internal/conversation** | Tier 1 contract: normalized turn/result types, channel names, and interfaces for adapters and engines. | Types, channel constants, `ChannelAdapter`, `ConversationEngine`. No implementations. |


## Structure

**Directory / files**

- `internal/conversation/` — Tier 1 conversation layer contract
  - `types.go` — structs `ConversationTurn`, `ConversationResult`; channel constants; optional `ValidChannel(ch string) bool` helper.
  - `interfaces.go` — `ChannelAdapter` and `ConversationEngine` interfaces with doc comments.
  - `types_test.go` — unit tests for type construction, channel constants, and helper (if any).

**Main types and interfaces**

- **ConversationTurn** (conversation): Normalized input from any channel. Fields: WorkspaceID, Channel, ConversationID, UserID, Message, Raw (map[string]any). Channel = which channel; ConversationID = channel-specific conversation/thread id (e.g. chat_id for portal).
- **ConversationResult** (conversation): Output of processing one turn. Fields: Reply (string), TaskIDs ([]string, chat_run_ids).
- **ChannelAdapter** (conversation): Normalizes channel input and sends output. Receive(ctx, raw) → ConversationTurn; Send(ctx, conversationID, output).
- **ConversationEngine** (conversation): Processes one turn. Process(ctx, workspaceID, chatID, turn) → ConversationResult.

## Method design


| Receiver (interface)   | Method  | Signature                                                                                              | Responsibility                                                                                                      |
| ---------------------- | ------- | ------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------- |
| **ChannelAdapter**     | Receive | `(ctx context.Context, raw any) (ConversationTurn, error)`                                             | Normalize channel-specific input into a ConversationTurn.                                                           |
| **ChannelAdapter**     | Send    | `(ctx context.Context, conversationID string, output string) error`                                    | Deliver output (reply or task result) to the channel; conversationID is channel-specific (e.g. chat_id for portal). |
| **ConversationEngine** | Process | `(ctx context.Context, workspaceID, chatID string, turn ConversationTurn) (ConversationResult, error)` | Process one turn; may return reply and/or task_ids (chat_run_ids).                                                  |


**Helper (optional)**


| Package      | Function     | Signature          | Responsibility                                          |
| ------------ | ------------ | ------------------ | ------------------------------------------------------- |
| conversation | ValidChannel | `(ch string) bool` | Return true if ch is one of the four channel constants. |


## How they work together

**Data/control flow (future use; not implemented in this task)**

1. Caller (e.g. server handler) obtains channel-specific raw input (HTTP body, webhook payload, etc.).
2. Caller invokes a `ChannelAdapter` implementation: `Receive(ctx, raw)` → `ConversationTurn`.
3. Caller invokes a `ConversationEngine` implementation: `Process(ctx, workspaceID, chatID, turn)` → `ConversationResult`.
4. Caller uses `Result.Reply` and `Result.TaskIDs` (e.g. create runs, stream reply). Optionally calls `adapter.Send(ctx, conversationID, output)` to push output back to the channel.

**Dependencies**

- `internal/conversation` imports only standard library (`context`) and no other `internal/` packages.
- Future packages (`server`, cron, webhook handlers) will depend on `internal/conversation` and provide adapter/engine implementations.

**Key data structures**

- **ConversationTurn**: Created by ChannelAdapter.Receive; consumed by ConversationEngine.Process. Carries workspace, channel, user, message, and optional raw payload.
- **ConversationResult**: Returned by ConversationEngine.Process; consumed by caller. Carries optional reply text and list of chat_run_ids.

## Tests

- **types_test.go**
  - Construct `ConversationTurn` with all fields set; assert fields are stored and readable.
  - Construct `ConversationResult` with Reply and TaskIDs; assert equality or field access.
  - Assert all four channel constants are non-empty and pairwise distinct.
  - If `ValidChannel` exists: test each constant returns true; test unknown string returns false.
  - No type that implements `ChannelAdapter` or `ConversationEngine`; no mocks required for this task.

## Changes for review

- **New**: `internal/conversation/types.go` — ConversationTurn, ConversationResult, channel constants (ChannelPortal, ChannelTelegram, ChannelCron, ChannelWebhook), optional ValidChannel(ch string) bool.
- **New**: `internal/conversation/interfaces.go` — ChannelAdapter (Receive, Send), ConversationEngine (Process) with doc comments.
- **New**: `internal/conversation/types_test.go` — unit tests for types, constants, and ValidChannel.

