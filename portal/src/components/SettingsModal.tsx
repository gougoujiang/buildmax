import { useEffect, useState } from "react"
import { useAuth } from "../contexts/AuthContext"
import { getUsage, type ApiUsage } from "../lib/api"
import { BaseModal } from "./BaseModal"

type SettingsTabId = "usage"

const SETTINGS_TABS: { id: SettingsTabId; label: string }[] = [
  { id: "usage", label: "Usage" },
]

interface SettingsModalProps {
  open: boolean
  onClose: () => void
}

export function SettingsModal({ open, onClose }: SettingsModalProps) {
  const [activeTab, setActiveTab] = useState<SettingsTabId>("usage")
  const { token } = useAuth()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [usage, setUsage] = useState<ApiUsage | null>(null)

  useEffect(() => {
    if (!open || !token) {
      setUsage(null)
      setError(null)
      return
    }
    setLoading(true)
    setError(null)
    getUsage(token)
      .then((data) => {
        setUsage(data)
      })
      .catch((e) => {
        setError(e instanceof Error ? e.message : "Failed to load usage")
      })
      .finally(() => {
        setLoading(false)
      })
  }, [open, token])

  return (
    <BaseModal
      open={open}
      title="Settings"
      titleId="settings-modal-title"
      onClose={onClose}
      className="modal--settings"
    >
      <div className="modal__body settings-modal__body">
        <div
          className="settings-tabs"
          role="tablist"
          aria-label="Settings sections"
        >
          {SETTINGS_TABS.map((tab) => (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={activeTab === tab.id}
              aria-controls={`settings-panel-${tab.id}`}
              id={`settings-tab-${tab.id}`}
              className={`settings-tabs__tab ${activeTab === tab.id ? "settings-tabs__tab--active" : ""}`}
              onClick={() => setActiveTab(tab.id)}
            >
              {tab.label}
            </button>
          ))}
        </div>
        <div
          id="settings-panel-usage"
          role="tabpanel"
          aria-labelledby="settings-tab-usage"
          className="settings-panel"
          hidden={activeTab !== "usage"}
        >
          <section className="settings-section">
            {loading && <p className="settings-section__muted">Loading usage…</p>}
            {error && (
              <p className="settings-section__error" role="alert">
                {error === "usage not available" ? "Usage not available." : error}
              </p>
            )}
            {!loading && !error && usage && (
              <div className="settings-usage">
                {usage.tier && (
                  <p className="settings-usage__row">
                    <span className="settings-usage__label">Tier</span>
                    <span>{usage.tier}</span>
                  </p>
                )}
                <p className="settings-usage__row">
                  <span className="settings-usage__label">Runs</span>
                  <span>
                    {usage.run_count}
                    {usage.max_runs_per_period != null
                      ? ` / ${usage.max_runs_per_period}`
                      : ""}
                  </span>
                </p>
                <p className="settings-usage__row">
                  <span className="settings-usage__label">Tokens</span>
                  <span>
                    {usage.total_tokens.toLocaleString()}
                    {usage.max_tokens_per_period != null
                      ? ` / ${usage.max_tokens_per_period.toLocaleString()}`
                      : ""}
                  </span>
                </p>
                {usage.period_days > 0 && (
                  <p className="settings-usage__row settings-usage__period">
                    Rolling {usage.period_days} days
                  </p>
                )}
              </div>
            )}
          </section>
        </div>
      </div>
    </BaseModal>
  )
}
