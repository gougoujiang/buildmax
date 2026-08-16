import { useEffect, useState } from "react"
import { BaseModal } from "@buildmax/gui"
import type { ApiTaskRunTrace, ApiTraceBoundary, ApiTraceToolCall } from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { getTaskRunTrace } from "./api"
import { describeBoundary, formatDuration, runElapsed } from "./summary"

interface RunTraceModalProps {
  open: boolean
  teamId: string | null
  token: string | null
  taskRunId: string | null
  onClose: () => void
}

/**
 * Boundary is stated first and in words, not as a checkmark.
 *
 * The sandbox is off by default on every surface today, so most runs report
 * false. That is exactly why it is shown plainly: a viewer who cannot tell a
 * confined run from an unconfined one has no reason to trust either.
 */
function BoundaryLine({ boundary }: { boundary?: ApiTraceBoundary }) {
  const described = describeBoundary(boundary)
  return (
    <p className={`run-trace__boundary run-trace__boundary--${described.tone}`}>
      {described.text}
      {described.sources ? (
        <span className="run-trace__sources">Decided by: {described.sources}</span>
      ) : null}
    </p>
  )
}

function ToolRow({ tool }: { tool: ApiTraceToolCall }) {
  return (
    <li className={tool.denied ? "run-trace__tool run-trace__tool--denied" : "run-trace__tool"}>
      <span className="run-trace__tool-name">{tool.name}</span>
      {tool.path ? <span className="run-trace__tool-path">{tool.path}</span> : null}
      {tool.denied ? (
        <span className="run-trace__tool-denied">
          denied{tool.deny_reason ? ` · ${tool.deny_reason}` : ""}
        </span>
      ) : (
        <span className="run-trace__tool-duration">{formatDuration(tool.duration_ms)}</span>
      )}
    </li>
  )
}

function TraceBody({ trace }: { trace: ApiTaskRunTrace }) {
  const tools = trace.tools ?? []
  const files = trace.files_changed ?? []
  return (
    <>
      <BoundaryLine boundary={trace.boundary} />

      {/* An unfinished run is not a successful one. Say so before the numbers,
          which otherwise read as a complete accounting. */}
      {!trace.complete ? (
        <p className="run-trace__incomplete" role="status">
          This run wrote no terminal record — it was killed, or its trace was cut short.
          The figures below cover only what was written.
        </p>
      ) : null}

      {trace.error ? (
        <div className="run-trace__error" role="alert">
          <span className="run-trace__label">Failed</span>
          <pre className="run-trace__error-text">{trace.error}</pre>
        </div>
      ) : null}

      <dl className="run-trace__stats">
        <div>
          <dt>Model</dt>
          <dd>{trace.model || "—"}</dd>
        </div>
        <div>
          <dt>Duration</dt>
          <dd>{runElapsed(trace)}</dd>
        </div>
        <div>
          <dt>Model calls</dt>
          <dd>{trace.llm_calls}</dd>
        </div>
        <div>
          <dt>Tool calls</dt>
          <dd>{trace.tool_calls}</dd>
        </div>
        <div>
          <dt>Tokens</dt>
          <dd>
            {trace.prompt_tokens.toLocaleString()} in · {trace.completion_tokens.toLocaleString()} out
          </dd>
        </div>
        {trace.compactions > 0 ? (
          <div>
            <dt>Compactions</dt>
            <dd>{trace.compactions}</dd>
          </div>
        ) : null}
      </dl>

      {files.length > 0 ? (
        <section className="run-trace__section">
          <h3 className="run-trace__heading">Files changed</h3>
          <ul className="run-trace__files">
            {files.map((path) => (
              <li key={path}>{path}</li>
            ))}
          </ul>
        </section>
      ) : null}

      {tools.length > 0 ? (
        <section className="run-trace__section">
          <h3 className="run-trace__heading">Tool calls</h3>
          <ul className="run-trace__tools">
            {tools.map((tool, i) => (
              <ToolRow key={`${tool.name}-${i}`} tool={tool} />
            ))}
          </ul>
          {/* A short list must never be mistaken for a short run. */}
          {trace.tools_truncated ? (
            <p className="run-trace__truncated">
              Showing the first {tools.length} of {trace.tool_calls} calls.
            </p>
          ) : null}
        </section>
      ) : null}
    </>
  )
}

/**
 * RunTraceModal answers what a run used, touched, spent, why it ended, and what
 * confined it.
 */
export function RunTraceModal({ open, teamId, token, taskRunId, onClose }: RunTraceModalProps) {
  const [trace, setTrace] = useState<ApiTaskRunTrace | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!open || !teamId || !token || !taskRunId) {
      return
    }
    let cancelled = false
    setLoading(true)
    setError(null)
    setTrace(null)
    getTaskRunTrace(teamId, taskRunId, token)
      .then((result) => {
        if (!cancelled) setTrace(result)
      })
      .catch((err) => {
        // The server explains a missing trace precisely — never recorded, or
        // gone from storage. Those mean different things to an operator, so
        // pass its message through instead of substituting a generic failure.
        if (!cancelled) setError(getErrorMessage(err, "Failed to load this run's trace"))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [open, teamId, token, taskRunId])

  return (
    <BaseModal
      open={open}
      title="Run details"
      titleId="run-trace-title"
      onClose={onClose}
      className="modal--large"
    >
      <div className="modal__body">
        <p className="modal__hint">{taskRunId ?? ""}</p>
        {loading ? (
          <p className="page-activity__empty">Loading…</p>
        ) : error ? (
          <p className="modal__error" role="alert">{error}</p>
        ) : trace ? (
          <TraceBody trace={trace} />
        ) : null}
      </div>
      <div className="modal__actions">
        <button type="button" className="modal__btn modal__btn--secondary" onClick={onClose}>
          Close
        </button>
      </div>
    </BaseModal>
  )
}
