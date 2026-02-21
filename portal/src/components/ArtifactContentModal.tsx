import { useEffect, useState } from "react"
import { cn } from "../lib/cn"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { BaseModal } from "./BaseModal"
import { getArtifactItems, getArtifactContent } from "../lib/api"
import { useFetch } from "../hooks/useFetch"

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
  const [selectedPath, setSelectedPath] = useState<string | null>(null)

  const itemsEnabled = !!(open && workspaceId && artifactId && token)
  const {
    data: itemsData,
    loading: itemsLoading,
    error: itemsError,
  } = useFetch(
    () => getArtifactItems(workspaceId, artifactId, token),
    [open, workspaceId, artifactId, token],
    {
      enabled: itemsEnabled,
      errorMessage: (e) => (e instanceof Error ? e.message : "Failed to load artifact files"),
    }
  )
  const items = itemsData ?? []

  useEffect(() => {
    if (!itemsEnabled) {
      setSelectedPath(null)
      return
    }
    if (items.length > 0) {
      setSelectedPath((prev) =>
        prev && items.some((i) => i.relative_path === prev) ? prev : items[0].relative_path
      )
    }
  }, [itemsEnabled, items])

  const contentEnabled = !!(open && selectedPath && token)
  const {
    data: content,
    loading: contentLoading,
    error: contentError,
  } = useFetch(
    () => getArtifactContent(workspaceId, artifactId, token, selectedPath!),
    [open, workspaceId, artifactId, token, selectedPath],
    {
      enabled: contentEnabled,
      errorMessage: (e) => (e instanceof Error ? e.message : "Failed to load file content"),
    }
  )

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
                      className={cn("artifact-modal__file-link", selectedPath === item.relative_path && "artifact-modal__file-link--active")}
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
              {content != null && !contentLoading && (
                <div className="artifact-modal__content page-chat__markdown">
                  <Markdown remarkPlugins={[remarkGfm]}>{content}</Markdown>
                </div>
              )}
            </div>
          </>
        )}
        {itemsEnabled && !itemsLoading && !itemsError && items.length === 0 && (
          <p className="artifact-modal__empty">No files in this artifact.</p>
        )}
      </div>
    </BaseModal>
  )
}
