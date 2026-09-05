import { useCallback, useEffect, useState } from "react"
import { getInitials } from "@buildmax/gui"
import type {
  ApiAgent,
  ApiPlugin,
  ApiPluginActivation,
  ApiPluginCuration,
  ApiPluginRelease,
} from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { getAgents } from "../agents/api"
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
        catalog.plugins
          .filter((entry: ApiPlugin) => !entry.archived_at)
          .map((entry: ApiPlugin) =>
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

  const activeCount = rows.filter((r) => r.activation?.enabled).length

  return (
    <section className="tp">
      <div className="tp__head">
        <div>
          <h2 className="tp__title">Plugins</h2>
          <p className="tp__copy">
            What this team&apos;s background runs may use. An agent loads only the
            plugins it names — activating one here makes it available to name, and
            changes no existing agent.
          </p>
        </div>
        {!loading && rows.length > 0 ? (
          <span className="tp__count">
            {activeCount} of {rows.length} active
          </span>
        ) : null}
      </div>

      {error ? (
        <p className="tp__error" role="alert">
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
        <div className="tp-list" aria-hidden>
          {[0, 1, 2].map((i) => (
            <div key={i} className="tp-card tp-card--skeleton" />
          ))}
        </div>
      ) : rows.length === 0 ? (
        <p className="tp-empty">This deployment has published nothing yet.</p>
      ) : (
        <ul className="tp-list">
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

      <p className="tp-foot">
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
    <div className="tp-mode">
      <div className="tp-mode__text">
        <span className="tp-mode__label">
          <span className={`tp-mode__badge tp-mode__badge--${curation}`}>
            {curation === "curated" ? "Curated" : "Open"}
          </span>
          catalog mode
        </span>
        <p className="tp-mode__copy">{curationCopy(curation)}</p>
      </div>
      {canManage ? (
        <button
          type="button"
          className="tp-btn"
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
  const status = pluginStatus(row)
  const chips = contributionChips(row.newest)
  return (
    <li className={`tp-card ${expanded ? "tp-card--open" : ""}`}>
      <div className="tp-card__head">
        <span className="tp-card__logo" aria-hidden>
          {getInitials(row.displayName)}
        </span>
        <div className="tp-card__ident">
          <span className="tp-card__title">{row.displayName}</span>
          <span className="tp-card__name">{row.name}</span>
        </div>
        <span className={`tp-status tp-status--${status.tone}`}>
          <span className="tp-status__dot" aria-hidden />
          {status.label}
        </span>
      </div>

      <p className="tp-card__meta">{metaLine(row)}</p>

      {chips.length > 0 || row.staleVersion ? (
        <div className="tp-chips">
          {chips.map((chip) => (
            <span key={chip} className="tp-chip">
              {chip}
            </span>
          ))}
          {row.staleVersion ? (
            <span className="tp-chip tp-chip--warn">Update to {row.staleVersion} available</span>
          ) : null}
        </div>
      ) : null}

      <div className="tp-card__actions">
        <button type="button" className="tp-btn tp-btn--ghost" onClick={onToggle}>
          {expanded ? "Hide details" : "Details"}
        </button>
        {canManage && !activation && row.newest ? (
          <button type="button" className="tp-btn" disabled={busy} onClick={onActivate}>
            {busy ? "Working…" : "Activate"}
          </button>
        ) : null}
        {canManage && activation && row.staleVersion ? (
          <button
            type="button"
            className="tp-btn"
            disabled={busy}
            onClick={() => onUpdate(row.staleVersion as string)}
          >
            {busy ? "Working…" : `Update to ${row.staleVersion}`}
          </button>
        ) : null}
        {canManage && activation ? (
          <button
            type="button"
            className="tp-btn tp-btn--ghost"
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

/** pluginStatus is the single at-a-glance state a reader scans down the list. */
function pluginStatus(row: PluginRow): { label: string; tone: string } {
  if (row.executableOnly) return { label: "Needs operator approval", tone: "blocked" }
  if (!row.activation) {
    if (!row.newest) return { label: "No release", tone: "idle" }
    return { label: "Available", tone: "idle" }
  }
  if (!row.activation.enabled) return { label: "Suspended", tone: "suspended" }
  return { label: "Active", tone: "active" }
}

/** metaLine is the plain-language line under the identity. */
function metaLine(row: PluginRow): string {
  if (!row.activation) {
    if (row.executableOnly) {
      return "Every release contributes hooks or MCP servers, which a team cannot activate yet."
    }
    if (!row.newest) return "Nothing here can be activated."
    return "Not activated — no agent can name it until it is."
  }
  const used =
    row.usedBy.length > 0
      ? `Named by ${row.usedBy.join(", ")}`
      : "No agent names it, so no run loads it"
  return `Pinned to v${row.activation.version} · ${used}`
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
    <div className="tp-detail">
      {row.description ? <p className="tp-detail__desc">{row.description}</p> : null}
      {row.activation ? (
        <p className="tp-detail__meta">
          {originCopy(row.activation)} · digest <code>{row.activation.digest}</code>
        </p>
      ) : null}
      {row.activation && !row.activation.enabled ? (
        <p className="tp-detail__note">
          While suspended, a run whose agent names this plugin fails rather than
          running without it.
        </p>
      ) : null}
      {row.newest ? <ReleaseReport release={row.newest} /> : null}
      {row.executableOnly ? (
        <p className="tp-detail__note">
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
  const groups = contributionGroups(release)
  return (
    <div className="tp-detail__section">
      <p className="tp-detail__meta">
        Newest activatable release <strong>v{release.version}</strong>, published by{" "}
        {release.published_by}.
      </p>
      {groups.map((group) => (
        <div key={group.label} className="tp-detail__group">
          <span className="tp-detail__group-label">{group.label}</span>
          <div className="tp-chips">
            {group.items.map((item) => (
              <span key={item} className="tp-chip tp-chip--mono">
                {item}
              </span>
            ))}
          </div>
        </div>
      ))}
      {release.inspection.env_refs?.length ? (
        <p className="tp-detail__note">
          {/* No per-team secret exists yet, so an unset variable is the usual
              reason an activated plugin starts and does nothing. */}
          Reads {release.inspection.env_refs.join(", ")}. A worker holds no per-team
          secrets, so any it needs will be unset.
        </p>
      ) : null}
    </div>
  )
}

/** contributionChips is the collapsed card's one-line payload summary. */
function contributionChips(release: ApiPluginRelease | null): string[] {
  if (!release) return []
  const insp = release.inspection
  const chips: string[] = []
  if (insp.skills?.length) chips.push(countLabel(insp.skills.length, "skill"))
  if (insp.subagents?.length) chips.push(countLabel(insp.subagents.length, "subagent"))
  if (insp.mcp?.length) chips.push(countLabel(insp.mcp.length, "MCP server"))
  if (insp.hooks?.length) chips.push(countLabel(insp.hooks.length, "hook"))
  return chips
}

/** contributionGroups is the expanded, named list of what a release brings. */
function contributionGroups(release: ApiPluginRelease): { label: string; items: string[] }[] {
  const insp = release.inspection
  const groups: { label: string; items: string[] }[] = []
  if (insp.skills?.length) groups.push({ label: "Skills", items: insp.skills })
  if (insp.subagents?.length)
    groups.push({ label: "Subagents", items: insp.subagents.map((s) => s.name) })
  if (insp.mcp?.length)
    groups.push({ label: "MCP servers", items: insp.mcp.map((s) => `${s.id} (${s.transport})`) })
  if (insp.hooks?.length)
    groups.push({ label: "Hooks", items: insp.hooks.map((h) => `${h.event} · ${h.type}`) })
  return groups
}

function countLabel(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? "" : "s"}`
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
