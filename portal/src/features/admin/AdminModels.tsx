import { useCallback, useEffect, useState } from "react"
import type { ApiAdminModel } from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { listAdminModels, setAdminModelEnabled } from "./api"

/**
 * AdminModels shows which models this deployment will call.
 *
 * A model is only usable when it is enabled *and* an alias points at it, and a
 * catalog that showed only the first would leave the most common failure
 * — "the model is on and nothing can call it" — invisible.
 *
 * Adding a model is a command rather than a form. The catalog holds provider
 * credentials, and a key typed into a browser travels through a request body
 * and a proxy log on its way to the database.
 */
export function AdminModels({ token }: { token: string | null }) {
  const [models, setModels] = useState<ApiAdminModel[]>([])
  const [defaultAlias, setDefaultAlias] = useState<string | undefined>()
  const [loading, setLoading] = useState(true)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(() => {
    if (!token) return
    setLoading(true)
    setError(null)
    listAdminModels(token)
      .then((res) => {
        setModels(res.models)
        setDefaultAlias(res.default_alias)
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to load the model catalog")))
      .finally(() => setLoading(false))
  }, [token])

  useEffect(load, [load])

  function toggle(entry: ApiAdminModel) {
    if (!token) return
    if (
      entry.enabled &&
      !window.confirm(
        `Retire ${entry.name}?\n\n` +
          "Teams stop being able to call it. Nothing is deleted and no credential " +
          "changes — enabling it again restores it.",
      )
    )
      return
    setBusyId(entry.id)
    setError(null)
    setAdminModelEnabled(token, entry.id, !entry.enabled)
      .then(load)
      .catch((err) => setError(getErrorMessage(err, "The change did not complete")))
      .finally(() => setBusyId(null))
  }

  return (
    <div className="admin-sections">
      <section className="settings-page__section">
        <div className="settings-page__section-head">
          <div>
            <h2 className="settings-page__section-title">Models</h2>
            <p className="settings-page__section-copy">
              The upstreams this deployment will call.
              {defaultAlias ? ` Callers that name no alias get “${defaultAlias}”.` : ""}
            </p>
          </div>
        </div>

        {error ? (
          <p className="settings-section__error" role="alert">
            {error}
          </p>
        ) : null}

        {loading ? (
          <p className="admin-empty">Loading…</p>
        ) : models.length === 0 ? (
          <p className="admin-empty">The catalog is empty.</p>
        ) : (
          <ul className="admin-list">
            {models.map((entry) => (
              <li key={entry.id} className="admin-list__row">
                <span className="admin-list__main">
                  {entry.name}
                  <span className="admin-list__meta"> · {entry.model}</span>
                </span>
                <span className={entry.enabled ? "admin-pill admin-pill--ok" : "admin-pill"}>
                  {entry.enabled ? "enabled" : "retired"}
                </span>
                {entry.aliases.length > 0 ? (
                  <span className="admin-list__meta">{entry.aliases.join(", ")}</span>
                ) : (
                  // Enabled and unreachable is a state worth naming, because
                  // nothing else in the product reports it.
                  <span className="admin-pill admin-pill--bad">no alias</span>
                )}
                <button
                  type="button"
                  className={entry.enabled ? "admin-button admin-button--danger" : "admin-button"}
                  disabled={busyId === entry.id}
                  onClick={() => toggle(entry)}
                >
                  {entry.enabled ? "Retire" : "Enable"}
                </button>
              </li>
            ))}
          </ul>
        )}

        <p className="admin-scope-note">
          Adding a model is a server command, because it carries a provider credential:
        </p>
        <code className="admin-code__value">
          buildmax-server model add --name &lt;name&gt; --api-url &lt;url&gt; --api-key
          &lt;key&gt; --model &lt;id&gt;
        </code>
        <p className="admin-scope-note">
          A model becomes usable once an alias in <code>server.yaml</code> points at its
          id. Aliases are configuration, so changing them is a deploy rather than a click.
        </p>
      </section>
    </div>
  )
}
