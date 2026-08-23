import { useEffect, useState } from "react"
import { BaseModal } from "@buildmax/gui"
import { getErrorMessage } from "../../../lib/errorMessage"
import { getRunOutputContent, listRunOutputFiles, type RunOutputFile } from "../../runs"

interface TaskFilesModalProps {
  open: boolean
  teamId: string | null
  token: string | null
  taskRunId: string | null
  onClose: () => void
}

/**
 * The files one run left behind, listed and read in place.
 *
 * A run's output directory is addressed by run and relative path, so this reads
 * the list first and fetches a file only when someone asks for it: a run may
 * have written something large, and opening the card should not download it.
 */
export function TaskFilesModal({ open, teamId, token, taskRunId, onClose }: TaskFilesModalProps) {
  const [files, setFiles] = useState<RunOutputFile[]>([])
  const [selected, setSelected] = useState<string | null>(null)
  const [content, setContent] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!open || !teamId || !token || !taskRunId) return
    let cancelled = false
    setFiles([])
    setSelected(null)
    setContent("")
    setError(null)
    setLoading(true)
    listRunOutputFiles(teamId, taskRunId, token)
      .then((list) => {
        if (cancelled) return
        setFiles(list)
        setSelected(list[0]?.relative_path ?? null)
      })
      .catch((err) => {
        if (!cancelled) setError(getErrorMessage(err, "Failed to list files"))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [open, teamId, token, taskRunId])

  useEffect(() => {
    if (!open || !teamId || !token || !taskRunId || !selected) return
    let cancelled = false
    setContent("")
    setError(null)
    getRunOutputContent(teamId, taskRunId, token, selected)
      .then((text) => {
        if (!cancelled) setContent(text)
      })
      .catch((err) => {
        if (!cancelled) setError(getErrorMessage(err, "Failed to read file"))
      })
    return () => {
      cancelled = true
    }
  }, [open, teamId, token, taskRunId, selected])

  return (
    <BaseModal
      open={open}
      title="Run files"
      titleId="task-files-title"
      onClose={onClose}
      className="modal--large"
    >
      <div className="modal__body">
        {loading ? <p className="page-activity__meta">Loading files…</p> : null}
        {error ? <p className="modal__error">{error}</p> : null}
        {!loading && files.length === 0 && !error ? (
          <p className="page-activity__empty">This run stored no files.</p>
        ) : null}
        {files.length > 0 ? (
          <ul className="task-card__file-list">
            {files.map((file) => (
              <li key={file.relative_path}>
                <button
                  type="button"
                  className={
                    file.relative_path === selected
                      ? "page-activity__action-btn page-activity__action-btn--active"
                      : "page-activity__action-btn"
                  }
                  onClick={() => setSelected(file.relative_path)}
                >
                  {file.relative_path}
                </button>
              </li>
            ))}
          </ul>
        ) : null}
        {content ? <pre className="task-card__preview">{content}</pre> : null}
      </div>
    </BaseModal>
  )
}
