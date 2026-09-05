import { useCallback, useEffect, useRef, useState } from "react"
import type { ApiArtifact } from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { downloadAuthenticated } from "../../lib/download"
import { navigate } from "../../router"
import { useAuth } from "../../contexts/AuthContext"
import { useTeam } from "../../contexts/TeamContext"
import {
  artifactContentUrl,
  artifactLabel,
  confirmArtifactDeletion,
  deleteArtifact,
  formatSize,
  formatTime,
  listArtifacts,
  mayDelete,
  sourceLabel,
  uploadArtifact,
} from "../../features/artifacts"

const PAGE_SIZE = 50

/**
 * Every durable file the current space holds.
 *
 * A top-level area rather than a settings tab: an artifact is what work
 * produced, so it is browsed alongside issues and runs, not alongside the
 * knobs that configure the space.
 */
export function Artifacts() {
  const { token, user } = useAuth()
  const { currentTeamId, currentUserRole } = useTeam()
  const [items, setItems] = useState<ApiArtifact[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)

  const load = useCallback(
    (offset: number) => {
      if (!currentTeamId || !token) return
      setLoading(true)
      setError(null)
      listArtifacts(currentTeamId, token, { limit: PAGE_SIZE, offset })
        .then((res) => {
          setItems((prev) => (offset === 0 ? res.items : [...prev, ...res.items]))
          setTotal(res.total)
        })
        .catch((err) => setError(getErrorMessage(err, "Failed to load artifacts")))
        .finally(() => setLoading(false))
    },
    [currentTeamId, token]
  )

  useEffect(() => {
    load(0)
  }, [load])

  async function onFileChosen(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    // Cleared straight away so choosing the same file twice still fires.
    event.target.value = ""
    if (!file || !currentTeamId || !token) return
    setUploading(true)
    setError(null)
    try {
      await uploadArtifact(currentTeamId, token, file)
      load(0)
    } catch (err) {
      setError(getErrorMessage(err, "Upload failed"))
    } finally {
      setUploading(false)
    }
  }

  async function onDownload(artifact: ApiArtifact) {
    if (!token) return
    setBusyId(artifact.id)
    setError(null)
    try {
      await downloadAuthenticated(artifactContentUrl(artifact.id), token, artifact.filename)
    } catch (err) {
      setError(getErrorMessage(err, "Download failed"))
    } finally {
      setBusyId(null)
    }
  }

  async function onDelete(artifact: ApiArtifact) {
    if (!token) return
    if (!confirmArtifactDeletion(artifact)) return
    setBusyId(artifact.id)
    setError(null)
    try {
      await deleteArtifact(artifact.id, token)
      setItems((prev) => prev.filter((a) => a.id !== artifact.id))
      setTotal((prev) => Math.max(0, prev - 1))
    } catch (err) {
      setError(getErrorMessage(err, "Delete failed"))
    } finally {
      setBusyId(null)
    }
  }

  const countLabel = total === 1 ? "1 artifact" : `${total} artifacts`

  return (
    <div className="page-activity">
      <div className="page-activity__head">
        <div>
          <h1 className="page-activity__title">Artifacts</h1>
          <p className="page-activity__subtitle">
            Files this space keeps. Each one has a stable reference any member can open, and its
            content never changes — a new version is a new artifact.
          </p>
        </div>
        <div className="page-activity__actions">
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => fileInput.current?.click()}
            disabled={uploading || !currentTeamId}
          >
            {uploading ? "Uploading…" : "Upload a file"}
          </button>
          <input
            ref={fileInput}
            type="file"
            className="artifact-list__file-input"
            onChange={onFileChosen}
            hidden
          />
        </div>
      </div>

      {error ? (
        <p className="settings-section__error" role="alert">
          {error}
        </p>
      ) : null}

      <section className="issues-page__panel">
        <div className="issues-page__toolbar">
          <h2 className="issues-page__section-title">All Artifacts</h2>
          <span className="page-activity__meta">{countLabel}</span>
        </div>

        {!error && items.length === 0 && !loading ? (
          <p className="page-activity__empty">
            Nothing kept here yet. Upload a file, or have an agent publish one with
            UploadArtifact.
          </p>
        ) : null}

        {items.length > 0 ? (
          <ul className="artifact-list">
            {items.map((artifact) => (
              <li key={artifact.id} className="artifact-row">
                <div className="artifact-row__main">
                  {/* The name opens the artifact rather than the whole row: the
                      row carries its own buttons, and one cannot nest another. */}
                  <button
                    type="button"
                    className="artifact-row__name"
                    onClick={() => navigate({ name: "artifact", artifactId: artifact.id })}
                  >
                    {artifactLabel(artifact)}
                  </button>
                  <span className="artifact-row__meta">
                    {artifact.filename} · {formatSize(artifact.size_bytes)} ·{" "}
                    {sourceLabel(artifact)}
                  </span>
                </div>
                <div className="artifact-row__meta">
                  <time>{formatTime(artifact.created_at)}</time>
                </div>
                <div className="artifact-row__actions">
                  <button
                    type="button"
                    className="page-activity__action-btn"
                    onClick={() => onDownload(artifact)}
                    disabled={busyId === artifact.id}
                  >
                    Download
                  </button>
                  {mayDelete(artifact, user?.id, currentUserRole) ? (
                    <button
                      type="button"
                      className="page-activity__action-btn"
                      onClick={() => void onDelete(artifact)}
                      disabled={busyId === artifact.id}
                    >
                      Delete
                    </button>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        ) : null}

        {loading ? <p className="page-activity__empty">Loading…</p> : null}

        {items.length < total ? (
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => load(items.length)}
            disabled={loading}
          >
            Show older ({total - items.length} more)
          </button>
        ) : null}
      </section>
    </div>
  )
}
