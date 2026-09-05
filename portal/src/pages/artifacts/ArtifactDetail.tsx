import { useCallback, useEffect, useState } from "react"
import type { ApiArtifact } from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { ApiRequestError } from "../../lib/api/client"
import { downloadAuthenticated } from "../../lib/download"
import { navigate } from "../../router"
import { useAuth } from "../../contexts/AuthContext"
import { useApp } from "../../contexts/AppContext"
import { useTeam } from "../../contexts/TeamContext"
import { CopyButton } from "../../components/CopyButton"
import {
  ArtifactPreview,
  ArtifactShareDialog,
  artifactContentUrl,
  artifactLabel,
  confirmArtifactDeletion,
  deleteArtifact,
  formatSize,
  formatTime,
  getArtifact,
  mayDelete,
  sourceLabel,
} from "../../features/artifacts"

interface ArtifactDetailProps {
  artifactId: string
}

/**
 * One artifact, at an address that can be pasted into a message or a document.
 *
 * The page leads with the content itself: the rendered preview is the body,
 * actions sit in the header, and the file's metadata lives in a Details
 * disclosure rather than a card that competes with the content for the top of
 * the page.
 */
export function ArtifactDetail({ artifactId }: ArtifactDetailProps) {
  const { token, user } = useAuth()
  const { setEntityLabel } = useApp()
  const { currentUserRole } = useTeam()
  const [artifact, setArtifact] = useState<ApiArtifact | null>(null)
  const [loading, setLoading] = useState(true)
  const [notFound, setNotFound] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [shareOpen, setShareOpen] = useState(false)

  const load = useCallback(() => {
    if (!token) return
    setLoading(true)
    setError(null)
    setNotFound(false)
    getArtifact(artifactId, token)
      .then(setArtifact)
      .catch((err) => {
        // Only the server's own 404 means there is nothing to show. Any other
        // failure -- the network, a 500 -- is this request going wrong, and
        // reporting it as "no such artifact" would tell the reader something
        // untrue about the deployment's contents.
        if (err instanceof ApiRequestError && err.status === 404) {
          setNotFound(true)
          return
        }
        setError(getErrorMessage(err, "Failed to load the artifact"))
      })
      .finally(() => setLoading(false))
  }, [artifactId, token])

  useEffect(() => {
    load()
  }, [load])

  // Publish the human label so the breadcrumb reads "Artifacts / Newton laws"
  // rather than the opaque id, the same seam agent and issue pages use.
  useEffect(() => {
    if (artifact) setEntityLabel(artifact.id, artifactLabel(artifact))
  }, [artifact, setEntityLabel])

  async function onDownload() {
    if (!artifact || !token) return
    setBusy(true)
    setError(null)
    try {
      await downloadAuthenticated(artifactContentUrl(artifact.id), token, artifact.filename)
    } catch (err) {
      setError(getErrorMessage(err, "Download failed"))
    } finally {
      setBusy(false)
    }
  }

  async function onDelete() {
    if (!artifact || !token) return
    if (!confirmArtifactDeletion(artifact)) return
    setBusy(true)
    setError(null)
    try {
      await deleteArtifact(artifact.id, token)
      navigate({ name: "artifacts" })
    } catch (err) {
      setError(getErrorMessage(err, "Delete failed"))
      setBusy(false)
    }
  }

  if (loading) {
    return (
      <div className="page-activity">
        <p className="page-activity__empty">Loading…</p>
      </div>
    )
  }

  if (!artifact) {
    // The server answers 404 both for an artifact that never existed and for
    // one in a team the reader is not in -- deliberately, so an id cannot be
    // used to probe. This page must not narrate a difference the API refuses
    // to make. A request that simply failed says so instead, and offers a
    // retry, because that one is worth trying again.
    return (
      <div className="page-activity">
        <div className="page-activity__head">
          <div>
            <h1 className="page-activity__title">Artifact</h1>
            <p className="page-activity__subtitle">
              {notFound
                ? "No artifact with this reference is available to you. It may never have existed, may have been deleted, or may belong to a space you are not in."
                : "This artifact could not be loaded."}
            </p>
          </div>
        </div>
        {error ? (
          <p className="settings-section__error" role="alert">
            {error}
          </p>
        ) : null}
        <div className="page-activity__actions">
          {!notFound ? (
            <button type="button" className="page-activity__action-btn" onClick={load}>
              Try again
            </button>
          ) : null}
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => navigate({ name: "artifacts" })}
          >
            Back to artifacts
          </button>
        </div>
      </div>
    )
  }

  const canDelete = mayDelete(artifact, user?.id, currentUserRole)

  return (
    <div className="page-activity">
      <div className="page-activity__head">
        <div>
          <h1 className="page-activity__title">{artifactLabel(artifact)}</h1>
          <p className="page-activity__subtitle">
            {artifact.filename} · {formatSize(artifact.size_bytes)} · {sourceLabel(artifact)}
          </p>
        </div>
        <div className="page-activity__actions">
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => void onDownload()}
            disabled={busy}
          >
            Download
          </button>
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => setShareOpen(true)}
          >
            Share
          </button>
          {canDelete ? (
            <button
              type="button"
              className="page-activity__action-btn"
              onClick={() => void onDelete()}
              disabled={busy}
            >
              Delete
            </button>
          ) : null}
        </div>
      </div>

      {error ? (
        <p className="settings-section__error" role="alert">
          {error}
        </p>
      ) : null}

      {/* The content itself, front and centre. A type the server will not render
          says so in place of a preview. */}
      {artifact.preview !== "none" ? (
        <ArtifactPreview artifact={artifact} token={token} />
      ) : (
        <p className="page-activity__empty">
          This type is served as a download rather than displayed, so there is no preview.
        </p>
      )}

      <details className="artifact-details">
        <summary className="artifact-details__summary">Details</summary>
        <dl className="artifact-details__facts">
          <div>
            <dt>Type</dt>
            <dd>{artifact.media_type || "Unknown"}</dd>
          </div>
          <div>
            <dt>Size</dt>
            <dd>{formatSize(artifact.size_bytes)}</dd>
          </div>
          <div>
            <dt>Origin</dt>
            <dd>{sourceLabel(artifact)}</dd>
          </div>
          <div>
            <dt>Created</dt>
            <dd>{formatTime(artifact.created_at)}</dd>
          </div>
          {artifact.expires_at ? (
            <div>
              <dt>Expires</dt>
              <dd>{formatTime(artifact.expires_at)}</dd>
            </div>
          ) : null}
          <div>
            {/* Proves what was stored, computed while streaming the upload. */}
            <dt>SHA-256</dt>
            <dd className="artifact-details__copyable">
              <code className="artifact-details__digest">{artifact.sha256}</code>
              <CopyButton value={artifact.sha256} label="Copy" />
            </dd>
          </div>
        </dl>
      </details>

      <ArtifactShareDialog
        artifactId={artifact.id}
        token={token}
        open={shareOpen}
        onClose={() => setShareOpen(false)}
      />
    </div>
  )
}
