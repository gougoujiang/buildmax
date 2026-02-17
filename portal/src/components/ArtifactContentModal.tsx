import { useEffect, useState } from "react"
import Markdown from "react-markdown"
import { BaseModal } from "./BaseModal"
import { getArtifactItems, getArtifactContent } from "../lib/api"

interface ArtifactContentModalProps {
  open: boolean
  workspaceId: string
  artifactId: string
  token: string
  onClose: () => void
}

export function ArtifactContentModal({
  open,
  workspaceId,
  artifactId,
  token,
  onClose,
}: ArtifactContentModalProps) {
  const [items, setItems] = useState<{ relative_path: string }[]>([])
  const [itemsLoading, setItemsLoading] = useState(false)
  const [itemsError, setItemsError] = useState<string | null>(null)
  const [selectedPath, setSelectedPath] = useState<string | null>(null)
  const [content, setContent] = useState<string | null>(null)
  const [contentLoading, setContentLoading] = useState(false)
  const [contentError, setContentError] = useState<string | null>(null)

  useEffect(() => {
    if (!open || !workspaceId || !artifactId || !token) {
      setItems([])
      setItemsError(null)
      setSelectedPath(null)
      setContent(null)
      setContentError(null)
      return
    }
    let cancelled = false
    setItemsLoading(true)
    setItemsError(null)
    getArtifactItems(workspaceId, artifactId, token)
      .then((list) => {
        if (!cancelled) {
          setItems(list)
          if (list.length > 0) {
            setSelectedPath(list[0].relative_path)
          }
        }
      })
      .catch((err) => {
        if (!cancelled) setItemsError(err instanceof Error ? err.message : "Failed to load artifact files")
      })
      .finally(() => {
        if (!cancelled) setItemsLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [open, workspaceId, artifactId, token])

  useEffect(() => {
    if (!open || !selectedPath || !token) {
      setContent(null)
      setContentError(null)
      return
    }
    let cancelled = false
    setContentLoading(true)
    setContentError(null)
    getArtifactContent(workspaceId, artifactId, token, selectedPath)
      .then((text) => {
        if (!cancelled) setContent(text)
      })
      .catch((err) => {
        if (!cancelled) setContentError(err instanceof Error ? err.message : "Failed to load file content")
      })
      .finally(() => {
        if (!cancelled) setContentLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [open, workspaceId, artifactId, token, selectedPath])

  return (
    <BaseModal
      open={open}
      title="Artifact"
      titleId="artifact-modal-title"
      onClose={onClose}
    >
      <div className="modal__body">
        {itemsLoading && <p className="artifact-modal__loading">Loading files…</p>}
        {itemsError && (
          <p className="artifact-modal__error" role="alert">
            {itemsError}
          </p>
        )}
        {!itemsLoading && items.length > 0 && (
          <>
            <div className="artifact-modal__files">
              <h3 className="artifact-modal__files-heading">Files</h3>
              <ul className="artifact-modal__file-list">
                {items.map((item) => (
                  <li key={item.relative_path} className="artifact-modal__file-item">
                    <button
                      type="button"
                      className={`artifact-modal__file-link ${selectedPath === item.relative_path ? "artifact-modal__file-link--active" : ""}`}
                      onClick={() => setSelectedPath(item.relative_path)}
                    >
                      {item.relative_path}
                    </button>
                  </li>
                ))}
              </ul>
            </div>
            <div className="artifact-modal__content-wrap">
              {contentLoading && <p className="artifact-modal__loading">Loading content…</p>}
              {contentError && (
                <p className="artifact-modal__error" role="alert">
                  {contentError}
                </p>
              )}
              {content !== null && !contentLoading && (
                <div className="artifact-modal__content page-task__markdown">
                  <Markdown>{content}</Markdown>
                </div>
              )}
            </div>
          </>
        )}
        {!itemsLoading && !itemsError && items.length === 0 && (
          <p className="artifact-modal__empty">No files in this artifact.</p>
        )}
      </div>
    </BaseModal>
  )
}
