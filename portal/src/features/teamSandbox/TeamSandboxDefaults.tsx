import { useCallback, useEffect, useState } from "react"
import { getErrorMessage } from "../../lib/errorMessage"
import {
  TEAM_SANDBOX_FILESYSTEM_TIER_OPTIONS,
  TEAM_SANDBOX_NETWORK_TIER_OPTIONS,
} from "../../lib/sandboxTiers"
import { getSandboxDefaults, setSandboxDefaults } from "./api"

/**
 * TeamSandboxDefaults is the tiers an agent that declares neither inherits --
 * the same "what this team's background runs may use" question TeamPlugins
 * answers for installed tools, answered here for network and filesystem
 * access. See docs/design/agent-sandbox-policy.md §9 M3.
 */
export function TeamSandboxDefaults({
  token,
  teamId,
  canManage,
}: {
  token: string | null
  teamId: string | null
  canManage: boolean
}) {
  const [networkTier, setNetworkTier] = useState("")
  const [filesystemTier, setFilesystemTier] = useState("")
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!token || !teamId) return
    setLoading(true)
    setError(null)
    try {
      const got = await getSandboxDefaults(token, teamId)
      setNetworkTier(got.sandbox_network_tier ?? "")
      setFilesystemTier(got.sandbox_filesystem_tier ?? "")
    } catch (err) {
      setError(getErrorMessage(err, "Failed to load this team's default sandbox tiers"))
    } finally {
      setLoading(false)
    }
  }, [token, teamId])

  useEffect(() => {
    void load()
  }, [load])

  async function save(next: { networkTier: string; filesystemTier: string }) {
    if (!token || !teamId) return
    setSaving(true)
    setError(null)
    const previous = { networkTier, filesystemTier }
    setNetworkTier(next.networkTier)
    setFilesystemTier(next.filesystemTier)
    try {
      await setSandboxDefaults(token, teamId, {
        sandbox_network_tier: next.networkTier,
        sandbox_filesystem_tier: next.filesystemTier,
      })
    } catch (err) {
      setNetworkTier(previous.networkTier)
      setFilesystemTier(previous.filesystemTier)
      setError(getErrorMessage(err, "Failed to update the team's default sandbox tiers"))
    } finally {
      setSaving(false)
    }
  }

  return (
    <section className="settings-page__section">
      <div className="settings-page__section-head">
        <div>
          <h2 className="settings-page__section-title">Sandbox defaults</h2>
          <p className="settings-page__section-copy">
            What an agent that declares no network or filesystem tier of its own runs
            under. An agent's own declaration always overrides this.
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
      ) : (
        <div className="admin-sections">
          <div>
            <label className="modal__label" htmlFor="team-sandbox-network-tier">
              Network access
            </label>
            <select
              id="team-sandbox-network-tier"
              className="modal__input"
              value={networkTier}
              disabled={!canManage || saving}
              onChange={(e) => save({ networkTier: e.target.value, filesystemTier })}
            >
              {TEAM_SANDBOX_NETWORK_TIER_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
            <p className="modal__hint">
              {TEAM_SANDBOX_NETWORK_TIER_OPTIONS.find((o) => o.value === networkTier)?.description}
            </p>
          </div>
          <div>
            <label className="modal__label" htmlFor="team-sandbox-filesystem-tier">
              Filesystem access
            </label>
            <select
              id="team-sandbox-filesystem-tier"
              className="modal__input"
              value={filesystemTier}
              disabled={!canManage || saving}
              onChange={(e) => save({ networkTier, filesystemTier: e.target.value })}
            >
              {TEAM_SANDBOX_FILESYSTEM_TIER_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
            <p className="modal__hint">
              {
                TEAM_SANDBOX_FILESYSTEM_TIER_OPTIONS.find((o) => o.value === filesystemTier)
                  ?.description
              }
            </p>
          </div>
        </div>
      )}
    </section>
  )
}
