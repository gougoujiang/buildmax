# Design 095: Tier 1 agent — chat management tools and system prompt

**Task:** [095.md](095.md)

## Goal

Add ListChats, GetChat, and ContinueChat tools to the Tier 1 conversation agent (camel-case names); optimize the Tier 1 system prompt (coordinator role, decision order, tool usage); and inject the latest 5 chats as a snippet into the system prompt so the agent has context without calling ListChats first.

## Modules and structure

| Package | Responsibility |
|--------|----------------|
| **internal/tools** | Runner interfaces and tool implementations for ListChats, GetChat, ContinueChat; tool name constants; no dependency on entity or app. |
| **internal/conversation** | Build tool list from optional runners; build effective system prompt (base + optional recent chats snippet); RunLoop/RunLoopStream accept runners struct and snippet. |
| **internal/app/conversation** | Implement runners using ChatService/ChatStore; before each turn, fetch latest 5 chats and format snippet; call RunLoop/RunLoopStream with runners and snippet. |

## Types and interfaces

### internal/tools

**New constants** (in `names.go`):

- `ToolNameListChats = "ListChats"`
- `ToolNameGetChat = "GetChat"`
- `ToolNameContinueChat = "ContinueChat"`

**ListChats**

- **ListChatsRunner** (interface):  
  `ListChats(ctx context.Context, workspaceID string) (summary string, err error)`  
  Returns a single LLM-oriented string (e.g. one line per chat: chat_id, title/snippet, status, created_at). Caller caps at 10 chats.
- **listChatsTool** (struct): holds `workspaceID string`, `runner ListChatsRunner`; implements `core.Tool` (Name, Description, Parameters, Execute).
- **NewListChatsTool**(workspaceID string, runner ListChatsRunner) core.Tool  
  If runner is nil, Execute returns "ListChats not configured".

**GetChat**

- **GetChatRunner** (interface):  
  `GetChat(ctx context.Context, workspaceID, chatID string) (detail string, err error)`  
  Returns LLM-oriented detail string or error (e.g. "chat not found or not in this workspace").
- **getChatTool** (struct): holds `workspaceID string`, `runner GetChatRunner`.
- **NewGetChatTool**(workspaceID string, runner GetChatRunner) core.Tool  
  Parameters: `chat_id` (required). Execute: validate chat_id; call runner; return detail or error message.

**ContinueChat**

- **ContinueChatRunner** (interface):  
  `ContinueChat(ctx context.Context, workspaceID, userID, chatID, input string) (runID string, err error)`  
  Creates a new run for the chat; workspace-scoped (error if chat not in workspace).
- **continueChatTool** (struct): holds `workspaceID`, `userID` string, `runner ContinueChatRunner`.
- **NewContinueChatTool**(workspaceID, userID string, runner ContinueChatRunner) core.Tool  
  Parameters: `chat_id`, `input` (required). Execute: validate; call runner; return short confirmation with run_id or error.

Tool **Descriptions** and **Parameters** (JSON schema) must be clear for the LLM (when to use, required args). **Execute** returns meaningful success/error strings per AGENTS.md (tool output for LLM).

### internal/conversation

**ConversationToolRunners** (struct, optional runners; nil means do not add that tool):

```go
type ConversationToolRunners struct {
    StartChat   tools.StartChatRunner
    ListChats   tools.ListChatsRunner
    GetChat     tools.GetChatRunner
    ContinueChat tools.ContinueChatRunner
}
```

**buildConversationTools**(workspaceID, userID string, runners *ConversationToolRunners) []core.Tool

- Start from `DefaultConversationTools()` (GetCurrentDate only).
- If runners != nil: append StartChat if runners.StartChat != nil, ListChats if runners.ListChats != nil, GetChat if runners.GetChat != nil, ContinueChat if runners.ContinueChat != nil.
- Tools are built with the given workspaceID/userID so they are workspace- and user-scoped.

**System prompt**

- Replace the current `systemPrompt` constant with a **base** prompt that includes:
  - Role: You are a coordinator between the user and background chat tasks.
  - Decision order: First consider whether to continue an existing chat (ContinueChat) rather than creating a new one (StartChat). Use ListChats/GetChat or the injected recent chats to decide.
  - Tools: GetCurrentDate, StartChat, ListChats, GetChat, ContinueChat — when to use each; reply concisely; when starting or continuing a task, tell the user the chat id (and run id) and where to check progress.
- **Effective prompt**: `effectiveSystemPrompt(basePrompt, recentChatsSnippet string) string`. If `recentChatsSnippet` is non-empty, append `"\n\n" + recentChatsSnippet` (e.g. "Recent chats in this workspace (latest 5):\n" + snippet). Otherwise return base prompt only.

**RunLoop / RunLoopStream**

- Add parameter: **recentChatsSnippet string** (optional). When building options for `agent.RunLoop`, use `effectiveSystemPrompt(systemPrompt, recentChatsSnippet)` as `SystemPrompt`.
- Replace **startChatRunner** parameter with **runners *ConversationToolRunners**. When `toolsList` is nil, build via `buildConversationTools(workspaceID, userID, runners)`.

Signatures become:

- **RunLoop**(ctx, convStore, msgStore, caller, conversationID, userContent, channel, toolsList, workspaceID, userID, **runners *ConversationToolRunners**, titleGenerator, **recentChatsSnippet string**) (reply string, err error)
- **RunLoopStream**(..., **runners *ConversationToolRunners**, titleGenerator, sink, **recentChatsSnippet string**) (reply string, err error)

Backward compatibility: callers that pass `runners == nil` get only default tools (GetCurrentDate); no StartChat. So app layer must always pass a non-nil runners struct when it wants StartChat and the new tools.

### internal/app/conversation

**Runners implementation**

- **startChatRunner** (existing): unchanged; still implements `tools.StartChatRunner`.
- **listChatsRunner** (new): type that holds `*chatapp.Service` or `entity.ChatStore`; implements `ListChats(ctx, workspaceID)` by calling `ListChatsByWorkspacePaginated(ctx, workspaceID, false, 10, 0)` and formatting each chat to a short line (chat_id, title or input snippet, status, created_at); returns concatenated string.
- **getChatRunner** (new): holds ChatStore; `GetChat(ctx, workspaceID, chatID)` calls `GetChat(ctx, chatID)`, then if chat != nil checks `chat.WorkspaceID == workspaceID`; if not, return error "chat not found or not in this workspace"; else format chat to LLM-friendly detail string (chat_id, title, input truncated e.g. 500 runes, status, created_at, last_run_id, optional output snippet).
- **continueChatRunner** (new): holds `*chatapp.Service`; `ContinueChat(ctx, workspaceID, userID, chatID, input)` first verifies chat exists and belongs to workspace (e.g. GetChat then check WorkspaceID), then calls `ChatService.CreateRun(ctx, CreateRunCmd{UserID: userID, ChatID: chatID, Input: input})`; returns run_id or error.

**Building runners and snippet for a turn**

- In `handleConversationTurn`, build:
  - `runners := &coreconv.ConversationToolRunners{ StartChat: s.startChatRunner(...), ListChats: s.listChatsRunner(), GetChat: s.getChatRunner(), ContinueChat: s.continueChatRunner() }` (or equivalent; getChat/listChats/continueChat can be methods that return the same runner instance if stateless).
- **Recent chats snippet**: call a new helper e.g. `s.recentChatsSnippet(ctx, cmd.WorkspaceID)` that: uses ChatStore (or ChatService) to list latest 5 (`ListChatsByWorkspacePaginated(ctx, workspaceID, false, 5, 0)`); format each to one line (chat_id, title/snippet, status, created_at); return "Recent chats in this workspace (latest 5):\n" + lines (or "No recent chats." if empty).
- Pass `runners` and `recentChatsSnippet` into `RunLoop` / `RunLoopStream`.

**ChatService vs ChatStore**

- ListChats and GetChat need only read access; ContinueChat needs CreateRun. App/conversation already has `ChatService *chatapp.Service`; ChatService has Chats (ChatStore) and ChatRuns. So listChatsRunner and getChatRunner can use `s.ChatService.Chats` (or expose ChatStore on Service if not already). ContinueChat uses `s.ChatService.CreateRun`. Prefer using ChatService so workspace/quota rules apply; for GetChat workspace check we only need to read one chat and compare WorkspaceID — ChatStore.GetChat is enough. So: listChatsRunner and getChatRunner can use ChatStore (either from Service or injected); continueChatRunner uses ChatService.CreateRun and must verify chat in workspace first (e.g. GetChat via ChatStore, then check WorkspaceID, then CreateRun).

## Method design (summary)

| Location | Method / Function | Responsibility |
|----------|-------------------|----------------|
| tools | ListChatsRunner.ListChats(ctx, workspaceID) (string, error) | Return formatted list of up to 10 recent chats. |
| tools | GetChatRunner.GetChat(ctx, workspaceID, chatID) (string, error) | Return formatted chat detail or workspace error. |
| tools | ContinueChatRunner.ContinueChat(ctx, workspaceID, userID, chatID, input) (runID string, err error) | Create new run; workspace-scoped. |
| tools | NewListChatsTool, NewGetChatTool, NewContinueChatTool | Return core.Tool implementations. |
| conversation | buildConversationTools(workspaceID, userID, runners *ConversationToolRunners) []core.Tool | Build default + optional tools from runners. |
| conversation | effectiveSystemPrompt(base, recentSnippet string) string | Append recent snippet when non-empty. |
| conversation | RunLoop(..., runners *ConversationToolRunners, titleGen, recentChatsSnippet string) | Use effective prompt and built tools. |
| conversation | RunLoopStream(..., runners *ConversationToolRunners, titleGen, sink, recentChatsSnippet string) | Same. |
| app/conversation | listChatsRunner(), getChatRunner(), continueChatRunner() | Return runners that use ChatService/ChatStore. |
| app/conversation | recentChatsSnippet(ctx, workspaceID) string | Fetch 5 chats, format, return snippet for prompt. |

## Data flow

1. **Portal (or other) turn** → `app/conversation.Service.HandleTurn(cmd)` with ConversationID set.
2. **handleConversationTurn**:
   - Build `runners` with StartChat, ListChats, GetChat, ContinueChat (all backed by ChatService/ChatStore).
   - Call `recentChatsSnippet(ctx, cmd.WorkspaceID)` → string (latest 5 chats formatted).
   - Call `conversation.RunLoopStream` or `RunLoop` with `runners`, `recentChatsSnippet`, and existing args.
3. **RunLoop/RunLoopStream** (conversation):
   - If toolsList is nil, `toolsList = buildConversationTools(workspaceID, userID, runners)`.
   - `effectivePrompt := effectiveSystemPrompt(systemPrompt, recentChatsSnippet)`.
   - Run `agent.RunLoop` with effectivePrompt and toolsList.
4. **Agent** may call ListChats, GetChat, or ContinueChat; each tool calls its runner with workspaceID (and userID for ContinueChat); runner uses ChatStore/ChatService; result string returned to LLM.

## Format of snippet and tool output

- **Recent chats snippet** (in system prompt): Plain text, one line per chat, e.g. `chat_id | title_or_snippet | status | created_at`. Snippet length per chat bounded (e.g. title or first 60 runes of input). Created_at can be human-readable or Unix.
- **ListChats tool result**: Same style as snippet, or slightly more structured (e.g. numbered list); LLM-friendly.
- **GetChat tool result**: chat_id, title, input (truncated), status, created_at, last_run_id; optionally one line for last run status or output preview.
- **ContinueChat tool result**: "Follow-up scheduled. chat_id: c_xxx, run_id: r_xxx. The new run will execute in the background."

## Changes for review

| Package / File | Change |
|----------------|--------|
| **internal/tools/names.go** | Add ToolNameListChats, ToolNameGetChat, ToolNameContinueChat. |
| **internal/tools/list_chats.go** (new) | ListChatsRunner interface, listChatsTool, NewListChatsTool. |
| **internal/tools/get_chat.go** (new) | GetChatRunner interface, getChatTool, NewGetChatTool. |
| **internal/tools/continue_chat.go** (new) | ContinueChatRunner interface, continueChatTool, NewContinueChatTool. |
| **internal/conversation/agent.go** | Define ConversationToolRunners; replace systemPrompt with expanded base prompt; add effectiveSystemPrompt(base, snippet); change buildConversationTools to take *ConversationToolRunners; add recentChatsSnippet param to RunLoop and RunLoopStream; use effectiveSystemPrompt in both. |
| **internal/app/conversation/service.go** | Add listChatsRunner, getChatRunner, continueChatRunner implementations; add recentChatsSnippet(ctx, workspaceID); in handleConversationTurn build runners (including StartChat), compute snippet, pass runners and snippet to RunLoop/RunLoopStream. |
| **internal/tools/*_test.go** | Unit tests for ListChats (nil runner, success), GetChat (nil runner, missing chat_id, not in workspace, success), ContinueChat (nil runner, missing args, not in workspace, success). |

No changes to entity, server routes, or portal UI. ChatStore and ChatService already provide the required operations.
