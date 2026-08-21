- Tool permissions are configurable. A `tools.permissions` block in
  `settings.yaml` sets any tool to `allow`, `ask`, or `deny`, keyed by tool name
  or by the target it dispatches to (`CallMcpTool:github/*`), most specific rule
  winning. `allow` turns off the category prompt but not the safety checks — a
  sensitive path and a risky shell command still prompt, and only `deny`
  outranks them. `ask` means a person must look, so on a surface with no person
  the call is refused rather than run. `buildmax tools status` prints every
  tool's classification, its resolved action, the layer that decided it, and any
  rule ignored for an unrecognised action; `/tools` in the TUI marks tools that
  do not simply run.
