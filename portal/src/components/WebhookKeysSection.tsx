import { useCallback, useEffect, useState } from "react"
import { getErrorMessage } from "../lib/errorMessage"
import {
  listWebhookKeys,
  createWebhookKey,
  revokeWebhookKey,
  type WebhookKeyMeta,
  type CreateWebhookKeyResponse,
} from "../features/webhookKeys/api"
import { cn } from "../lib/cn"

interface WorkspaceOption {
  id: string
  name: string
}

interface WebhookKeysSectionProps {
  workspaces: WorkspaceOption[]
  token: string | null
}

export function WebhookKeysSection({ workspaces, token }: WebhookKeysSectionProps) {
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState("")
  const [keys, setKeys] = useState<WebhookKeyMeta[]>([])
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [newKey, setNewKey] = useState<CreateWebhookKeyResponse | null>(null)
  const [keyName, setKeyName] = useState("")
  const [revokingId, setRevokingId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  const fetchKeys = useCallback(() => {
    if (!token || !selectedWorkspaceId) return
    setLoading(true)
    listWebhookKeys(selectedWorkspaceId, token)
      .then((res) => setKeys(res.keys))
      .catch((err) => setError(getErrorMessage(err, "Failed to load keys")))
      .finally(() => setLoading(false))
  }, [selectedWorkspaceId, token])

  useEffect(() => {
    fetchKeys()
  }, [fetchKeys])

  function handleCreateKey() {
    if (!token || !selectedWorkspaceId) return
    setError(null)
    setNewKey(null)
    setCreating(true)
    createWebhookKey(selectedWorkspaceId, { name: keyName || undefined }, token)
      .then((res) => {
        setNewKey(res)
        setKeyName("")
        fetchKeys()
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to create key")))
      .finally(() => setCreating(false))
  }

  function handleCopyKey() {
    if (!newKey?.key) return
    navigator.clipboard.writeText(newKey.key).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  function handleCloseNewKey() {
    setNewKey(null)
  }

  function handleRevoke(keyId: string) {
    if (!token || !selectedWorkspaceId) return
    setError(null)
    setRevokingId(keyId)
    revokeWebhookKey(selectedWorkspaceId, keyId, token)
      .then(() => fetchKeys())
      .catch((err) => setError(getErrorMessage(err, "Failed to revoke key")))
      .finally(() => setRevokingId(null))
  }

  const noWorkspaceSelected = !selectedWorkspaceId

  return (
    <section className="settings-section settings-webhook">
      <h2 className="settings-panel__heading">Webhook API keys</h2>
      <div className="settings-panel__heading-divider" role="separator" />
      <div className="settings-webhook__workspace-row">
        <label htmlFor="webhook-workspace-select" className="settings-webhook__label">
          Workspace
        </label>
        <select
          id="webhook-workspace-select"
          className="settings-webhook__select"
          value={selectedWorkspaceId}
          onChange={(e) => setSelectedWorkspaceId(e.target.value)}
          aria-label="Choose workspace for webhook keys"
        >
          <option value="">Choose a workspace…</option>
          {workspaces.map((w) => (
            <option key={w.id} value={w.id}>
              {w.name || w.id}
            </option>
          ))}
        </select>
      </div>
      {noWorkspaceSelected && (
        <p className="settings-section__muted">
          Choose a workspace above to create and manage webhook API keys for it.
        </p>
      )}
      {!noWorkspaceSelected && (
        <>
      <p className="settings-webhook__description">
        Use these keys to trigger runs from external systems (e.g. CI, scripts). Send the key in{" "}
        <code>Authorization: Bearer &lt;key&gt;</code> or <code>X-Webhook-Key</code> when calling{" "}
        <code>POST /api/workspaces/{selectedWorkspaceId}/webhook</code>.
      </p>

      {error && (
        <div className="settings-webhook__error" role="alert">
          {error}
        </div>
      )}

      {newKey && (
        <div className="settings-webhook__new-key" role="dialog" aria-label="New webhook key">
          <p className="settings-webhook__new-key-warning">
            Copy the key now. It won&apos;t be shown again.
          </p>
          <div className="settings-webhook__new-key-row">
            <code className="settings-webhook__new-key-value">{newKey.key}</code>
            <button
              type="button"
              className="settings-webhook__btn settings-webhook__btn--copy"
              onClick={handleCopyKey}
            >
              {copied ? "Copied" : "Copy"}
            </button>
          </div>
          <button
            type="button"
            className="settings-webhook__btn settings-webhook__btn--secondary"
            onClick={handleCloseNewKey}
          >
            Done
          </button>
        </div>
      )}

      <div className="settings-webhook__create">
        <input
          type="text"
          className="settings-webhook__input"
          placeholder="Key name (optional)"
          value={keyName}
          onChange={(e) => setKeyName(e.target.value)}
          disabled={creating}
        />
        <button
          type="button"
          className="settings-webhook__btn settings-webhook__btn--primary"
          onClick={handleCreateKey}
          disabled={creating}
        >
          {creating ? "Creating…" : "Create key"}
        </button>
      </div>

      {loading ? (
        <p className="settings-section__muted">Loading keys…</p>
      ) : keys.length === 0 ? (
        <p className="settings-section__muted">No webhook keys yet. Create one above.</p>
      ) : (
        <ul className="settings-webhook__key-list">
          {keys.map((k) => (
            <li key={k.key_id} className="settings-webhook__key-item">
              <span className="settings-webhook__key-name">{k.name || k.key_id}</span>
              <span className="settings-webhook__key-meta">
                {new Date(k.created_at * 1000).toLocaleString()}
              </span>
              <button
                type="button"
                className={cn(
                  "settings-webhook__btn settings-webhook__btn--danger",
                  revokingId === k.key_id && "settings-webhook__btn--busy"
                )}
                onClick={() => handleRevoke(k.key_id)}
                disabled={revokingId !== null}
              >
                {revokingId === k.key_id ? "Revoking…" : "Revoke"}
              </button>
            </li>
          ))}
        </ul>
      )}
        </>
      )}
    </section>
  )
}
