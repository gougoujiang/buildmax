# Design 080: Streaming mode in BuildMax CLI agent

## Goal

Enable streaming of LLM response content so the user sees assistant text as it arrives in both print mode and TUI mode, without changing session or persistence semantics.

## Modules


| Module (package)   | Responsibility                                                                                                                      | Owns                                                                                                              |
| ------------------ | ----------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| **internal/llm**   | OpenAI-compatible client; non-streaming and streaming chat-with-tools.                                                              | `Client`, `ChatWithTools`, `ChatWithToolsStream`; request/response mapping.                                       |
| **internal/agent** | Agent loop; optional stream sink for content deltas; session append unchanged.                                                      | `Agent`, `LLMCaller`, `StreamSink`, `Process` / `ProcessAfterUserAppended` with process options.                  |
| **internal/cmd**   | Print mode: wire stream sink that writes deltas to stdout.                                                                          | `runPrintMode` with `agent.WithStreamSink(...)`.                                                                  |
| **internal/tui**   | TUI: stream sink that sends `streamDeltaMsg`; model holds streaming buffer; viewport shows buffer then finalizes on `agentDoneMsg`. | `streamDeltaMsg`, streaming buffer in `Model`, `buildViewportContent`/`RefreshAndGotoBottom` with streaming tail. |


## Structure

**Directory / files**

- `internal/llm/` — LLM client
  - `client.go` — existing `ChatWithTools`; add `ChatWithToolsStream`.
  - `types.go` — unchanged (Message, ToolDef, ToolCall).
- `internal/agent/` — Agent loop and streaming hook
  - `agent.go` — `StreamSink` interface; `ProcessOption` and `WithStreamSink`; `processLoop` takes optional sink and calls caller’s streaming or non-streaming method.
  - `agent_test.go` — tests for streaming path (mock caller that streams; assert sink receives deltas and session has full message).
- `internal/cmd/` — CLI
  - `print.go` — `runPrintMode` builds a sink that writes to `os.Stdout` (with flush) and passes it to `Process` via options.
- `internal/tui/` — TUI
  - `model.go` — `streamDeltaMsg` type; `Model.streamingBuffer`, `Model.streamChannel`; handle `streamDeltaMsg` (append to buffer, refresh viewport); handle a “stream channel ready” msg to start reading from channel; run agent with sink that sends deltas and final `agentDoneMsg` on a channel; Cmd that reads next msg from channel.
  - `format.go` — `buildViewportContent(..., streamingTail string)`; when non-empty, append streaming tail as current assistant line; when busy and no tail, keep carousel.
  - `viewport_block.go` — `RefreshAndGotoBottom(..., streamingTail string)`; pass through to `buildViewportContent`.

**Main types and interfaces**

- **StreamSink** (agent): interface with `OnDelta(delta string)`. Called for each content delta from the LLM stream.
- **LLMCaller** (agent): extended with `ChatWithToolsStream(ctx, messages, tools, onDelta func(string)) (content string, toolCalls []ToolCall, err error)`. When streaming, caller invokes `onDelta` for each content delta; at stream end returns full content and tool_calls.
- **processConfig** (agent, internal): holds optional `StreamSink`; populated from `ProcessOption` (e.g. `WithStreamSink(sink)`). Passed into `processLoop`.
- **streamDeltaMsg** (tui): struct `{ Delta string }`; tea.Msg sent when a content delta arrives.
- **streamChannelReadyMsg** (tui): struct `{ Channel chan tea.Msg }`; tea.Msg sent once when agent goroutine is started, so the TUI can schedule Cmds that read from `Channel` until closed (then agentDoneMsg is received).

## Method design


| Receiver                | Method                   | Signature                                                                                          | Responsibility                                                                                                                                                                                                                                                                                               |
| ----------------------- | ------------------------ | -------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Client** (llm)        | ChatWithTools            | `(ctx, messages, tools) (content, toolCalls, err)`                                                 | Unchanged: non-streaming completion.                                                                                                                                                                                                                                                                         |
| **Client** (llm)        | ChatWithToolsStream      | `(ctx, messages, tools, onDelta func(string)) (content string, toolCalls []ToolCall, err error)`   | Build request as in ChatWithTools; call `CreateChatCompletionStream`; for each chunk with `delta.Content`, call `onDelta(delta.Content)`; accumulate full content and tool_calls from stream; on stream end return them; close stream and handle errors.                                                     |
| **Agent** (agent)       | Process                  | `(ctx, sess, userMessage string, opts ...ProcessOption) (reply string, stats RunStats, err error)` | Append user message; build processConfig from opts; call processLoop(ctx, sess, config); return.                                                                                                                                                                                                             |
| **Agent** (agent)       | ProcessAfterUserAppended | `(ctx, sess *Session, opts ...ProcessOption) (reply string, stats RunStats, err error)`            | Validate last message is user; build processConfig from opts; call processLoop(ctx, sess, config); return.                                                                                                                                                                                                   |
| **Agent** (agent)       | processLoop              | `(ctx, sess, config processConfig) (reply string, stats RunStats, err error)`                      | Loop: build messages; if config.StreamSink != nil call caller.ChatWithToolsStream(..., config.StreamSink.OnDelta), else caller.ChatWithTools(...). On return: if toolCalls empty, append assistant message and return; else append assistant+tool_calls, execute tools, repeat. Session semantics unchanged. |
| **LLMCaller** (agent)   | ChatWithTools            | existing                                                                                           | Unchanged.                                                                                                                                                                                                                                                                                                   |
| **LLMCaller** (agent)   | ChatWithToolsStream      | `(ctx, messages, tools, onDelta func(string)) (content string, toolCalls []ToolCall, err error)`   | Implementers that don’t stream can implement by calling ChatWithTools and then `onDelta(content)` once.                                                                                                                                                                                                      |
| **(tui)**               | buildViewportContent     | `(sess, version, width, busy, carouselDots, streamingTail string) string`                          | When streamingTail non-empty, after session messages append "• " + streamingTail (wrapped); when busy and streamingTail empty, keep carousel.                                                                                                                                                                |
| **ViewportBlock** (tui) | RefreshAndGotoBottom     | `(sess, version, width, busy, carouselDots, streamingTail string)`                                 | Delegate to buildViewportContent with streamingTail; set content and GotoBottom.                                                                                                                                                                                                                             |
| **(cmd)**               | runPrintMode             | existing + sink                                                                                    | Build sink that writes each delta to os.Stdout and flushes; call `Process(ctx, sess, prompt, agent.WithStreamSink(sink))`; keep persist and title logic; do not print full reply again at end (already streamed).                                                                                            |


**ProcessOption and WithStreamSink**

- `type ProcessOption func(*processConfig)`.
- `processConfig` (internal struct in agent): `streamSink StreamSink`.
- `func WithStreamSink(sink StreamSink) ProcessOption` — sets config.streamSink.

**TUI: delivering deltas from agent goroutine to Update**

- Agent runs inside a `tea.Cmd` in a separate goroutine. The stream sink’s `OnDelta` runs in that goroutine and must get deltas into the TUI’s Update. Use a single channel of type `chan tea.Msg`: the sink sends `streamDeltaMsg{Delta: d}` for each delta; when `ProcessAfterUserAppended` returns, the runner sends `agentDoneMsg{Reply, Err}` and closes the channel.
- Flow: On submit, create `ch := make(chan tea.Msg)`, build a `StreamSink` that does `ch <- streamDeltaMsg{Delta: d}` in `OnDelta`, start a goroutine that runs `ProcessAfterUserAppended(ctx, sess, WithStreamSink(sink))` then `ch <- agentDoneMsg{...}; close(ch)`. The Cmd returned is `tea.Cmd(func() tea.Msg { return <-ch })`. So the first message received might be a streamDeltaMsg or agentDoneMsg. When Update receives streamDeltaMsg, append to Model.streamingBuffer, refresh viewport with the new tail, and return another Cmd that reads from the same channel: `tea.Cmd(func() tea.Msg { return <-ch })`. When Update receives agentDoneMsg, handle as today (append to session, persist, title), clear streamingBuffer, and do not schedule another read. Problem: after the first Cmd returns, we need to keep reading from `ch`; we must store the channel on the Model so that each time we get a streamDeltaMsg we can return a Cmd that reads again from the same channel. So Model holds `streamChannel chan tea.Msg` (set when we start the agent run). The Cmd we return is always `tea.Cmd(func() tea.Msg { return <- m.streamCh })` when we have an active stream. So: when we submit, we create ch, start goroutine (agent + sink that writes to ch, then send agentDoneMsg and close ch), and we need to “seed” the first read. So we return tea.Batch(tea.Tick(carousel), tea.Cmd(func() tea.Msg { return <-ch })). Then when we get a msg, if it’s streamDeltaMsg we update buffer and return tea.Cmd(func() tea.Msg { return <-ch }); if it’s agentDoneMsg we handle and don’t schedule another read. We must store ch on the model when we start so that the Cmd we return can read from it (the closure captures ch). So we set m.streamChannel = ch when we create it, then return tea.Cmd(func() tea.Msg { return <- m.streamCh }). When we receive agentDoneMsg we set m.streamCh = nil. So the design is: on submit, m.streamCh = make(chan tea.Msg); start goroutine: run ProcessAfterUserAppended(..., sink that does m.streamCh <- streamDeltaMsg{Delta}); ch <- agentDoneMsg{...}; close(m.streamCh); return tea.Batch(tea.Tick(...), tea.Cmd(func() tea.Msg { return <- m.streamCh })). When we handle streamDeltaMsg we append and refresh and return tea.Cmd(func() tea.Msg { return <- m.streamCh }). When we handle agentDoneMsg we set m.streamCh = nil and do existing logic.
- runAgentAfterUserAppended changes: it receives (opts TUIOpts) and now also a channel (or the model creates the channel and passes a sink that uses it). So the signature could be: runAgentAfterUserAppended(opts TUIOpts, streamChannel chan tea.Msg) tea.Msg — no, the Cmd can’t receive the channel as argument if we create it in Update. So we create the channel in Update when we submit, set m.streamChannel = ch, build a sink that sends to ch, start a goroutine that runs the agent with that sink and then sends agentDoneMsg and closes ch, and the Cmd we return is the first read from ch. So the function that runs in the Cmd is: create ch, create sink, start goroutine, return <-ch (first message). But then we lose the reference to ch for subsequent reads. So the Cmd must be created in a place where we have access to the model to set m.streamChannel. So in Update, on submit: ch := make(chan tea.Msg); m.streamChannel = ch; sink := &streamSinkToChannel{channel: ch}; go func() { reply, _, err := opts.Agent.ProcessAfterUserAppended(ctx, opts.Session, agent.WithStreamSink(sink)); ch <- agentDoneMsg{Reply: reply, Err: err}; close(ch) }(); return m, tea.Batch(tea.Tick(...), tea.Cmd(func() tea.Msg { return <-ch })). So we don’t need runAgentAfterUserAppended to change signature; we inline the logic in Update: create channel, set m.streamCh, create sink, start goroutine, return Cmd that reads from channel. So runAgentAfterUserAppended could be replaced by a function that takes the channel and returns the Cmd and starts the goroutine. E.g. runAgentWithStream(opts, ch) that starts the goroutine and returns tea.Cmd(func() tea.Msg { return <-ch }). So we have runAgentWithStream(opts TUIOpts, ch chan tea.Msg) tea.Cmd. It starts a goroutine that: ProcessAfterUserAppended(..., sink writing to ch); ch <- agentDoneMsg{...}; close(ch). Returns tea.Cmd(func() tea.Msg { return <-ch }). So in Update on submit we do: ch := make(chan tea.Msg); m.streamCh = ch; return m, tea.Batch(tea.Tick(...), runAgentWithStream(m.opts, ch)). Then when we get streamDeltaMsg we return tea.Cmd(func() tea.Msg { return <- m.streamCh }). When we get agentDoneMsg we set m.streamCh = nil. Good.
- Summary for design doc: On submit, create channel and store on model, create sink that sends streamDeltaMsg to channel, start goroutine that runs ProcessAfterUserAppended with sink then sends agentDoneMsg and closes channel; return Cmd that reads one message from channel. On streamDeltaMsg: append to streamingBuffer, refresh viewport with tail, return Cmd that reads next from channel. On agentDoneMsg: clear streamChannel and streamingBuffer, append full message to session, refresh, persist, title as today.

## How they work together

**Data/control flow**

1. **Print mode**: runPrintMode builds an implementation of StreamSink that writes each delta to os.Stdout (and flushes). It calls Process(ctx, sess, prompt, WithStreamSink(sink)). Agent’s processLoop uses ChatWithToolsStream and passes sink.OnDelta; LLM client streams and invokes onDelta for each content delta; at end returns full content and tool_calls. Agent appends full message to session and returns; runPrintMode persists and generates title as today; does not print reply again.
2. **TUI**: On submit, model creates channel, stores it, builds sink that sends streamDeltaMsg on channel, starts goroutine that runs ProcessAfterUserAppended(..., WithStreamSink(sink)) then sends agentDoneMsg and closes channel. Model returns Cmd that reads one msg from channel. When streamDeltaMsg arrives, append to streamingBuffer, call RefreshAndGotoBottom(..., streamingBuffer), return Cmd that reads next. When agentDoneMsg arrives, clear streamingBuffer and streamChannel, append full assistant message to session (already done inside ProcessAfterUserAppended), refresh viewport without tail, persist, trigger title generation.
3. **Agent loop**: processLoop builds messages; if config.StreamSink != nil, calls caller.ChatWithToolsStream(ctx, messages, toolDefs, config.StreamSink.OnDelta); else ChatWithTools(...). Rest of loop unchanged: append assistant message, if toolCalls run tools and repeat.

**Dependencies**

- internal/agent depends on internal/llm (Message, ToolDef, ToolCall). Agent does not depend on internal/tui or internal/cmd.
- internal/cmd depends on internal/agent (Process, WithStreamSink) and provides a StreamSink that writes to os.Stdout.
- internal/tui depends on internal/agent (ProcessAfterUserAppended, WithStreamSink) and provides a StreamSink that sends to a channel; model reads from channel and updates streaming buffer and viewport.

**Key data structures**

- **StreamSink** (agent): interface; implementations in cmd (stdout writer) and tui (channel sender).
- **streamDeltaMsg** (tui): Delta string; received when a content chunk arrives.
- **streamChannel** (tui, on Model): channel for receiving streamDeltaMsg and agentDoneMsg; set when agent run starts, cleared when agentDoneMsg is handled.
- **streamingBuffer** (tui, on Model): string accumulated for the current turn’s assistant content; shown as streaming tail in viewport; cleared on agentDoneMsg.

## Changes for review

- **New** (internal/llm): `ChatWithToolsStream(ctx, messages, tools, onDelta func(string)) (content string, toolCalls []ToolCall, err error)` on Client; use go-openai CreateChatCompletionStream, map deltas to onDelta, accumulate content and tool_calls.
- **New** (internal/agent): `StreamSink` interface (`OnDelta(delta string)`); `processConfig` struct with streamSink field; `ProcessOption` and `WithStreamSink(sink)`; `processLoop(ctx, sess, config)`; Process and ProcessAfterUserAppended accept `opts ...ProcessOption` and pass config to processLoop; LLMCaller interface extended with `ChatWithToolsStream(...)`.
- **Modified** (internal/agent): mock LLMCaller in tests implements ChatWithToolsStream (e.g. call ChatWithTools then onDelta(content) once); add test that uses streaming and asserts sink receives deltas and session has full message.
- **Modified** (internal/cmd/print.go): build StreamSink that writes to os.Stdout with flush; call Process(..., WithStreamSink(sink)); remove final fmt.Println(reply) (content already streamed).
- **New** (internal/tui): `streamDeltaMsg` struct; `streamSinkToChannel` or similar that implements StreamSink and sends streamDeltaMsg to a channel; `runAgentWithStream(opts, channel) tea.Cmd` that starts goroutine (ProcessAfterUserAppended with sink, then send agentDoneMsg and close channel) and returns Cmd that reads one msg from channel.
- **Modified** (internal/tui/model.go): Model.streamingBuffer string; Model.streamChannel chan tea.Msg; on submit create channel, set streamChannel, call runAgentWithStream(opts, channel) in Batch with Tick; on streamDeltaMsg append to streamingBuffer, RefreshAndGotoBottom(..., streamingBuffer), return Cmd(<-streamChannel); on agentDoneMsg clear streamChannel and streamingBuffer, then existing handleAgentDone logic; RefreshAndGotoBottom calls updated signature with streamingTail.
- **Modified** (internal/tui/format.go): `buildViewportContent(..., streamingTail string)`; when streamingTail non-empty append as assistant line (same style as "• " + content), wrapped; when busy and no tail, keep carousel.
- **Modified** (internal/tui/viewport_block.go): `RefreshAndGotoBottom(..., streamingTail string)`; pass streamingTail to buildViewportContent.
- **Modified** (internal/tui): All call sites of RefreshAndGotoBottom and buildViewportContent pass streamingTail (empty string when not streaming or when showing final session).

