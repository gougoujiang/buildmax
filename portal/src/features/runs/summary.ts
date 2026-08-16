import type { ApiTaskRunTrace, ApiTraceBoundary } from "../../lib/api/types"

/** How a boundary should read to someone deciding whether to trust a run. */
export interface BoundaryDescription {
  /**
   * open — nothing confined the run.
   * sandboxed — a sandbox was in effect.
   * unknown — the trace predates boundary recording.
   *
   * "unknown" is deliberately not folded into "open": a run nobody checked and
   * a run checked and found unconfined are different facts, and only the second
   * is something the deployment chose.
   */
  tone: "open" | "sandboxed" | "unknown"
  text: string
  /** The layer chain that decided it, already joined for display. */
  sources: string | null
}

export function describeBoundary(boundary?: ApiTraceBoundary): BoundaryDescription {
  if (!boundary) {
    return {
      tone: "unknown",
      text: "This trace predates boundary recording, so what confined the run is unknown.",
      sources: null,
    }
  }
  const sources = boundary.sources?.length ? boundary.sources.join(" → ") : null
  if (!boundary.sandboxed) {
    return {
      tone: "open",
      text: "Not sandboxed — nothing confined this run's shell commands.",
      sources,
    }
  }
  const parts = ["Ran sandboxed"]
  if (boundary.backend) parts.push(`via ${boundary.backend}`)
  let text = parts.join(" ")
  if (boundary.mode) text += ` (${boundary.mode})`
  text += "."
  if (boundary.downgraded) text += " The boundary resolved weaker than configured."
  return { tone: "sandboxed", text, sources }
}

export function formatDuration(ms?: number): string {
  if (!ms || ms < 0) return "—"
  if (ms < 1000) return `${ms} ms`
  return `${(ms / 1000).toFixed(1)} s`
}

/** Wall-clock time between the run's first and last record. */
export function runElapsed(trace: ApiTaskRunTrace): string {
  if (!trace.started_at || !trace.ended_at) return "—"
  const start = Date.parse(trace.started_at)
  const end = Date.parse(trace.ended_at)
  if (Number.isNaN(start) || Number.isNaN(end)) return "—"
  return formatDuration(end - start)
}
