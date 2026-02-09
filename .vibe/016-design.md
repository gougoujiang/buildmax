# Design 016 - Web fetch tool

## Goal

Define the structure and APIs for an agent tool **WebFetch** that fetches content from a URL, converts HTML to markdown, optionally processes it with the configured LLM using a user prompt, and returns the result; with a 15-minute in-memory cache and explicit cross-host redirect handling so the LLM can re-call with the redirect URL.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/tools** | Concrete agent tools (Read, Write, **WebFetch**). WebFetch: HTTP fetch, HTML→markdown, optional LLM call, cache, redirect handling. Implements `agent.Tool`. | `readfile.go`, `writefile.go`, **new** `webfetch.go`, `webfetch_test.go` |
| **internal/agent** | Agent loop, `Tool` and `LLMCaller` interfaces, tool invocation. | `agent.go` (unchanged) |
| **internal/llm** | OpenAI-compatible client; `*Client` implements `agent.LLMCaller`. | `client.go`, `types.go` (unchanged) |
| **cmd/buildmax** | CLI entry, agent/session setup, tool and LLM construction. | `root.go` (wiring: add WebFetch to tool list) |

## Structure

**Directory / files**

- `internal/tools/` — agent tools
  - `readfile.go`, `readfile_test.go` — (existing)
  - `writefile.go`, `writefile_test.go` — (existing)
  - **`webfetch.go`** — WebFetch tool: `WebFetch` struct, `NewWebFetch`, cache, HTTP client with custom redirect policy, HTML→markdown, optional LLM call; implements `agent.Tool`
  - **`webfetch_test.go`** — Unit tests: invalid URL, HTTP→HTTPS, same/cross-host redirects, cache, HTML conversion, prompt vs no prompt, truncation; `httptest` and mock `LLMCaller`

- `cmd/buildmax/` — CLI
  - `root.go` — **Edit** `setupAgentAndSession`: create `webFetchTool` with `NewWebFetch(client, 15*time.Minute)` after building client; pass `readFileTool`, `writeFileTool`, `webFetchTool` to `NewAgent`

**Main types and interfaces**

- **WebFetch** (internal/tools): Tool that fetches a URL, converts HTML to markdown, optionally calls the LLM with content + prompt, and returns the result. Holds `caller agent.LLMCaller`, in-memory cache (see below), and `cacheTTL time.Duration`. Implements `agent.Tool` (Name → `"WebFetch"`, Description, Parameters, Execute).
- **cacheEntry** (internal/tools, unexported or local): `result string`, `expiresAt time.Time`. Cache key = final fetched URL (after same-host redirects). Storage: `map[string]cacheEntry` protected by `sync.RWMutex`; on Get, if expired remove and treat as miss.
- **Tool** (internal/agent): Unchanged. `Name()`, `Description()`, `Parameters() any`, `Execute(ctx, args) (string, error)`.
- **LLMCaller** (internal/agent): Unchanged. `ChatWithTools(ctx, messages, tools) (content string, toolCalls []llm.ToolCall, err error)`. WebFetch calls it with `tools == nil` for the “process with prompt” step.

## Method design

| Receiver   | Method       | Signature | Responsibility |
|-----------|--------------|-----------|-----------------|
| (package) | **NewWebFetch** | `(caller agent.LLMCaller, cacheTTL time.Duration) (*WebFetch, error)` | Store caller and TTL; init cache map and mutex. Return `&WebFetch{...}`. If caller is nil, return error. |
| **WebFetch** | **Name** | `() string` | Return `"WebFetch"`. |
| **WebFetch** | **Description** | `() string` | Short text for the LLM: fetch URL, convert HTML to markdown, optional `prompt` to process content with the model; prefer MCP web fetch if available; read-only; content may be truncated if large; cross-host redirect returns redirect URL for a new request; 15-minute cache. |
| **WebFetch** | **Parameters** | `() any` | JSON schema: `type: "object"`, `properties`: `url` (string, required), `prompt` (string, optional). `required`: `["url"]`. Snake_case keys. |
| **WebFetch** | **Execute** | `(ctx context.Context, args map[string]any) (string, error)` | See **Execute flow** below. |

**Execute flow (WebFetch.Execute)**

1. **Parse args**: Extract `url` (required); if missing or not string, return error. Extract `prompt` (optional string); treat empty as “no prompt”.
2. **Normalize URL**: If scheme is `http`, replace with `https` (e.g. `strings.Replace` or `url.Parse` + set `Scheme = "https"` + `String()`).
3. **Cache lookup**: Under read lock, get entry by URL. If found and `time.Now().Before(expiresAt)`, return cached result. If found but expired, remove under write lock and continue. If not found, continue.
4. **HTTP fetch**: Build `*http.Client` with `CheckRedirect` that rejects cross-host redirects: in `CheckRedirect(req, via)`, if `len(via) > 0` then original host = `via[0].URL.Host`; if `req.URL.Host != originalHost`, return a **custom error** that carries the redirect URL (e.g. `errRedirectToOtherHost{url: req.URL.String()}`). Execute `client.Get(normalizedURL)` with a reasonable timeout (e.g. 30s) using `ctx`. If error is the custom redirect error, return the fixed message: `"Redirected to a different host. Fetch this URL instead: <redirect_url>"` (so the LLM can call WebFetch again with that URL). On other errors (e.g. connection failed, timeout), return a clear error string.
5. **Read body**: Defer `resp.Body.Close()`. Read body (e.g. `io.ReadAll`); respect `ctx` cancellation. On non-2xx status, return error with status.
6. **Convert to text**: If Content-Type contains `"html"` or body starts with `<!`, convert HTML to markdown via the chosen library (e.g. `html2text.FromString(html)`); otherwise treat as plain text (UTF-8). If conversion fails, return body as-is or a short error.
7. **Truncate**: Apply max content size (e.g. 200_000 runes). If exceeded, truncate and append `(content truncated; total N characters)`.
8. **Optional LLM step**: If `prompt` is non-empty: build messages = `[{Role: "system", Content: "Answer based only on the following content.\n\n" + content}, {Role: "user", Content: prompt}]`. Call `w.caller.ChatWithTools(ctx, messages, nil)`. Return the returned `content` (and ignore `toolCalls`). On error, return error. If `prompt` is empty, skip this step.
9. **Result**: The result string is either the LLM reply (if prompt was set) or the fetched/converted/truncated content. Store in cache keyed by **final request URL** (the URL that actually returned the body; after same-host redirects this is `resp.Request.URL.String()`). Set `expiresAt = time.Now().Add(w.cacheTTL)`. Return result.

**Cross-host redirect**: Do not cache the redirect response; return the message immediately so the LLM can issue a new WebFetch with the redirect URL.

**Dependencies (external)**  
Add one Go HTML-to-markdown library (e.g. `github.com/jaytaylor/html2text`). Use it only in `webfetch.go`. Prefer pure-Go, simple API: HTML string in → markdown string out.

## How they work together

**Data/control flow**

1. **Setup**: `setupAgentAndSession` loads config, gets cwd, creates `readFileTool`, `writeFileTool`, then **builds `client := llm.NewClient(cfg)`**, then `webFetchTool, err := tools.NewWebFetch(client, 15*time.Minute)`, then `agent.NewAgent(client, []agent.Tool{readFileTool, writeFileTool, webFetchTool})`. Agent builds `toolDefs` and `toolsByName` including `"WebFetch"`.
2. **Agent loop**: User message → `Process` / `ProcessAfterUserAppended` → `processLoop` → `ChatWithTools(messages, toolDefs)` → LLM may return a tool_call with name `"WebFetch"` and arguments `{"url": "...", "prompt": "..."}` (prompt optional).
3. **Tool execution**: `processOneToolCall` looks up `a.toolsByName["WebFetch"]`, unmarshals arguments, calls `tool.Execute(ctx, args)`. WebFetch: cache lookup → fetch (or return redirect message) → convert → truncate → optional LLM call → cache store → return result. Result is appended as tool-role message; loop continues or returns final reply.

**Dependencies**

- **internal/tools** depends on **internal/agent** for `Tool` and `LLMCaller` (WebFetch holds and calls LLMCaller). No dependency from agent to tools except at construction in cmd.
- **cmd/buildmax** imports **internal/tools**, **internal/agent**, **internal/llm**; constructs `*llm.Client` (implements `agent.LLMCaller`), passes it to `NewWebFetch` and to `NewAgent`.

**Key data structures**

- **args** for WebFetch: `map[string]any` with `url` (string, required), `prompt` (string, optional). Produced by LLM JSON, consumed by `WebFetch.Execute`.
- **WebFetch.caller**: Used only when `prompt` is non-empty; `ChatWithTools(ctx, messages, nil)` returns the model reply as the tool result.
- **cacheEntry**: Holds result and expiry; cache key = final fetched URL so same URL + same-host redirects hit cache.

## Changes for review

- **New**: `internal/tools/webfetch.go` — `WebFetch` struct (`caller agent.LLMCaller`, cache map + mutex, `cacheTTL time.Duration`); `NewWebFetch(caller, cacheTTL) (*WebFetch, error)`; `Name()`, `Description()`, `Parameters()`, `Execute()` implementing `agent.Tool`. Custom HTTP `CheckRedirect` returning an error that carries redirect URL when host changes; normalized URL (HTTP→HTTPS); HTML detection and conversion via chosen library; max content size 200_000 runes with truncation note; optional LLM call via `caller.ChatWithTools(ctx, messages, nil)`; in-memory cache with TTL and lazy expiry on access.
- **New**: `internal/tools/webfetch_test.go` — Unit tests: missing/invalid `url`; HTTP→HTTPS upgrade; same-host redirect returns content; cross-host redirect returns message with redirect URL; cache hit returns cached result; cache miss/expiry triggers fetch; HTML→markdown conversion; non-empty `prompt` returns LLM reply (mock LLMCaller); empty `prompt` returns raw content; oversized content truncated with note. Use `net/http/httptest` for servers; mock `agent.LLMCaller` for prompt path.
- **Modified**: `cmd/buildmax/root.go` — In `setupAgentAndSession`, **reorder**: create `client := llm.NewClient(cfg)` immediately after validating config and cwd (so the same client is used for both the agent and WebFetch). Then create `readFileTool`, `writeFileTool`, and `webFetchTool := tools.NewWebFetch(client, 15*time.Minute)`; on WebFetch error log and return. Pass `[]agent.Tool{readFileTool, writeFileTool, webFetchTool}` to `NewAgent(client, tools)`. Add `"time"` import.
- **Modified**: `go.mod` / `go.sum` — Add dependency for HTML-to-markdown library (e.g. `github.com/jaytaylor/html2text`).

**Clarification (root.go)**  
Current flow creates `client` after the two tools. For WebFetch, the same `client` must be passed to both `NewWebFetch(client, ...)` and `NewAgent(client, ...)`. Reorder so that `client := llm.NewClient(cfg)` is created once right after config and cwd are validated, then create all three tools, then `NewAgent(client, tools)`.
