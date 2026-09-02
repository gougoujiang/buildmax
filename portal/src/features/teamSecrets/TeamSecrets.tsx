import { useCallback, useEffect, useState } from "react"
import type { ApiSecret } from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { createSecret, editSecret, listSecrets, setSecretState } from "./api"

/**
 * TeamSecrets manages a team's stored credentials: create one, edit its items,
 * disable or destroy it. Values are write-only -- nothing here reads one back.
 *
 * The two consequences in §3 of the design are stated plainly at the top,
 * because a Team that misreads them will grant a credential it should not: an
 * agent can read every Secret granted to its run, and a member who can trigger
 * a shared agent can obtain its values. See docs/design/team-secrets.md.
 */

type ItemRow = { key: string; value: string }

function emptyRows(): ItemRow[] {
  return [{ key: "", value: "" }]
}

function rowsToItems(rows: ItemRow[]): Record<string, string> {
  const items: Record<string, string> = {}
  for (const row of rows) {
    const key = row.key.trim()
    if (key) items[key] = row.value
  }
  return items
}

export function TeamSecrets({
  token,
  teamId,
  canManage,
}: {
  token: string | null
  teamId: string | null
  canManage: boolean
}) {
  const [secrets, setSecrets] = useState<ApiSecret[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!token || !teamId) return
    setLoading(true)
    setError(null)
    try {
      const got = await listSecrets(token, teamId)
      setSecrets(got.secrets ?? [])
    } catch (err) {
      setError(getErrorMessage(err, "Failed to load this team's secrets"))
    } finally {
      setLoading(false)
    }
  }, [token, teamId])

  useEffect(() => {
    void load()
  }, [load])

  if (!canManage) {
    return (
      <section className="settings-page__section">
        <div className="settings-page__section-head">
          <div>
            <h2 className="settings-page__section-title">Secrets</h2>
            <p className="settings-page__section-copy">
              Only a team owner can view or manage this team's secrets.
            </p>
          </div>
        </div>
      </section>
    )
  }

  return (
    <section className="settings-page__section">
      <div className="settings-page__section-head">
        <div>
          <h2 className="settings-page__section-title">Secrets</h2>
          <p className="settings-page__section-copy">
            Credentials this team's agents can use — a GitHub token, an internal API
            key. Stored encrypted; values are never shown again after you save them.
          </p>
        </div>
        {!creating ? (
          <button className="btn btn--primary" onClick={() => setCreating(true)}>
            New secret
          </button>
        ) : null}
      </div>

      <div className="settings-section__notice" role="note">
        <strong>Before you add one:</strong> an agent you grant a secret to can read
        its value — it runs commands the model chooses, and a value in its
        environment can be printed. Anyone who can trigger such an agent can obtain
        the value, without owning the secret. Prefer a short-lived, narrowly scoped
        credential, and don't grant a secret to an agent you would not hand it to
        directly.
      </div>

      {error ? (
        <p className="settings-section__error" role="alert">
          {error}
        </p>
      ) : null}

      {creating ? (
        <CreateSecretForm
          onCancel={() => setCreating(false)}
          onCreate={async (name, description, items) => {
            if (!token || !teamId) return
            await createSecret(token, teamId, { name, description, items })
            setCreating(false)
            await load()
          }}
        />
      ) : null}

      {loading ? (
        <p className="admin-empty">Loading…</p>
      ) : secrets.length === 0 && !creating ? (
        <p className="admin-empty">This team has no secrets yet.</p>
      ) : (
        <ul className="admin-list">
          {secrets.map((secret) => (
            <li key={secret.id} className="admin-list__item">
              <SecretRow
                secret={secret}
                open={editing === secret.id}
                onToggle={() => setEditing(editing === secret.id ? null : secret.id)}
                onEditItems={async (req) => {
                  if (!token || !teamId) return
                  await editSecret(token, teamId, secret.id, req)
                  setEditing(null)
                  await load()
                }}
                onSetState={async (state) => {
                  if (!token || !teamId) return
                  await setSecretState(token, teamId, secret.id, state)
                  await load()
                }}
              />
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

function CreateSecretForm({
  onCancel,
  onCreate,
}: {
  onCancel: () => void
  onCreate: (name: string, description: string, items: Record<string, string>) => Promise<void>
}) {
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [rows, setRows] = useState<ItemRow[]>(emptyRows())
  const [raw, setRaw] = useState(false)
  const [rawText, setRawText] = useState("{\n  \n}")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit() {
    setError(null)
    let items: Record<string, string>
    if (raw) {
      try {
        const parsed: unknown = JSON.parse(rawText)
        if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
          throw new Error("expected a JSON object of string values")
        }
        items = {}
        for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
          items[k] = String(v)
        }
      } catch (err) {
        setError(getErrorMessage(err, "The items are not valid JSON"))
        return
      }
    } else {
      items = rowsToItems(rows)
    }
    if (!name.trim() || Object.keys(items).length === 0) {
      setError("A secret needs a name and at least one item.")
      return
    }
    setBusy(true)
    try {
      await onCreate(name.trim(), description.trim(), items)
    } catch (err) {
      setError(getErrorMessage(err, "Failed to create the secret"))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="admin-card">
      <label className="modal__label" htmlFor="secret-name">
        Name
      </label>
      <input
        id="secret-name"
        className="modal__input"
        value={name}
        placeholder="aws-prod"
        onChange={(e) => setName(e.target.value)}
      />
      <label className="modal__label" htmlFor="secret-description">
        Description
      </label>
      <input
        id="secret-description"
        className="modal__input"
        value={description}
        placeholder="Optional — must not contain a value"
        onChange={(e) => setDescription(e.target.value)}
      />

      <div className="settings-page__section-head">
        <span className="modal__label">Items</span>
        <button className="btn btn--secondary" onClick={() => setRaw(!raw)}>
          {raw ? "Row editor" : "Raw JSON"}
        </button>
      </div>

      {raw ? (
        <textarea
          className="modal__input"
          rows={6}
          value={rawText}
          spellCheck={false}
          onChange={(e) => setRawText(e.target.value)}
        />
      ) : (
        <ItemRowsEditor rows={rows} setRows={setRows} />
      )}

      {error ? (
        <p className="settings-section__error" role="alert">
          {error}
        </p>
      ) : null}

      <div className="modal__actions">
        <button className="btn btn--secondary" onClick={onCancel} disabled={busy}>
          Cancel
        </button>
        <button className="btn btn--primary" onClick={() => void submit()} disabled={busy}>
          Create
        </button>
      </div>
    </div>
  )
}

function ItemRowsEditor({
  rows,
  setRows,
}: {
  rows: ItemRow[]
  setRows: (rows: ItemRow[]) => void
}) {
  return (
    <div className="admin-sections">
      {rows.map((row, i) => (
        <div key={i} className="secret-item-row">
          <input
            className="modal__input"
            placeholder="item name"
            value={row.key}
            onChange={(e) => {
              const next = rows.slice()
              next[i] = { ...row, key: e.target.value }
              setRows(next)
            }}
          />
          <input
            className="modal__input"
            placeholder="value"
            type="password"
            value={row.value}
            onChange={(e) => {
              const next = rows.slice()
              next[i] = { ...row, value: e.target.value }
              setRows(next)
            }}
          />
          <button
            className="btn btn--secondary"
            onClick={() => setRows(rows.filter((_, j) => j !== i))}
            aria-label="Remove item"
          >
            ✕
          </button>
        </div>
      ))}
      <button className="btn btn--secondary" onClick={() => setRows([...rows, { key: "", value: "" }])}>
        Add item
      </button>
    </div>
  )
}

function SecretRow({
  secret,
  open,
  onToggle,
  onEditItems,
  onSetState,
}: {
  secret: ApiSecret
  open: boolean
  onToggle: () => void
  onEditItems: (req: {
    items?: Record<string, string>
    set?: Record<string, string>
    remove?: string[]
  }) => Promise<void>
  onSetState: (state: "active" | "disabled" | "destroyed") => Promise<void>
}) {
  const destroyed = secret.state === "destroyed"
  return (
    <div>
      <div className="admin-list__row">
        <div>
          <div className="admin-list__title">{secret.name}</div>
          <div className="admin-list__subtitle">
            {secret.item_names.join(", ") || "no items"} · {secret.state}
          </div>
          {secret.description ? (
            <div className="admin-list__subtitle">{secret.description}</div>
          ) : null}
        </div>
        {!destroyed ? (
          <div className="admin-list__actions">
            <button className="btn btn--secondary" onClick={onToggle}>
              {open ? "Close" : "Edit items"}
            </button>
            {secret.state === "active" ? (
              <button className="btn btn--secondary" onClick={() => void onSetState("disabled")}>
                Disable
              </button>
            ) : (
              <button className="btn btn--secondary" onClick={() => void onSetState("active")}>
                Enable
              </button>
            )}
            <button
              className="btn btn--danger"
              onClick={() => {
                if (window.confirm(`Destroy secret "${secret.name}"? This cannot be undone.`)) {
                  void onSetState("destroyed")
                }
              }}
            >
              Destroy
            </button>
          </div>
        ) : null}
      </div>
      {open && !destroyed ? (
        <EditItemsForm secret={secret} onEditItems={onEditItems} />
      ) : null}
    </div>
  )
}

function EditItemsForm({
  secret,
  onEditItems,
}: {
  secret: ApiSecret
  onEditItems: (req: {
    items?: Record<string, string>
    set?: Record<string, string>
    remove?: string[]
  }) => Promise<void>
}) {
  const [rows, setRows] = useState<ItemRow[]>(emptyRows())
  const [remove, setRemove] = useState<Set<string>>(new Set())
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function toggleRemove(name: string) {
    const next = new Set(remove)
    if (next.has(name)) next.delete(name)
    else next.add(name)
    setRemove(next)
  }

  async function submit() {
    setError(null)
    const set = rowsToItems(rows)
    const removeList = [...remove]
    if (Object.keys(set).length === 0 && removeList.length === 0) {
      setError("Nothing to change: set an item or mark one to remove.")
      return
    }
    setBusy(true)
    try {
      await onEditItems({ set, remove: removeList })
    } catch (err) {
      setError(getErrorMessage(err, "Failed to edit the secret's items"))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="admin-card">
      <p className="modal__hint">
        Current items: {secret.item_names.join(", ") || "none"}. Values are never shown;
        set an item to replace its value, or mark one to remove.
      </p>
      {secret.item_names.length > 0 ? (
        <div className="admin-sections">
          {secret.item_names.map((name) => (
            <label key={name} className="secret-remove-row">
              <input
                type="checkbox"
                checked={remove.has(name)}
                onChange={() => toggleRemove(name)}
              />
              Remove <code>{name}</code>
            </label>
          ))}
        </div>
      ) : null}

      <span className="modal__label">Set or add items</span>
      <ItemRowsEditor rows={rows} setRows={setRows} />

      {error ? (
        <p className="settings-section__error" role="alert">
          {error}
        </p>
      ) : null}

      <div className="modal__actions">
        <button className="btn btn--primary" onClick={() => void submit()} disabled={busy}>
          Save items
        </button>
      </div>
    </div>
  )
}
