import { describe, expect, it } from "vitest"
import type { ApiRunProvenance } from "../../lib/api/types"
import { describeOrigin, inputMatchesMessage } from "./origin"

function provenance(over: Partial<ApiRunProvenance> = {}): ApiRunProvenance {
  return {
    task_run_id: "tr_1",
    task_id: "tk_1",
    status: "SUCCEEDED",
    input: "investigate the flaky test",
    created_at: 1000,
    ...over,
  }
}

describe("describeOrigin", () => {
  it("names the path that put the run in flight", () => {
    expect(describeOrigin(provenance({ trigger_source: "workflow_step" })).text).toContain(
      "workflow step"
    )
    expect(describeOrigin(provenance({ trigger_source: "portal_conversation" })).text).toContain(
      "conversation"
    )
  })

  it("says an unrecorded path is unrecorded rather than inventing one", () => {
    expect(describeOrigin(provenance()).text).toContain("unrecorded")
  })

  it("separates a missing quote that is expected from one that is a gap", () => {
    // A retry carries no new instruction, so nothing was said.
    expect(describeOrigin(provenance({ trigger_source: "task_retry" })).quote).toBe("none-expected")
    // A conversation run should name a message. Not having one is a gap.
    expect(describeOrigin(provenance({ trigger_source: "portal_conversation" })).quote).toBe(
      "none-recorded"
    )
  })

  it("marks a run that repeats another, however it was labelled", () => {
    expect(describeOrigin(provenance({ trigger_source: "task_retry" })).isRepeat).toBe(true)
    expect(describeOrigin(provenance({ retry_of_task_run_id: "tr_0" })).isRepeat).toBe(true)
    expect(describeOrigin(provenance({ trigger_source: "portal_conversation" })).isRepeat).toBe(
      false
    )
  })

  it("says when a runtime rather than a person started the run", () => {
    expect(
      describeOrigin(provenance({ trigger_source: "workflow_step", created_by_type: "system" })).text
    ).toContain("not a person")
    expect(
      describeOrigin(provenance({ trigger_source: "webhook", created_by_type: "webhook" })).text
    ).toContain("external caller")
  })

  it("reports a quoted message as quoted", () => {
    const described = describeOrigin(
      provenance({
        trigger_source: "portal_conversation",
        source_message: { id: "cm_1", content: "look into it", truncated: false, created_at: 1 },
      })
    )
    expect(described.quote).toBe("quoted")
  })
})

describe("inputMatchesMessage", () => {
  it("is true only when Tier 1 passed the request through unchanged", () => {
    expect(
      inputMatchesMessage(
        provenance({
          input: "look into it",
          source_message: { id: "cm_1", content: "look into it", truncated: false, created_at: 1 },
        })
      )
    ).toBe(true)
    expect(
      inputMatchesMessage(
        provenance({
          input: "investigate the flaky test",
          source_message: {
            id: "cm_1",
            content: "look into the flaky test, but leave CI alone",
            truncated: false,
            created_at: 1,
          },
        })
      )
    ).toBe(false)
  })

  it("never claims a match on a truncated quote, which is not the whole text", () => {
    expect(
      inputMatchesMessage(
        provenance({
          input: "same",
          source_message: { id: "cm_1", content: "same", truncated: true, created_at: 1 },
        })
      )
    ).toBe(false)
  })

  it("is false when there is nothing to compare", () => {
    expect(inputMatchesMessage(provenance())).toBe(false)
  })
})
