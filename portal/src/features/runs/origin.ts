import type { ApiRunProvenance } from "../../lib/api/types"

/** How a run's origin should read to someone asking why it exists. */
export interface OriginDescription {
  /** One sentence naming what put this run in flight. */
  text: string
  /**
   * Whether the run repeats an earlier one. A repeat carries no new
   * instruction, so "nothing was said" is expected rather than missing.
   */
  isRepeat: boolean
  /**
   * How to read the absence of a quoted message.
   *
   * quoted — the message is there to compare against the run input.
   * none-expected — this origin never has one.
   * none-recorded — this origin should have one and does not, because the run
   *   predates the record or its message could not be resolved.
   *
   * The last two are deliberately distinct: an old run with no attribution and
   * an origin that never had any look identical if they are folded together,
   * and only one of them is a gap.
   */
  quote: "quoted" | "none-expected" | "none-recorded"
}

const triggerText: Record<string, string> = {
  task_create: "Created with the task.",
  task_rerun: "Run again with new instructions.",
  task_retry: "A repeat of an earlier run, with the same input.",
  portal_conversation: "Asked for in a conversation.",
  portal_task_create: "Created from the Portal.",
  portal_task_rerun: "Run again from the Portal.",
  issue_agent_run: "Started by an issue's agent.",
  workflow_step: "Dispatched by a workflow step.",
  webhook: "Started by an inbound webhook.",
}

/** Origins where nobody typed anything, so no message can be quoted. */
const messagelessTriggers = new Set(["task_retry", "workflow_step", "issue_agent_run"])

export function describeOrigin(provenance: ApiRunProvenance): OriginDescription {
  const trigger = provenance.trigger_source ?? ""
  const isRepeat = trigger === "task_retry" || !!provenance.retry_of_task_run_id
  const parts: string[] = [triggerText[trigger] ?? "Started by an unrecorded path."]
  if (provenance.created_by_type === "system") {
    parts.push("The runtime started it, not a person.")
  } else if (provenance.created_by_type === "webhook") {
    parts.push("An external caller started it, not a person.")
  }
  let quote: OriginDescription["quote"] = "quoted"
  if (!provenance.source_message) {
    quote = messagelessTriggers.has(trigger) ? "none-expected" : "none-recorded"
  }
  return { text: parts.join(" "), isRepeat, quote }
}

/**
 * Whether the quoted message and the run's instruction are the same text.
 *
 * Worth saying out loud. Equal means Tier 1 passed the request through
 * verbatim; different is the normal case and the reason both are stored.
 */
export function inputMatchesMessage(provenance: ApiRunProvenance): boolean {
  const said = provenance.source_message
  if (!said || said.truncated) return false
  return said.content.trim() === provenance.input.trim()
}
