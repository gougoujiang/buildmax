- MCP calls are gated by what the server says about the tool. `CallMcpTool` now
  reads the `readOnlyHint` annotation from `tools/list`, which BuildMax fetched
  and discarded until now: a tool the server advertises as read-only runs
  unprompted, and anything else asks. **This tightens autonomous surfaces**, the
  one behavior change there — a task run, print-mode run, or Portal conversation
  calling an MCP tool that is not advertised as read-only is now refused rather
  than run silently. A server that omits the annotation is treated the same as
  one that says `false`, because the protocol cannot distinguish them. The hint
  decides whether BuildMax asks; it never grants trust.
