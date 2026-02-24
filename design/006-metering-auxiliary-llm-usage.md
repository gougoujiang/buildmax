# Design 006: Metering auxiliary LLM usage (e.g. title generation)

## Goal

Accurate user/workspace metering for billing: every LLM call (including title generation and future helper calls) is counted.

## Attribution


| LLM call            | Attributed to     | Storage                                                                |
| ------------------- | ----------------- | ---------------------------------------------------------------------- |
| Main agent run      | ChatRun / Session | Existing: `chat_run.prompt_tokens` / `completion_tokens`, session file |
| Session title (CLI) | Session           | Session file via `AddUsage` after title gen                            |
| Chat title (server) | Chat              | `chat.title_prompt_tokens`, `chat.title_completion_tokens`             |


Billing can sum: run usage (per chat_run) + chat title usage (per chat) + session usage (CLI session files).

## Changes (implemented)

1. **Session package**
  - `TitleChatClient.Chat` returns `(string, llm.Usage, error)`.  
  - `GenerateTitle` and `GenerateTitleFromInput` return `(string, llm.Usage, error)`.
2. **CLI (TUI + print)**
  - After LLM title generation, call `Session.AddUsage(usage)` and persist so session file reflects total (run + title).
3. **Server**
  - `ChatTitleGenerator.GenerateChatTitle` returns `(title string, usage TokenUsage, err error)`.  
  - Create-chat handler passes title usage into `CreateChat(..., titlePromptTokens, titleCompletionTokens)`.
4. **Chat model and storage**
  - `Chat` has `TitlePromptTokens` and `TitleCompletionTokens` (int, 0 when title is truncated input).  
  - `CreateChat` accepts and stores these two fields.

## Future auxiliary calls

Any new LLM call (e.g. summarization, classification) should either:

- Be attributed to an existing run/session (and use run/session usage), or  
- Be stored as auxiliary usage (e.g. more columns or a small usage-events table) so billing can sum everything.

