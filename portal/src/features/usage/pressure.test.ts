import { describe, expect, it } from "vitest"
import type { ApiUsage } from "../../lib/api/types"
import { describeQuotaPressure } from "./pressure"

function usage(overrides: Partial<ApiUsage> = {}): ApiUsage {
  return {
    run_count: 0,
    total_tokens: 0,
    tier: "free_trial",
    period_days: 30,
    ...overrides,
  }
}

describe("describeQuotaPressure", () => {
  it("says nothing when there is nothing to say", () => {
    expect(
      describeQuotaPressure(usage({ run_count: 2, max_runs_per_period: 100 }))
    ).toBeNull()
  })

  it("treats a tier with no limits as unknown rather than as comfortable", () => {
    // An unknown limit and a generous one look identical from here, and only
    // one of them means there is nothing to worry about.
    expect(describeQuotaPressure(usage({ run_count: 10_000 }))).toBeNull()
    expect(describeQuotaPressure(null)).toBeNull()
  })

  it("warns before the refusals start", () => {
    const got = describeQuotaPressure(usage({ run_count: 8, max_runs_per_period: 10 }))
    expect(got?.tone).toBe("near")
    expect(got?.text).toContain("runs")
  })

  it("separates being at the limit from being near it", () => {
    const got = describeQuotaPressure(usage({ run_count: 10, max_runs_per_period: 10 }))
    expect(got?.tone).toBe("reached")
    expect(got?.text).toContain("refused")
  })

  it("reports both limits together rather than only the first", () => {
    // A space at both limits has one problem, and two separate warnings invite
    // fixing only one of them.
    const got = describeQuotaPressure(
      usage({
        run_count: 9,
        max_runs_per_period: 10,
        total_tokens: 90_000,
        max_tokens_per_period: 100_000,
      })
    )
    expect(got?.tone).toBe("near")
    expect(got?.text).toContain("runs and tokens")
  })

  it("leads with the limit that is spent when only one of them is", () => {
    const got = describeQuotaPressure(
      usage({
        run_count: 5,
        max_runs_per_period: 10,
        total_tokens: 100_000,
        max_tokens_per_period: 100_000,
      })
    )
    expect(got?.tone).toBe("reached")
    expect(got?.text).toContain("tokens")
    expect(got?.text).not.toContain("runs")
  })

  it("names the window, so a rolling count is not read as a lifetime one", () => {
    const got = describeQuotaPressure(
      usage({ run_count: 9, max_runs_per_period: 10, period_days: 30 })
    )
    expect(got?.text).toContain("30-day")
  })
})
