import { useCallback, useEffect, useState } from "react"
import { getInitials } from "@buildmax/gui"
import type { ApiSecret } from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { createSecret, editSecret, listSecrets, setSecretState } from "./api"

/**
 * SpaceSecrets manages a space's stored credentials: create one, edit its items,
 * disable or destroy it. Values are write-only -- nothing here reads one back.
 *
 * The two consequences in §3 of the design are stated plainly at the top,
 * because a Space that misreads them will grant a credential it should not: an
 * agent can read every Secret granted to its run, and a member who can trigger
 * a shared agent can obtain its values. See docs/design/space-secrets.md.
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

export function SpaceSecrets({
  token,
  spaceId,
  canManage,
}: {
  token: string | null
  spaceId: string | null
  canManage: boolean
}) {
  const [secrets, setSecrets] = useState<ApiSecret[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!token || !spaceId) return
    setLoading(true)
    setError(null)
    try {
      const got = await listSecrets(token, spaceId)
      setSecrets(got.secrets ?? [])
    } catch (err) {
      setError(getErrorMessage(err, "Failed to load this space's secrets"))
    } finally {
      setLoading(false)
    }
  }, [token, spaceId])

  useEffect(() => {
    void load()
  }, [load])

  if (!canManage) {
    return (
      <section className="sec">
        <div className="sec__head">
          <div>
            <h2 className="sec__title">Secrets</h2>
            <p className="sec__copy">
              Only a space owner can view or manage this space&apos;s secrets.
            </p>
          </div>
        </div>
      </section>
    )
  }

  const live = secrets.filter((s) => s.state !== "destroyed")

  return (
    <section className="sec">
      <div className="sec__head">
        <div>
          <h2 className="sec__title">Secrets</h2>
          <p className="sec__copy">
            Credentials this space&apos;s agents can use — a GitHub token, an internal
            API key. Stored encrypted; values are never shown again after you save
            them.
          </p>
        </div>
        {!creating ? (
          <button className="btn btn--primary" onClick={() => setCreating(true)}>
            New secret
          </button>
        ) : null}
      </div>

      <div className="sec-callout" role="note">
        <KeyIcon />
        <div>
          <strong>An agent you grant a secret to can read its value.</strong> It runs
          commands the model chooses, and a value in its environment can be printed —
          so anyone who can trigger that agent can obtain the value without owning the
          secret. Prefer a short-lived, narrowly scoped credential, and don&apos;t
          grant one to an agent you would not hand it to directly.
        </div>
      </div>

      {error ? (
        <p className="sec__error" role="alert">
          {error}
        </p>
      ) : null}

      {creating ? (
        <CreateSecretForm
          onCancel={() => setCreating(false)}
          onCreate={async (name, description, items) => {
            if (!token || !spaceId) return
            await createSecret(token, spaceId, { name, description, items })
            setCreating(false)
            await load()
          }}
        />
      ) : null}

      {loading ? (
        <div className="sec-list" aria-hidden>
          {[0, 1].map((i) => (
            <div key={i} className="sec-card sec-card--skeleton" />
          ))}
        </div>
      ) : live.length === 0 && !creating && !error ? (
        <div className="sec-empty">
          <KeyIcon />
          <p className="sec-empty__title">No secrets yet</p>
          <p className="sec-empty__copy">
            Add a credential your space&apos;s agents can use. Its value is encrypted on
            save and never shown again.
          </p>
        </div>
      ) : (
        <ul className="sec-list">
          {secrets.map((secret) => (
            <li key={secret.id}>
              <SecretCard
                secret={secret}
                open={editing === secret.id}
                onToggle={() => setEditing(editing === secret.id ? null : secret.id)}
                onEditItems={async (req) => {
                  if (!token || !spaceId) return
                  await editSecret(token, spaceId, secret.id, req)
                  setEditing(null)
                  await load()
                }}
                onSetState={async (state) => {
                  if (!token || !spaceId) return
                  await setSecretState(token, spaceId, secret.id, state)
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
    <div className="sec-form">
      <div className="sec-form__head">
        <h3 className="sec-form__title">New secret</h3>
      </div>

      <div className="sec-field">
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
      </div>

      <div className="sec-field">
        <label className="modal__label" htmlFor="secret-description">
          Description <span className="sec-field__optional">optional</span>
        </label>
        <input
          id="secret-description"
          className="modal__input"
          value={description}
          placeholder="What it is for — never the value itself"
          onChange={(e) => setDescription(e.target.value)}
        />
      </div>

      <div className="sec-field">
        <div className="sec-field__row">
          <span className="modal__label">Items</span>
          <button className="btn btn--ghost btn--sm" onClick={() => setRaw(!raw)}>
            {raw ? "Row editor" : "Paste JSON"}
          </button>
        </div>
        {raw ? (
          <textarea
            className="modal__input sec-json"
            rows={6}
            value={rawText}
            spellCheck={false}
            onChange={(e) => setRawText(e.target.value)}
          />
        ) : (
          <ItemRowsEditor rows={rows} setRows={setRows} />
        )}
      </div>

      {error ? (
        <p className="sec__error" role="alert">
          {error}
        </p>
      ) : null}

      <div className="sec-form__actions">
        <button className="btn btn--secondary" onClick={onCancel} disabled={busy}>
          Cancel
        </button>
        <button className="btn btn--primary" onClick={() => void submit()} disabled={busy}>
          {busy ? "Saving…" : "Create secret"}
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
    <div className="sec-items">
      {rows.map((row, i) => (
        <div key={i} className="sec-item">
          <input
            className="modal__input sec-item__key"
            placeholder="ITEM_NAME"
            value={row.key}
            onChange={(e) => {
              const next = rows.slice()
              next[i] = { ...row, key: e.target.value }
              setRows(next)
            }}
          />
          <input
            className="modal__input sec-item__val"
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
            className="sec-item__remove"
            onClick={() => setRows(rows.filter((_, j) => j !== i))}
            aria-label="Remove item"
            title="Remove item"
          >
            ✕
          </button>
        </div>
      ))}
      <button
        className="btn btn--ghost btn--sm sec-items__add"
        onClick={() => setRows([...rows, { key: "", value: "" }])}
      >
        + Add item
      </button>
    </div>
  )
}

const STATE_TONE: Record<ApiSecret["state"], string> = {
  active: "active",
  disabled: "suspended",
  destroyed: "blocked",
}

function SecretCard({
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
    <div className={`sec-card ${open ? "sec-card--open" : ""} ${destroyed ? "sec-card--dead" : ""}`}>
      <div className="sec-card__head">
        <span className="sec-card__logo" aria-hidden>
          {getInitials(secret.name)}
        </span>
        <div className="sec-card__ident">
          <span className="sec-card__name">{secret.name}</span>
          {secret.description ? (
            <span className="sec-card__desc">{secret.description}</span>
          ) : null}
        </div>
        <span className={`sec-status sec-status--${STATE_TONE[secret.state]}`}>
          <span className="sec-status__dot" aria-hidden />
          {secret.state}
        </span>
      </div>

      <div className="sec-card__items">
        {secret.item_names.length > 0 ? (
          secret.item_names.map((n) => (
            <span key={n} className="sec-chip">
              {n}
            </span>
          ))
        ) : (
          <span className="sec-card__noitems">no items</span>
        )}
      </div>

      {!destroyed ? (
        <div className="sec-card__actions">
          <button className="btn btn--secondary btn--sm" onClick={onToggle}>
            {open ? "Close" : "Edit items"}
          </button>
          {secret.state === "active" ? (
            <button className="btn btn--ghost btn--sm" onClick={() => void onSetState("disabled")}>
              Disable
            </button>
          ) : (
            <button className="btn btn--ghost btn--sm" onClick={() => void onSetState("active")}>
              Enable
            </button>
          )}
          <button
            className="btn btn--danger btn--sm sec-card__destroy"
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
    <div className="sec-edit">
      <p className="sec-edit__hint">
        Values are never shown. Set an item to replace its value, or mark one to
        remove.
      </p>
      {secret.item_names.length > 0 ? (
        <div className="sec-remove">
          {secret.item_names.map((name) => (
            <label key={name} className={`sec-remove__row ${remove.has(name) ? "sec-remove__row--on" : ""}`}>
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
        <p className="sec__error" role="alert">
          {error}
        </p>
      ) : null}

      <div className="sec-form__actions">
        <button className="btn btn--primary btn--sm" onClick={() => void submit()} disabled={busy}>
          {busy ? "Saving…" : "Save items"}
        </button>
      </div>
    </div>
  )
}

function KeyIcon() {
  return (
    <svg
      className="sec-icon"
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <circle cx="7.5" cy="15.5" r="4.5" />
      <path d="m10.7 12.3 8.3-8.3" />
      <path d="m16 5 3 3" />
      <path d="m13 8 2.5 2.5" />
    </svg>
  )
}
