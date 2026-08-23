import { useCallback, useEffect, useState } from "react"
import type { ApiAdminModel } from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { listAdminModels, setAdminModelEnabled } from "./api"

/**
 * AdminModels shows which models this deployment will call.
 *
 * Every enabled model is callable by every user of the deployment, so enabled
 * state is the whole answer to whether a row can be used. One of them is the
 * default, which is what a caller that names no model gets.
 *
 * Adding a model is a command rather than a form. The catalog holds provider
 * credentials, and a key typed into a browser travels through a request body
 * and a proxy log on its way to the database.
 */
export function AdminModels({ token }: { token: string | null }) {
  const [models, setModels] = useState<ApiAdminModel[]>([])
  const [defaultModel, setDefaultModel] = useState<string | undefined>()
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
        setDefaultModel(res.default_model)
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
          "Callers stop being able to name it. Nothing is deleted and no credential " +
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
              {defaultModel ? ` Callers that name no model get “${defaultModel}”.` : ""}
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
                {entry.name === defaultModel ? (
                  <span className="admin-list__meta">default</span>
                ) : null}
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
