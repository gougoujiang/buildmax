import { useCallback, useEffect, useState } from "react"
import type { ApiPlugin, ApiPluginRelease } from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { getPlugin, listPlugins } from "./api"

/**
 * PluginCatalog is what this deployment publishes, for somebody deciding
 * whether to install it.
 *
 * It says nothing about whether anything is installed. Installing happens on a
 * machine — a terminal or Desktop — and this page cannot see one, so it hands
 * over the command rather than a button that would be lying about where it ran.
 */
export function PluginCatalog({ token }: { token: string | null }) {
  const [plugins, setPlugins] = useState<ApiPlugin[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [opened, setOpened] = useState<string | null>(null)

  const load = useCallback(() => {
    if (!token) return
    setLoading(true)
    setError(null)
    listPlugins(token)
      .then((res) => setPlugins(res.plugins))
      .catch((err) => setError(getErrorMessage(err, "Failed to load the plugin catalog")))
      .finally(() => setLoading(false))
  }, [token])

  useEffect(load, [load])

  return (
    <section className="settings-page__section">
      <div className="settings-page__section-head">
        <div>
          <h2 className="settings-page__section-title">Plugins</h2>
          <p className="settings-page__section-copy">
            What this deployment publishes: skills, subagents, MCP servers, and hooks
            you can install on your own machine.
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
      ) : plugins.length === 0 ? (
        <p className="admin-empty">This deployment has published nothing yet.</p>
      ) : (
        <ul className="admin-list">
          {plugins.map((entry) => (
            <li key={entry.plugin_id} className="admin-list__row">
              <span className="admin-list__main">
                {entry.display_name || entry.name}
                {entry.description ? (
                  <span className="admin-list__meta"> · {entry.description}</span>
                ) : null}
              </span>
              <button
                type="button"
                className="admin-button"
                onClick={() => setOpened(opened === entry.name ? null : entry.name)}
              >
                {opened === entry.name ? "Hide" : "Details"}
              </button>
            </li>
          ))}
        </ul>
      )}

      {opened ? <PluginDetail token={token} name={opened} /> : null}

      <p className="admin-scope-note">
        Installing happens where the agent runs. This page cannot see your machine,
        so what is installed there is <code>buildmax plugin list</code> there.
      </p>
    </section>
  )
}

/** PluginDetail shows one plugin's newest release and how to install it. */
function PluginDetail({ token, name }: { token: string | null; name: string }) {
  const [releases, setReleases] = useState<ApiPluginRelease[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!token) return
    setLoading(true)
    setError(null)
    getPlugin(token, name)
      .then((res) => setReleases(res.releases))
      .catch((err) => setError(getErrorMessage(err, "Failed to load releases")))
      .finally(() => setLoading(false))
  }, [token, name])

  if (loading) return <p className="admin-empty">Loading releases…</p>
  if (error)
    return (
      <p className="settings-section__error" role="alert">
        {error}
      </p>
    )

  const newest = newestInstallable(releases)
  if (!newest) {
    return (
      <p className="admin-empty">
        Nothing here is installable: every release was withdrawn. An exact version can
        still be recovered from a terminal.
      </p>
    )
  }

  return (
    <div className="admin-sections">
      <p className="admin-scope-note">
        Newest release <strong>{newest.version}</strong>, published by {newest.published_by}.
        {newest.min_buildmax_version
          ? ` Needs BuildMax ${newest.min_buildmax_version} or newer.`
          : ""}
      </p>
      <ul className="admin-list">
        {contributionRows(newest).map((row) => (
          <li key={row} className="admin-list__row">
            <span className="admin-list__main">{row}</span>
          </li>
        ))}
      </ul>
      {newest.inspection.env_refs?.length ? (
        <p className="admin-scope-note">
          {/* The usual reason a plugin looks installed and does nothing. */}
          Reads these environment variables: {newest.inspection.env_refs.join(", ")}
        </p>
      ) : null}
      <code className="admin-code__value">buildmax plugin install {name}</code>
    </div>
  )
}

/**
 * newestInstallable is what `buildmax plugin install` would take by default.
 *
 * Withdrawn releases are skipped, and so is anything that will not order —
 * publishing rejects those, so one here came from a build that did not.
 */
export function newestInstallable(releases: ApiPluginRelease[]): ApiPluginRelease | null {
  let best: ApiPluginRelease | null = null
  let bestKey: number[] | null = null
  for (const release of releases) {
    if (release.yanked_at) continue
    const key = releaseOrder(release.version)
    if (!key) continue
    if (!bestKey || compareOrder(key, bestKey) > 0) {
      best = release
      bestKey = key
    }
  }
  return best
}

/** releaseOrder parses a stable release, or null for a prerelease or garbage. */
function releaseOrder(version: string): number[] | null {
  const match = /^(\d+)\.(\d+)\.(\d+)$/.exec(version)
  if (!match) return null
  return [Number(match[1]), Number(match[2]), Number(match[3])]
}

function compareOrder(a: number[], b: number[]): number {
  for (let i = 0; i < a.length; i += 1) {
    if (a[i] !== b[i]) return a[i] - b[i]
  }
  return 0
}

/** contributionRows lists what a release brings, one line per kind. */
export function contributionRows(release: ApiPluginRelease): string[] {
  const rows: string[] = []
  const insp = release.inspection
  if (insp.skills?.length) rows.push(`Skills: ${insp.skills.join(", ")}`)
  if (insp.subagents?.length)
    rows.push(`Subagents: ${insp.subagents.map((s) => s.name).join(", ")}`)
  if (insp.mcp?.length)
    rows.push(`MCP servers: ${insp.mcp.map((s) => `${s.id} (${s.transport})`).join(", ")}`)
  if (insp.hooks?.length)
    rows.push(`Hooks: ${insp.hooks.map((h) => `${h.event} ${h.type}`).join(", ")}`)
  if (rows.length === 0) rows.push("Contributes nothing this build recognises")
  return rows
}
