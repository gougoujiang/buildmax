import type { ApiUsage } from "../../lib/api/types"

/**
 * The share of a limit at which the server records a warning, mirrored here so
 * the Portal warns at the same point the trail does.
 *
 * The two are separate constants for a reason worth stating: the server's is
 * the one that decides what is recorded, and this one only decides what is
 * shown. Reading a number the server did not send would be worse — it would
 * make the display authoritative about a policy it does not hold — so this
 * stays a display threshold, and `internal/service/quota/alert.go` stays the
 * decision.
 */
export const QUOTA_WARN_THRESHOLD = 0.8

export interface QuotaPressure {
  /** near — approaching a limit. reached — the limit is spent. */
  tone: "near" | "reached"
  text: string
}

function shareOf(used: number, max?: number): number | null {
  if (max == null || max <= 0) return null
  return used / max
}

/**
 * How close a team is to its quota, or null when it is not close.
 *
 * A tier with no limits reports nothing rather than reporting comfort: an
 * unknown limit and a generous one look identical from here, and only one of
 * them means there is nothing to worry about.
 *
 * Runs and tokens are reported together when both are under pressure, because
 * a team that is at its run limit and its token limit has one problem, not two,
 * and reading two separate warnings invites fixing only the first.
 */
export function describeQuotaPressure(usage: ApiUsage | null): QuotaPressure | null {
  if (!usage) return null
  const runs = shareOf(usage.run_count, usage.max_runs_per_period)
  const tokens = shareOf(usage.total_tokens, usage.max_tokens_per_period)

  const reached: string[] = []
  const near: string[] = []
  if (runs != null && runs >= 1) reached.push("runs")
  else if (runs != null && runs >= QUOTA_WARN_THRESHOLD) near.push("runs")
  if (tokens != null && tokens >= 1) reached.push("tokens")
  else if (tokens != null && tokens >= QUOTA_WARN_THRESHOLD) near.push("tokens")

  const period = usage.period_days > 0 ? ` in this ${usage.period_days}-day window` : ""
  if (reached.length > 0) {
    return {
      tone: "reached",
      text: `This space has used its full ${reached.join(" and ")} quota${period}. New work is refused until usage falls out of the window or the tier changes.`,
    }
  }
  if (near.length > 0) {
    return {
      tone: "near",
      text: `This space has used most of its ${near.join(" and ")} quota${period}.`,
    }
  }
  return null
}
