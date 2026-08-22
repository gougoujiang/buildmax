import { useEffect, useState } from "react"
import { BaseModal } from "@buildmax/gui"
import type { IssueOutput } from "../../lib/types"
import { getRunOutputContent } from "../runs/runOutputs"
import { artifactContentUrl } from "../artifacts"
import { downloadAuthenticated } from "../../lib/download"
import { getErrorMessage } from "../../lib/errorMessage"

interface OutputCardProps {
  output: IssueOutput
  token?: string | null
  onOpenFull: (output: IssueOutput) => void
  onOpenConversation?: (conversationId: string) => void
  onOpenRun?: (workflowRunId: string) => void
  onOpenTrace?: (taskRunId: string) => void
}

function formatTimestamp(seconds: number): string {
  return new Date(seconds * 1000).toLocaleString()
}

export function OutputCard({
  output,
  token,
  onOpenFull,
  onOpenConversation,
  onOpenRun,
  onOpenTrace,
}: OutputCardProps) {
  const { source } = output
  // An artifact is reached by its own id and has no preview text to show. It is
  // a file someone is meant to keep, so the action is to take it away.
  const isArtifact = !!output.artifactId
  return (
    <article className="issue-outputs__card">
      <header className="issue-outputs__card-head">
        <div>
          <h3 className="issue-outputs__card-title">{output.title}</h3>
          <div className="page-activity__meta">
            {output.filename ?? output.relativePath ? (
              <>
                <span>{output.filename ?? output.relativePath}</span>
                <span> · </span>
              </>
            ) : null}
            <span>{output.kind}</span>
            <span> · </span>
            <span>{formatTimestamp(output.createdAt)}</span>
          </div>
        </div>
      </header>
      {isArtifact ? (
        <p className="page-activity__meta">
          <code>{output.artifactId}</code>
        </p>
      ) : output.preview ? (
        <pre className="issue-outputs__preview">
          {output.preview}
          {output.previewTruncated ? "\n…" : ""}
        </pre>
      ) : (
        <p className="page-activity__empty">Preview unavailable.</p>
      )}
      <footer className="issue-outputs__card-actions">
        {isArtifact ? (
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => {
              if (!token || !output.artifactId) return
              void downloadAuthenticated(
                artifactContentUrl(output.artifactId),
                token,
                output.filename ?? output.title
              )
            }}
            disabled={!token}
          >
            Download
          </button>
        ) : (
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => onOpenFull(output)}
            disabled={!source.taskRunId}
          >
            Open full output
          </button>
        )}
        {source.conversationId && onOpenConversation ? (
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => onOpenConversation(source.conversationId!)}
          >
            Open conversation
          </button>
        ) : null}
        {source.workflowRunId && onOpenRun ? (
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => onOpenRun(source.workflowRunId!)}
          >
            Open run detail
          </button>
        ) : null}
        {source.taskRunId && onOpenTrace ? (
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => onOpenTrace(source.taskRunId!)}
          >
            Run details
          </button>
        ) : null}
      </footer>
    </article>
  )
}

interface OutputsListProps {
  outputs: IssueOutput[]
  token?: string | null
  onOpenFull: (output: IssueOutput) => void
  onOpenConversation?: (conversationId: string) => void
  onOpenRun?: (workflowRunId: string) => void
  onOpenTrace?: (taskRunId: string) => void
}

export function OutputsList({ outputs, token, onOpenFull, onOpenConversation, onOpenRun, onOpenTrace }: OutputsListProps) {
  if (outputs.length === 0) {
    return <p className="page-activity__empty">No results produced yet.</p>
  }
  return (
    <div className="issue-outputs__list">
      {outputs.map((o) => (
        <OutputCard
          key={o.id}
          output={o}
          token={token}
          onOpenFull={onOpenFull}
          onOpenConversation={onOpenConversation}
          onOpenRun={onOpenRun}
          onOpenTrace={onOpenTrace}
        />
      ))}
    </div>
  )
}

interface OutputViewerModalProps {
  open: boolean
  teamId: string | null
  token: string | null
  output: IssueOutput | null
  onClose: () => void
}

export function OutputViewerModal({ open, teamId, token, output, onClose }: OutputViewerModalProps) {
  const [content, setContent] = useState<string>("")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!open || !output || !teamId || !token || !output.source.taskRunId) {
      return
    }
    let cancelled = false
    setLoading(true)
    setError(null)
    setContent("")
    getRunOutputContent(teamId, output.source.taskRunId, token, output.relativePath)
      .then((text) => {
        if (!cancelled) setContent(text)
      })
      .catch((err) => {
        if (!cancelled) setError(getErrorMessage(err, "Failed to load output"))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [open, output, teamId, token])

  if (!output) return null

  return (
    <BaseModal
      open={open}
      title={output.title}
      titleId="issue-output-viewer-title"
      onClose={onClose}
      className="modal--large"
    >
      <div className="modal__body">
        <p className="modal__hint">
          {output.relativePath ?? output.kind}
          {output.source.taskRunId ? ` · ${output.source.taskRunId}` : ""}
        </p>
        {loading ? (
          <p className="page-activity__empty">Loading…</p>
        ) : error ? (
          <p className="modal__error" role="alert">{error}</p>
        ) : (
          <pre className="issue-outputs__full">{content}</pre>
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
