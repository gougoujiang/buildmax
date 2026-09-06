import { useEffect, useState } from "react"
import type { ApiAdminMe, ApiAdminSystem } from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { getAdminConfig, getAdminMe, getAdminSystem } from "./api"

function StatusPill({ ok, label }: { ok: boolean; label: string }) {
  return (
    <span className={ok ? "admin-pill admin-pill--ok" : "admin-pill admin-pill--bad"}>{label}</span>
  )
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="admin-fact">
      <span className="admin-fact__label">{label}</span>
      <span className="admin-fact__value">{value}</span>
    </div>
  )
}

/**
 * AdminOverview answers "is this deployment all right", which is why it is the
 * first page rather than a list of accounts.
 *
 * A failed dependency is named and not explained. That matches the server,
 * which withholds the reason on purpose: connection errors carry DSNs,
 * endpoints, and bucket names, and they belong in the log where an operator
 * already has to be.
 */
export function AdminOverview({ token }: { token: string | null }) {
  const [system, setSystem] = useState<ApiAdminSystem | null>(null)
  const [config, setConfig] = useState<Record<string, unknown> | null>(null)
  const [me, setMe] = useState<ApiAdminMe | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!token) return
    let cancelled = false
    setLoading(true)
    Promise.all([
      getAdminSystem(token),
      getAdminConfig(token).catch(() => null),
      getAdminMe(token).catch(() => null),
    ])
      .then(([sys, cfg, mine]) => {
        if (cancelled) return
        setSystem(sys)
        setConfig(cfg)
        setMe(mine)
      })
      .catch((err) => {
        if (!cancelled) setError(getErrorMessage(err, "Failed to load the deployment status"))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [token])

  if (loading) return <p className="admin-empty">Loading the deployment status…</p>
  if (error) {
    return (
      <p className="settings-section__error" role="alert">
        {error}
      </p>
    )
  }
  if (!system) return null

  const warnings = Array.isArray(config?.warnings) ? (config.warnings as string[]) : []
  const runStatuses = Object.entries(system.task_runs).sort(([a], [b]) => a.localeCompare(b))
  const myGrant = me?.grants?.[0]
  const grantedBy = myGrant
    ? myGrant.granted_by === "buildmax-server"
      ? "the operator command"
      : myGrant.granted_by
    : ""
  // The redacted config minus warnings, which have their own section. The server
  // has already reduced every credential to a set/not-set boolean, so this is
  // safe to render whole.
  const configEntries = Object.entries(config ?? {}).filter(([key]) => key !== "warnings")

  return (
    <div className="admin-sections">
      {myGrant ? (
        <section className="settings-page__section">
          <div className="settings-page__section-head">
            <div>
              <h2 className="settings-page__section-title">Your access</h2>
              <p className="settings-page__section-copy">
                Why you can see this area. Deployment authority is separate from any
                space role you also hold.
              </p>
            </div>
          </div>
          <div className="admin-facts">
            <Fact label="Role" value={myGrant.role} />
            <Fact label="Granted by" value={grantedBy} />
            <Fact label="Granted" value={new Date(myGrant.granted_at).toLocaleString()} />
          </div>
        </section>
      ) : null}

      <section className="settings-page__section">
        <div className="settings-page__section-head">
          <div>
            <h2 className="settings-page__section-title">Health</h2>
            <p className="settings-page__section-copy">
              What this server reports about itself. A failed check names the dependency
              and not the reason — the reason is in the server log.
            </p>
          </div>
          <StatusPill ok={system.ready} label={system.ready ? "Ready" : "Not ready"} />
        </div>
        <div className="admin-facts">
          {system.dependencies.length === 0 ? (
            <p className="admin-empty">This deployment registered no dependency checks.</p>
          ) : (
            system.dependencies.map((dep) => (
              <div key={dep.name} className="admin-fact">
                <span className="admin-fact__label">{dep.name}</span>
                <StatusPill ok={dep.status === "ok"} label={dep.status} />
              </div>
            ))
          )}
        </div>
      </section>

      <section className="settings-page__section">
        <div className="settings-page__section-head">
          <div>
            <h2 className="settings-page__section-title">Deployment</h2>
            <p className="settings-page__section-copy">
              Version, execution mode, and how work is flowing.
            </p>
          </div>
        </div>
        <div className="admin-facts">
          <Fact label="Version" value={system.version} />
          <Fact label="Worker run mode" value={system.worker_run_mode ?? "unknown"} />
          <Fact label="Worker model transport" value={system.worker_llm_transport ?? "unknown"} />
          {/*
            Empty means no worker path passes a sandbox surface, which is every
            deployment today. Saying so is the point: an unreported boundary is
            worse than a missing one.
          */}
          <Fact
            label="Sandbox surface"
            value={system.sandbox_surface ? system.sandbox_surface : "none applied"}
          />
          <Fact label="Self-registration" value={system.allow_signup ? "open" : "closed"} />
          <Fact label="System administrators" value={String(system.system_admins)} />
        </div>
      </section>

      <section className="settings-page__section">
        <div className="settings-page__section-head">
          <div>
            <h2 className="settings-page__section-title">Task runs</h2>
            <p className="settings-page__section-copy">Counts by status across every space.</p>
          </div>
        </div>
        <div className="admin-facts">
          {runStatuses.length === 0 ? (
            <p className="admin-empty">No runs yet.</p>
          ) : (
            runStatuses.map(([status, count]) => (
              <Fact key={status} label={status} value={String(count)} />
            ))
          )}
        </div>
      </section>

      {warnings.length > 0 ? (
        <section className="settings-page__section">
          <div className="settings-page__section-head">
            <div>
              <h2 className="settings-page__section-title">Configuration notes</h2>
              <p className="settings-page__section-copy">
                States worth knowing about. These are not errors — the server is running.
              </p>
            </div>
          </div>
          <ul className="admin-warnings">
            {warnings.map((warning) => (
              <li key={warning} className="admin-warnings__item">
                {warning}
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {configEntries.length > 0 ? (
        <section className="settings-page__section">
          <div className="settings-page__section-head">
            <div>
              <h2 className="settings-page__section-title">Effective configuration</h2>
              <p className="settings-page__section-copy">
                The resolved <code>server.yaml</code>, read-only. Every credential is
                shown only as whether it is set — never its value. Change it by editing
                the file and restarting the server.
              </p>
            </div>
          </div>
          <details className="admin-config">
            <summary className="admin-config__summary">Show configuration</summary>
            <div className="admin-facts">
              {configEntries.map(([key, value]) => (
                <Fact
                  key={key}
                  label={key}
                  value={typeof value === "object" ? JSON.stringify(value) : String(value)}
                />
              ))}
            </div>
          </details>
        </section>
      ) : null}

      <section className="settings-page__section">
        <div className="settings-page__section-head">
          <div>
            <h2 className="settings-page__section-title">Schema</h2>
            <p className="settings-page__section-copy">
              Migrations applied beyond the additive schema the row structs own. Not a
              schema version.
            </p>
          </div>
        </div>
        {system.schema_migrations.length === 0 ? (
          <p className="admin-empty">No migrations have been applied.</p>
        ) : (
          <ul className="admin-list">
            {system.schema_migrations.map((migration) => (
              <li key={migration.id} className="admin-list__row">
                <span className="admin-list__main">{migration.id}</span>
                <time className="admin-list__meta">
                  {new Date(migration.applied_at).toLocaleString()}
                </time>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
