import { describe, expect, it } from "vitest"
import type { ApiTaskRunTrace } from "../../lib/api/types"
import { describeBoundary, formatDuration, runElapsed } from "./summary"

// The sandbox is off by default on every surface today, so most runs report
// false. A viewer who cannot tell a confined run from an unconfined one has no
// reason to trust either, which is what these assertions protect.
describe("describeBoundary", () => {
  it("states plainly when nothing confined the run", () => {
    const got = describeBoundary({ sandboxed: false, backend: "none", sources: ["default:cli"] })
    expect(got.tone).toBe("open")
    expect(got.text).toContain("Not sandboxed")
    expect(got.sources).toBe("default:cli")
  })

  it("does not treat an unrecorded boundary as an unconfined one", () => {
    // Both are "not known to be sandboxed", but only one is a fact about the
    // deployment. Collapsing them would blame an old trace for a gap it never had.
    expect(describeBoundary(undefined).tone).toBe("unknown")
    expect(describeBoundary({ sandboxed: false }).tone).toBe("open")
  })

  it("reports the backend and mode that were in effect", () => {
    const got = describeBoundary({
      sandboxed: true,
      backend: "bwrap",
      mode: "auto_allow",
      sources: ["default:worker", "policy"],
    })
    expect(got.tone).toBe("sandboxed")
    expect(got.text).toContain("bwrap")
    expect(got.text).toContain("auto_allow")
    expect(got.sources).toBe("default:worker → policy")
  })

  it("surfaces a downgraded boundary rather than reporting plain success", () => {
    const got = describeBoundary({ sandboxed: true, backend: "bwrap", downgraded: true })
    expect(got.tone).toBe("sandboxed")
    expect(got.text).toContain("weaker than configured")
  })
})

describe("formatDuration", () => {
  it("keeps sub-second calls readable", () => {
    expect(formatDuration(12)).toBe("12 ms")
    expect(formatDuration(1500)).toBe("1.5 s")
  })

  it("shows a dash rather than a fake zero when there is no duration", () => {
    expect(formatDuration(undefined)).toBe("—")
    expect(formatDuration(0)).toBe("—")
  })
})

describe("runElapsed", () => {
  const base: ApiTaskRunTrace = {
    task_run_id: "r_1",
    llm_calls: 0,
    tool_calls: 0,
    compactions: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    complete: true,
  }

  it("measures between the first and last record", () => {
    expect(
      runElapsed({ ...base, started_at: "2026-08-16T08:14:02.000Z", ended_at: "2026-08-16T08:14:04.500Z" })
    ).toBe("2.5 s")
  })

  it("refuses to guess for a run that never finished", () => {
    // An unfinished run has no end. Inventing one would make a killed run look
    // like it completed quickly.
    expect(runElapsed({ ...base, started_at: "2026-08-16T08:14:02.000Z", complete: false })).toBe("—")
    expect(runElapsed({ ...base, started_at: "not a date", ended_at: "also not" })).toBe("—")
  })
})
