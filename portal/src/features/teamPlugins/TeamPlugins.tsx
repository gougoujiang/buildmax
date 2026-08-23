import { useCallback, useEffect, useState } from "react"
import type {
  ApiAgent,
  ApiPlugin,
  ApiPluginActivation,
  ApiPluginCuration,
  ApiPluginRelease,
} from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { getAgents } from "../agents/api"
import { contributionRows } from "../plugins/PluginCatalog"
import { getPlugin, listPlugins } from "../plugins/api"
import {
  activatePlugin,
  listActivations,
  movePin,
  setActivationEnabled,
  setCuration,
} from "./api"
import { buildPluginRow, curationCopy, originCopy, type PluginRow } from "./model"

/**
 * TeamPlugins is what this team's background runs may use.
 *
 * It is not the catalog page, which says what the deployment publishes and
 * hands over an install command for somebody's own machine. This is the team's
 * decision about its workers, and the record that answers "why did this run
 * have this capability".
 */
export function TeamPlugins({
  token,
  teamId,
  canManage,
}: {
  token: string | null
  teamId: string | null
  canManage: boolean
}) {
  const [rows, setRows] = useState<PluginRow[]>([])
  const [curation, setCurationState] = useState<ApiPluginCuration>("open")
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [opened, setOpened] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!token || !teamId) return
    setLoading(true)
    setError(null)
    try {
      const [catalog, activations, agents] = await Promise.all([
        listPlugins(token),
        listActivations(token, teamId),
        // A reader who cannot list agents still gets the activations; the
        // "which agents name it" line is the part that goes missing.
        getAgents(teamId, token).catch((): ApiAgent[] => []),
      ])
      const byName = new Map<string, ApiPluginActivation>(
        activations.activations.map((a: ApiPluginActivation) => [a.plugin_name, a]),
      )
      const releasesByName = await loadReleases(
        token,
        catalog.plugins.map((p: ApiPlugin) => p.name),
      )
      setCurationState(activations.curation)
      setRows(
        catalog.plugins.map((entry: ApiPlugin) =>
          buildPluginRow({
            name: entry.name,
            displayName: entry.display_name || entry.name,
            description: entry.description ?? "",
            releases: releasesByName.get(entry.name) ?? [],
            activation: byName.get(entry.name) ?? null,
            agents,
          }),
        ),
      )
    } catch (err) {
      setError(getErrorMessage(err, "Failed to load this team's plugins"))
    } finally {
      setLoading(false)
    }
  }, [token, teamId])

  useEffect(() => {
    void load()
  }, [load])

  async function run(key: string, action: () => Promise<unknown>) {
    if (!token || !teamId) return
    setBusy(key)
    setError(null)
    try {
      await action()
      await load()
    } catch (err) {
      setError(getErrorMessage(err, "That did not work"))
    } finally {
      setBusy(null)
    }
  }

  return (
    <section className="settings-page__section">
      <div className="settings-page__section-head">
        <div>
          <h2 className="settings-page__section-title">Plugins</h2>
          <p className="settings-page__section-copy">
            What this team&apos;s background runs may use. An agent loads only the
            plugins it names — activating one here makes it available to name, and
            changes no existing agent.
          </p>
        </div>
      </div>

      {error ? (
        <p className="settings-section__error" role="alert">
          {error}
        </p>
      ) : null}

      <CurationControl
        curation={curation}
        canManage={canManage}
        busy={busy === "curation"}
        onChange={(next) =>
          run("curation", () => setCuration(token as string, teamId as string, next))
        }
      />

      {loading ? (
        <p className="admin-empty">Loading…</p>
      ) : rows.length === 0 ? (
        <p className="admin-empty">This deployment has published nothing yet.</p>
      ) : (
        <ul className="admin-list">
          {rows.map((row) => (
            <PluginRowView
              key={row.name}
              row={row}
              canManage={canManage}
              busy={busy === row.name}
              expanded={opened === row.name}
              onToggle={() => setOpened(opened === row.name ? null : row.name)}
              onActivate={() =>
                run(row.name, () => activatePlugin(token as string, teamId as string, row.name))
              }
              onUpdate={(version) =>
                run(row.name, () =>
                  movePin(token as string, teamId as string, row.name, version),
                )
              }
              onSetEnabled={(enabled) =>
                run(row.name, () =>
                  setActivationEnabled(token as string, teamId as string, row.name, enabled),
                )
              }
            />
          ))}
        </ul>
      )}

      <p className="admin-scope-note">
        A pin never moves on its own: a release published after an activation cannot
        change what a run loads until somebody updates it here. What is installed on
        your own machine is a different thing — <code>buildmax plugin list</code> there.
      </p>
    </section>
  )
}

function CurationControl({
  curation,
  canManage,
  busy,
  onChange,
}: {
  curation: ApiPluginCuration
  canManage: boolean
  busy: boolean
  onChange: (next: ApiPluginCuration) => void
}) {
  const other: ApiPluginCuration = curation === "curated" ? "open" : "curated"
  return (
    <div className="admin-sections">
      <p className="admin-scope-note">{curationCopy(curation)}</p>
      {canManage ? (
        <button
          type="button"
          className="admin-button"
          disabled={busy}
          onClick={() => onChange(other)}
        >
          {busy ? "Saving…" : other === "curated" ? "Curate this list" : "Open the catalog"}
        </button>
      ) : null}
    </div>
  )
}

function PluginRowView({
  row,
  canManage,
  busy,
  expanded,
  onToggle,
  onActivate,
  onUpdate,
  onSetEnabled,
}: {
  row: PluginRow
  canManage: boolean
  busy: boolean
  expanded: boolean
  onToggle: () => void
  onActivate: () => void
  onUpdate: (version: string) => void
  onSetEnabled: (enabled: boolean) => void
}) {
  const { activation } = row
  return (
    <li className="admin-list__row admin-list__row--stacked">
      <span className="admin-list__main">
        {row.displayName}
        <span className="admin-list__meta"> · {activationSummary(row)}</span>
      </span>

      <div className="admin-list__actions">
        <button type="button" className="admin-button" onClick={onToggle}>
          {expanded ? "Hide" : "Details"}
        </button>
        {canManage && !activation && row.newest ? (
          <button type="button" className="admin-button" disabled={busy} onClick={onActivate}>
            {busy ? "Working…" : "Activate"}
          </button>
        ) : null}
        {canManage && activation && row.staleVersion ? (
          <button
            type="button"
            className="admin-button"
            disabled={busy}
            onClick={() => onUpdate(row.staleVersion as string)}
          >
            {busy ? "Working…" : `Update to ${row.staleVersion}`}
          </button>
        ) : null}
        {canManage && activation ? (
          <button
            type="button"
            className="admin-button"
            disabled={busy}
            onClick={() => onSetEnabled(!activation.enabled)}
          >
            {activation.enabled ? "Suspend" : "Resume"}
          </button>
        ) : null}
      </div>

      {expanded ? <PluginRowDetail row={row} /> : null}
    </li>
  )
}

/** activationSummary is the one line that says where this plugin stands. */
export function activationSummary(row: PluginRow): string {
  if (!row.activation) {
    if (row.executableOnly) {
      return "Cannot be activated yet: every release contributes hooks or MCP servers"
    }
    if (!row.newest) return "Nothing here can be activated"
    return "Not activated"
  }
  const state = row.activation.enabled ? "Activated" : "Suspended"
  const stale = row.staleVersion ? `, ${row.staleVersion} available` : ""
  const used =
    row.usedBy.length > 0
      ? `named by ${row.usedBy.join(", ")}`
      : "no agent names it, so no run loads it"
  return `${state} at ${row.activation.version}${stale} · ${used}`
}

function PluginRowDetail({ row }: { row: PluginRow }) {
  return (
    <div className="admin-sections">
      {row.description ? <p className="admin-scope-note">{row.description}</p> : null}
      {row.activation ? (
        <p className="admin-scope-note">
          {originCopy(row.activation)} · digest <code>{row.activation.digest}</code>
        </p>
      ) : null}
      {row.activation && !row.activation.enabled ? (
        <p className="admin-scope-note">
          While suspended, a run whose agent names this plugin fails rather than
          running without it.
        </p>
      ) : null}
      {row.newest ? <ReleaseReport release={row.newest} /> : null}
      {row.executableOnly ? (
        <p className="admin-scope-note">
          Hooks and MCP servers start processes on the infrastructure a worker runs
          on. Activating them needs an operator&apos;s decision, which this
          deployment cannot record yet.
        </p>
      ) : null}
    </div>
  )
}

/** ReleaseReport is the same sanitized report an install shows locally. */
function ReleaseReport({ release }: { release: ApiPluginRelease }) {
  return (
    <>
      <p className="admin-scope-note">
        Newest activatable release <strong>{release.version}</strong>, published by{" "}
        {release.published_by}.
      </p>
      <ul className="admin-list">
        {contributionRows(release).map((line) => (
          <li key={line} className="admin-list__row">
            <span className="admin-list__main">{line}</span>
          </li>
        ))}
      </ul>
      {release.inspection.env_refs?.length ? (
        <p className="admin-scope-note">
          {/* No per-team secret exists yet, so an unset variable is the usual
              reason an activated plugin starts and does nothing. */}
          Reads these environment variables: {release.inspection.env_refs.join(", ")}. A
          worker holds no per-team secrets, so any it needs will be unset.
        </p>
      ) : null}
    </>
  )
}

async function loadReleases(
  token: string,
  names: string[],
): Promise<Map<string, ApiPluginRelease[]>> {
  const pairs = await Promise.all(
    names.map(async (name) => {
      try {
        const res = await getPlugin(token, name)
        return [name, res.releases] as const
      } catch {
        // One unreadable entry must not blank the page: it shows as having no
        // activatable release, which is what the reader can act on anyway.
        return [name, [] as ApiPluginRelease[]] as const
      }
    }),
  )
  return new Map(pairs)
}
