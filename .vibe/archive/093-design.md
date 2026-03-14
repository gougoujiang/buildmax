# Design 093 — Conversation entity, conversation_message, and Tier 1 agentic API

## Goal

Introduce a **conversation** entity (with unique conversation_id and channel) and a **conversation_message** table (role: user, assistant, tool; channel stored for user messages). Expose APIs: list/create conversations, get messages, add message (follow-up). Backend runs a Tier 1 LLM loop with a single demo tool `get_current_date`, persisting every message to the DB. Portal "New Chat" uses these APIs. Chat/chat_run remain unchanged; Tier 1 ↔ Tier 2 integration is later.

## Modules

| Module | Responsibility |
|--------|----------------|
| **internal/model** | Conversation, ConversationMessage domain structs (snake_case json). |
| **internal/util** | Add PrefixConversation, PrefixConversationMessage; NewPrefixedID for cv_, cm_. |
| **internal/storage/entity** | Conversation and ConversationMessage GORM models; ConversationStore, ConversationMessageStore interfaces and Store implementation; AutoMigrate new tables. |
| **internal/server** | New handlers: GET/POST conversations, GET/POST conversation messages; Tier 1 agent loop (load messages → LLM with get_current_date → persist each turn). Config: ConversationStore, ConversationMessageStore; optional LLM config for Tier 1. |
| **internal/cmd/server** | Wire Store (already has DB); ensure new store methods available; pass to server Config. |
| **portal** | New Chat: call POST conversations (channel=portal, message=user input); conversation detail: GET messages, POST messages for follow-up. |

## Data model

**Conversation (table: `conversation`)**

| Column | Type | Notes |
|--------|------|-------|
| id | PK auto | Internal. |
| conversation_id | varchar(64), unique | Prefixed id (cv_). |
| workspace_id | varchar(64), index | Scope. |
| channel | varchar(32) | portal, cron, webhook, telegram. |
| created_by | varchar(64) | user_id. |
| created_at | int64 / bigint | Unix. |

**ConversationMessage (table: `conversation_message`)**

| Column | Type | Notes |
|--------|------|-------|
| id | PK auto | Internal. |
| conversation_message_id | varchar(64), unique | Prefixed (cm_). |
| conversation_id | varchar(64), index | FK to conversation. |
| role | varchar(16) | user, assistant, tool. |
| content | text | Message body. |
| channel | varchar(32), nullable | Set for role=user (source channel). |
| created_at | int64 | Order. |
| (optional) tool_call_id / name | varchar | If needed to pair tool results with calls. |

Singular table names per AGENTS.md.

## Store interface and methods

**ConversationStore**

- `CreateConversation(ctx, workspaceID, channel, createdBy string) (*Conversation, error)` — Generate conversation_id (cv_), insert row.
- `GetConversation(ctx, conversationID string) (*Conversation, error)` — By conversation_id; return (nil, nil) if not found.
- `ListConversationsByWorkspace(ctx, workspaceID string, limit, offset int) ([]Conversation, int, error)` — Order by created_at DESC; total count for pagination.

**ConversationMessageStore**

- `AppendMessage(ctx, conversationID, role, content string, channel *string) (*ConversationMessage, error)` — Generate cm_, insert; channel used when role=user.
- `ListMessages(ctx, conversationID string) ([]ConversationMessage, error)` — Order by created_at ASC.

Conversation must belong to workspace for auth: in handlers, after GetConversation, check conversation.WorkspaceID == workspaceID.

## API contract

**GET** `/api/workspaces/{workspace_id}/conversations`  
Query: optional limit, offset.  
Response 200: `{ "conversations": [ { "id": "cv_xxx", "workspace_id", "channel", "created_at", "created_by" } ], "total": N }`.  
Auth: JWT; workspace access.

**POST** `/api/workspaces/{workspace_id}/conversations`  
Body: `{ "channel": "portal", "message": "optional first user message" }`.  
- Create conversation (channel, created_by from JWT).  
- If `message` present: append user message (channel=body.channel), run Tier 1 LLM loop, persist all new messages.  
Response 201: `{ "conversation_id": "cv_xxx", "reply": "optional first assistant reply if message was sent" }`.  
Auth: JWT; workspace access.

**GET** `/api/workspaces/{workspace_id}/conversations/{conversation_id}/messages`  
Response 200: `{ "messages": [ { "id": "cm_xxx", "role", "content", "channel" (if user), "created_at" } ] }` ordered by created_at.  
Auth: JWT; conversation must be in workspace.

**POST** `/api/workspaces/{workspace_id}/conversations/{conversation_id}/messages`  
Body: `{ "content": "user follow-up text" }`.  
- Append user message (channel from conversation.channel).  
- Run Tier 1 LLM loop; persist user, assistant, and tool messages.  
Response 200 or 201: `{ "reply": "assistant text" }` (and optionally new message ids if needed).  
Auth: JWT; conversation in workspace.

All IDs in response use snake_case (conversation_id, conversation_message_id in payloads if needed).

## Tier 1 agent loop (backend)

- **Input:** conversation_id, new user content, channel (for user message).
- **Steps:**  
  1. Load ListMessages(conversationID).  
  2. Build LLM message list: convert stored messages to llm.Message (user/assistant/tool); append new user message.  
  3. Call LLM with tools: only `get_current_date` (returns current date string; no args or trivial args).  
  4. If assistant response has tool_calls: for each tool call, execute get_current_date, append tool message to DB, append to message list; call LLM again with updated messages. Repeat until no tool_calls or max iterations.  
  5. Append each assistant message and tool message to conversation_message as they are produced.  
- **System prompt:** Short (e.g. "You are a helpful assistant. You can call get_current_date to get today's date."). No workspace or Tier 2 tools.  
- **Persistence:** After appending user message, after each assistant content block, after each tool result, call AppendMessage so the DB has full history.

Placement: new file in `internal/server` (e.g. `conversation_handlers.go` and `conversation_agent.go`) or a small `internal/conversation/engine` package that server calls. Server already has or can receive LLM config; use same config as rest of server or a dedicated Tier 1 model key later.

## Server wiring

- **Config:** Add `ConversationStore entity.ConversationStore` and `ConversationMessageStore entity.ConversationMessageStore` (or Store that implements both).  
- **Routes:** Register GET/POST `.../conversations` and GET/POST `.../conversations/{conversation_id}/messages`.  
- **Handlers:** withWorkspaceAuth(..., "workspace_id"); for conversation_id path, load conversation and verify conversation.WorkspaceID == workspaceID.

## Portal changes

- **New Chat:** Instead of `createChat(workspaceId, { input }, token)` then navigate to chat and createChatRun, call `POST /api/workspaces/{id}/conversations` with `{ "channel": "portal", "message": input }`. Navigate to conversation view (e.g. `#/{workspaceId}/conversation/{conversationId}`).  
- **Conversation view:** Fetch messages with GET `.../conversations/{id}/messages`; on user send, POST `.../conversations/{id}/messages` with `{ "content": userInput }`; display reply.  
- **Routing:** Add route for conversation-by-id; sidebar "New Chat" and list can show recent conversations from GET conversations.

## Changes for review

| Area | Change |
|------|--------|
| internal/util/id.go | Add PrefixConversation ("cv"), PrefixConversationMessage ("cm"). |
| internal/model | Add Conversation, ConversationMessage structs (snake_case json, GORM tags if in model or only in entity). |
| internal/storage/entity | Conversation and ConversationMessage structs; ConversationStore, ConversationMessageStore interfaces; Store implements them; AutoMigrate in New(). |
| internal/server | New handlers (conversations list/create, messages get/add); Tier 1 loop with get_current_date; Config ConversationStore, ConversationMessageStore; register routes. |
| internal/cmd/server | Pass store (existing Store) to Config; Store must implement new interfaces. |
| portal | New Chat → POST conversations; new conversation detail page and GET/POST messages. |

## Tests

- **Entity:** CreateConversation; AppendMessage (user, assistant, tool); ListMessages order; ListConversationsByWorkspace.  
- **Handlers:** Create conversation with and without first message (mock LLM); GET messages; POST message (mock LLM), assert messages in DB.  
- **Tier 1 loop:** Unit test with mock LLM returning tool_call get_current_date; assert tool executed and tool message appended; then LLM returns text; assert assistant message appended.
