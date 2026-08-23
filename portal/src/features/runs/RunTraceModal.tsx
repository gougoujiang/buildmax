import { useEffect, useState } from "react"
import { BaseModal } from "@buildmax/gui"
import type {
  ApiRunProvenance,
  ApiTaskRunLLMCall,
  ApiTaskRunTrace,
  ApiTraceBoundary,
  ApiTraceToolCall,
} from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { getTaskRunProvenance, getTaskRunTrace, listTaskRunLLMCalls } from "./api"
import { describeOrigin, inputMatchesMessage } from "./origin"
import { callElapsed, describeSpend, summarizeSpend } from "./spend"
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

function SpendCallRow({ call }: { call: ApiTaskRunLLMCall }) {
  const failed = call.status === "FAILED" || call.status === "CANCELED"
  const tokens =
    typeof call.total_tokens === "number"
      ? `${call.total_tokens.toLocaleString()} tokens`
      : // An unreported count is not a free call, so it says so rather than
        // showing a zero the provider never sent.
        "usage not reported"
  return (
    <li className={failed ? "run-trace__call run-trace__call--failed" : "run-trace__call"}>
      <span className="run-trace__call-alias">{call.alias || "—"}</span>
      <span className="run-trace__call-tokens">{tokens}</span>
      {failed ? (
        <span className="run-trace__call-failed">
          {call.status.toLowerCase()}
          {call.error_class ? ` · ${call.error_class}` : ""}
        </span>
      ) : (
        <span className="run-trace__call-duration">{callElapsed(call)}</span>
      )}
      {typeof call.attempts === "number" && call.attempts > 1 ? (
        <span className="run-trace__call-attempts">{call.attempts} attempts</span>
      ) : null}
    </li>
  )
}

/**
 * What the deployment was asked to serve for this run, and on which approved
 * alias.
 *
 * This is a different record from the trace above it. The trace is what the
 * agent did, written by the run itself; this is the governance ledger, written
 * by the server as it served each call — the same rows a team's quota is
 * computed from. When the two disagree about how many calls a run made, that
 * gap is the point: it means the run reached a provider the server never saw.
 */
function SpendSection({
  calls,
  error,
  trace,
}: {
  calls: ApiTaskRunLLMCall[]
  error: string | null
  trace: ApiTaskRunTrace | null
}) {
  const note = describeSpend({ calls, error, trace })
  const summary = summarizeSpend(calls)
  return (
    <section className="run-trace__section">
      <h3 className="run-trace__heading">Managed model calls</h3>
      {note ? (
        <p className="run-trace__spend-note" role={error ? "alert" : undefined}>
          {note}
        </p>
      ) : (
        <>
          <dl className="run-trace__stats">
            <div>
              <dt>Accounted calls</dt>
              <dd>{summary.calls}</dd>
            </div>
            <div>
              <dt>Accounted tokens</dt>
              <dd>
                {summary.totalTokens.toLocaleString()}
                {summary.unreported > 0 ? (
                  <span className="run-trace__unreported">
                    {" "}
                    · {summary.unreported} call{summary.unreported === 1 ? "" : "s"} unreported
                  </span>
                ) : null}
              </dd>
            </div>
            {summary.failed > 0 ? (
              <div>
                <dt>Failed calls</dt>
                <dd>{summary.failed}</dd>
              </div>
            ) : null}
            {summary.inFlight > 0 ? (
              <div>
                <dt>Unfinished calls</dt>
                <dd>{summary.inFlight}</dd>
              </div>
            ) : null}
            {summary.retried > 0 ? (
              <div>
                <dt>Retries</dt>
                <dd>{summary.retried}</dd>
              </div>
            ) : null}
          </dl>

          {/* Named even when there is one, because which approved alias a run
              was allowed to spend on is the governance question. */}
          <ul className="run-trace__aliases">
            {summary.byAlias.map((entry) => (
              <li key={entry.alias}>
                <span className="run-trace__call-alias">{entry.alias}</span>
                <span className="run-trace__call-tokens">
                  {entry.calls} call{entry.calls === 1 ? "" : "s"} ·{" "}
                  {entry.totalTokens.toLocaleString()} tokens
                </span>
              </li>
            ))}
          </ul>

          <ul className="run-trace__calls">
            {calls.map((call) => (
              <SpendCallRow key={call.id} call={call} />
            ))}
          </ul>
        </>
      )}
    </section>
  )
}

/**
 * Where the run came from, and what was actually asked for.
 *
 * This sits above the trace because it is the question that comes first and the
 * one that survives every absence below it: a run that failed before an agent
 * started wrote no trace and still came from somewhere.
 *
 * The message and the instruction are shown together on purpose. The run input
 * is what Tier 1 decided to send a worker; the quote is what the person said. A
 * constraint present in one and missing from the other is exactly what a reader
 * is here to find, and it cannot be seen unless both are in front of them.
 */
function OriginSection({
  provenance,
  error,
}: {
  provenance: ApiRunProvenance | null
  error: string | null
}) {
  if (!provenance) {
    return (
      <section className="run-trace__section">
        <h3 className="run-trace__heading">Origin</h3>
        <p className="run-trace__spend-note" role={error ? "alert" : undefined}>
          {error ?? "Where this run came from was not recorded."}
        </p>
      </section>
    )
  }
  const origin = describeOrigin(provenance)
  const said = provenance.source_message
  const verbatim = inputMatchesMessage(provenance)
  return (
    <section className="run-trace__section">
      <h3 className="run-trace__heading">Origin</h3>
      <p className="run-trace__origin-text">{origin.text}</p>
      {said ? (
        <>
          <span className="run-trace__origin-label">Asked for as</span>
          <pre className="run-trace__quote">{said.content}</pre>
          {said.truncated ? (
            <p className="run-trace__truncated">
              Quoted to the first part of the message; the conversation has the rest.
            </p>
          ) : null}
        </>
      ) : (
        <p className="run-trace__spend-note">
          {origin.quote === "none-expected"
            ? origin.isRepeat
              ? "Nothing was said for this run — it repeats an earlier one with the same input."
              : "No message asked for this run; a runtime dispatched it."
            : "No message is recorded for this run, so what was asked for cannot be compared."}
        </p>
      )}
      <span className="run-trace__origin-label">Sent to the worker</span>
      <pre className="run-trace__quote">{provenance.input}</pre>
      {said && !said.truncated ? (
        <p className="run-trace__truncated">
          {verbatim
            ? "The request was passed through unchanged."
            : "The instruction was rewritten from the message above."}
        </p>
      ) : null}
    </section>
  )
}

/**
 * RunTraceModal answers where a run came from, what it used, touched, spent,
 * why it ended, and what confined it.
 */
export function RunTraceModal({ open, teamId, token, taskRunId, onClose }: RunTraceModalProps) {
  const [trace, setTrace] = useState<ApiTaskRunTrace | null>(null)
  const [provenance, setProvenance] = useState<ApiRunProvenance | null>(null)
  const [provenanceError, setProvenanceError] = useState<string | null>(null)
  const [calls, setCalls] = useState<ApiTaskRunLLMCall[]>([])
  const [callsError, setCallsError] = useState<string | null>(null)
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
    setCalls([])
    setCallsError(null)
    setProvenance(null)
    setProvenanceError(null)

    // The two records are fetched together and fail apart. A run whose trace
    // expired from storage still has a ledger, and a deployment that accounts
    // no managed calls still has a trace — neither absence may hide the other.
    const traceRequest = getTaskRunTrace(teamId, taskRunId, token)
      .then((result) => {
        if (!cancelled) setTrace(result)
      })
      .catch((err) => {
        // The server explains a missing trace precisely — never recorded, or
        // gone from storage. Those mean different things to an operator, so
        // pass its message through instead of substituting a generic failure.
        if (!cancelled) setError(getErrorMessage(err, "Failed to load this run's trace"))
      })
    const callsRequest = listTaskRunLLMCalls(teamId, taskRunId, token)
      .then((result) => {
        if (!cancelled) setCalls(result)
      })
      .catch((err) => {
        if (!cancelled) {
          setCallsError(getErrorMessage(err, "Failed to load this run's model calls"))
        }
      })

    const provenanceRequest = getTaskRunProvenance(teamId, taskRunId, token)
      .then((result) => {
        if (!cancelled) setProvenance(result)
      })
      .catch((err) => {
        if (!cancelled) {
          setProvenanceError(getErrorMessage(err, "Failed to load where this run came from"))
        }
      })

    void Promise.all([traceRequest, callsRequest, provenanceRequest]).finally(() => {
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
        ) : (
          <>
            {/* First and unconditional: a run that wrote no trace still came
                from somewhere, and that is the question a reader opens with. */}
            <OriginSection provenance={provenance} error={provenanceError} />
            {error ? (
              <p className="modal__error" role="alert">{error}</p>
            ) : trace ? (
              <TraceBody trace={trace} />
            ) : null}
            {/* Shown even when the trace could not be read: what a run spent is
                accounted server-side and survives a trace that did not. */}
            <SpendSection calls={calls} error={callsError} trace={trace} />
          </>
        )}
      </div>
      <div className="modal__actions">
        <button type="button" className="modal__btn modal__btn--secondary" onClick={onClose}>
          Close
        </button>
      </div>
    </BaseModal>
  )
}
