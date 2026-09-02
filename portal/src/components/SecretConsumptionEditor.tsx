import type { ApiSecret, ApiSecretConsumption, ApiSecretEnvGrant } from "../lib/api/types"

/**
 * Editor for an agent's Team Secret consumption: a list of environment grants,
 * each delivering a selected item under a chosen variable name, or a whole
 * group under each item's own name with an optional prefix. Values are never
 * shown here -- only which secret and item a run receives. See
 * docs/design/team-secrets.md §6.
 */

export function SecretConsumptionEditor({
  value,
  onChange,
  secrets,
}: {
  value: ApiSecretConsumption
  onChange: (next: ApiSecretConsumption) => void
  secrets: ApiSecret[]
}) {
  const grants = value.env ?? []

  function update(next: ApiSecretEnvGrant[]) {
    onChange({ ...value, env: next })
  }

  function setGrant(i: number, patch: Partial<ApiSecretEnvGrant>) {
    update(grants.map((g, j) => (j === i ? { ...g, ...patch } : g)))
  }

  const active = secrets.filter((s) => s.state === "active")

  return (
    <div className="secret-consumption">
      <div className="modal__label">Secret consumption</div>
      <p className="modal__hint">
        Team secrets this agent's runs receive as environment variables. An agent can
        read every secret you grant it.
      </p>

      {secrets.length === 0 ? (
        <p className="modal__hint">
          This team has no active secrets to grant. Create one under Space settings →
          Secrets first.
        </p>
      ) : null}

      {grants.map((grant, i) => {
        const chosen = secrets.find((s) => s.id === grant.secret)
        const wholeGroup = !grant.item
        return (
          <div key={i} className="admin-card">
            <div className="secret-item-row">
              <select
                className="modal__input"
                value={grant.secret}
                onChange={(e) => setGrant(i, { secret: e.target.value, item: "", env_name: "" })}
              >
                <option value="">Choose a secret…</option>
                {active.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
                {chosen && chosen.state !== "active" ? (
                  <option value={chosen.id}>{chosen.name} (disabled)</option>
                ) : null}
              </select>
              <button
                className="btn btn--secondary"
                onClick={() => update(grants.filter((_, j) => j !== i))}
                aria-label="Remove grant"
              >
                ✕
              </button>
            </div>

            <label className="secret-remove-row">
              <input
                type="checkbox"
                checked={wholeGroup}
                onChange={(e) => setGrant(i, e.target.checked ? { item: "" } : { item: "", prefix: "" })}
              />
              Grant the whole group (every item as its own variable)
            </label>

            {wholeGroup ? (
              <div>
                <label className="modal__label">Variable name prefix (optional)</label>
                <input
                  className="modal__input"
                  placeholder="e.g. AWS_"
                  value={grant.prefix ?? ""}
                  onChange={(e) => setGrant(i, { prefix: e.target.value })}
                />
              </div>
            ) : (
              <div className="secret-item-row">
                <select
                  className="modal__input"
                  value={grant.item ?? ""}
                  onChange={(e) => setGrant(i, { item: e.target.value })}
                >
                  <option value="">Choose an item…</option>
                  {(chosen?.item_names ?? []).map((name) => (
                    <option key={name} value={name}>
                      {name}
                    </option>
                  ))}
                </select>
                <input
                  className="modal__input"
                  placeholder="variable name, e.g. GH_TOKEN"
                  value={grant.env_name ?? ""}
                  onChange={(e) => setGrant(i, { env_name: e.target.value })}
                />
              </div>
            )}

            <label className="secret-remove-row">
              <input
                type="checkbox"
                checked={grant.optional ?? false}
                onChange={(e) => setGrant(i, { optional: e.target.checked })}
              />
              Optional — skip if the secret is missing rather than failing the run
            </label>
          </div>
        )
      })}

      <button
        className="btn btn--secondary"
        disabled={secrets.length === 0}
        onClick={() => update([...grants, { secret: "", item: "", env_name: "" }])}
      >
        Add secret grant
      </button>
    </div>
  )
}
