import { useCallback, useEffect, useState } from "react"
import { getErrorMessage } from "../../lib/errorMessage"
import { getTeamAgentInstructions, setTeamAgentInstructions } from "./api"

const MAX_INSTRUCTIONS_CHARS = 8192

export function TeamAgentInstructions({
  token,
  teamId,
  canManage,
}: {
  token: string | null
  teamId: string | null
  canManage: boolean
}) {
  const [saved, setSaved] = useState("")
  const [draft, setDraft] = useState("")
  const [revision, setRevision] = useState(0)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!token || !teamId) return
    setLoading(true)
    setError(null)
    try {
      const got = await getTeamAgentInstructions(token, teamId)
      setSaved(got.instructions)
      setDraft(got.instructions)
      setRevision(got.revision)
    } catch (err) {
      setError(getErrorMessage(err, "Failed to load Space agent instructions"))
    } finally {
      setLoading(false)
    }
  }, [token, teamId])

  useEffect(() => {
    void load()
  }, [load])

  async function save() {
    if (!token || !teamId || saving || draft === saved) return
    setSaving(true)
    setError(null)
    try {
      const got = await setTeamAgentInstructions(token, teamId, draft)
      setSaved(got.instructions)
      setDraft(got.instructions)
      setRevision(got.revision)
    } catch (err) {
      setError(getErrorMessage(err, "Failed to update Space agent instructions"))
    } finally {
      setSaving(false)
    }
  }

  return (
    <section className="settings-page__section">
      <div className="settings-page__section-head">
        <div>
          <h2 className="settings-page__section-title">Agent instructions</h2>
          <p className="settings-page__section-copy">
            Shared guidance inherited by every background Agent run in this Space. An
            Agent&apos;s own instructions are appended after this layer.
          </p>
        </div>
      </div>

      {error ? <p className="settings-section__error" role="alert">{error}</p> : null}

      {loading ? (
        <p className="admin-empty">Loading…</p>
      ) : (
        <div>
          <label className="modal__label" htmlFor="team-agent-instructions">
            Space-level instructions
          </label>
          <textarea
            id="team-agent-instructions"
            className="modal__textarea team-agent-instructions__textarea"
            rows={10}
            maxLength={MAX_INSTRUCTIONS_CHARS}
            value={draft}
            disabled={!canManage || saving}
            placeholder="For example: Use British English and explain decisions for a technical audience."
            onChange={(event) => setDraft(event.target.value)}
          />
          <div className="team-agent-instructions__footer">
            <p className="modal__hint">
              Sent with every model call. Do not include passwords, API keys, or other secrets.
              {revision > 0 ? ` Current revision: ${revision}.` : ""}
            </p>
            <span className="team-agent-instructions__count">
              {draft.length.toLocaleString()} / {MAX_INSTRUCTIONS_CHARS.toLocaleString()}
            </span>
          </div>
          {canManage ? (
            <button
              type="button"
              className="btn btn--primary"
              disabled={saving || draft === saved}
              onClick={() => void save()}
            >
              {saving ? "Saving…" : "Save instructions"}
            </button>
          ) : (
            <p className="modal__hint">Only Space owners and admins can change these instructions.</p>
          )}
        </div>
      )}
    </section>
  )
}
