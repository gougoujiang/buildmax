import { useEffect, useState } from "react"
import type { ApiArtifact } from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { fetchArtifactPreview } from "./api"

interface ArtifactPreviewProps {
  artifact: ApiArtifact | null
  token: string | null
  onClose: () => void
}

/**
 * ArtifactPreview shows content the server marked safe to display.
 *
 * It renders text as text and images as images, and nothing else — no markup is
 * interpreted here. The server has already decided which types may be displayed
 * at all; this narrows further rather than widening, so a type that slipped
 * into the allowlist still cannot become a way to run something.
 */
export function ArtifactPreview({ artifact, token, onClose }: ArtifactPreviewProps) {
  const [text, setText] = useState<string | null>(null)
  const [objectUrl, setObjectUrl] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!artifact || !token) return
    let cancelled = false
    let created: string | null = null
    setLoading(true)
    setError(null)
    setText(null)
    setObjectUrl(null)
    fetchArtifactPreview(artifact.id, token)
      .then((res) => {
        if (cancelled) {
          if (res.objectUrl) URL.revokeObjectURL(res.objectUrl)
          return
        }
        if (res.text != null) setText(res.text)
        if (res.objectUrl) {
          created = res.objectUrl
          setObjectUrl(res.objectUrl)
        }
      })
      .catch((err) => {
        if (!cancelled) setError(getErrorMessage(err, "Failed to load the preview"))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
      if (created) URL.revokeObjectURL(created)
    }
  }, [artifact, token])

  if (!artifact) return null

  return (
    <div className="artifact-preview" role="dialog" aria-label={artifact.filename}>
      <div className="artifact-preview__head">
        <span className="artifact-preview__title">{artifact.title || artifact.filename}</span>
        <button type="button" className="page-activity__action-btn" onClick={onClose}>
          Close
        </button>
      </div>
      <div className="artifact-preview__body">
        {loading ? <p className="page-activity__empty">Loading…</p> : null}
        {error ? (
          <p className="settings-section__error" role="alert">
            {error}
          </p>
        ) : null}
        {text != null ? <pre className="artifact-preview__text">{text}</pre> : null}
        {objectUrl ? (
          <img className="artifact-preview__image" src={objectUrl} alt={artifact.filename} />
        ) : null}
      </div>
    </div>
  )
}
