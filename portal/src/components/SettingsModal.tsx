import { useEffect, useState, type ComponentType } from "react"
import { useAuth } from "../contexts/AuthContext"
import type { ApiUsage } from "../lib/api"
import { getUsage } from "../features/usage"
import { BaseModal } from "@buildmax/gui"
import { UserAvatar } from "./UserAvatar"
import SettingsIcon from "../icons/settings.svg?react"
import UsageIcon from "../icons/usage.svg?react"

type SettingsTabId = "general" | "usage"

const SETTINGS_TABS: {
  id: SettingsTabId
  label: string
  icon: ComponentType<{ className?: string }>
}[] = [
  { id: "general", label: "General", icon: SettingsIcon },
  { id: "usage", label: "Usage", icon: UsageIcon },
]

interface SettingsModalProps {
  open: boolean
  onClose: () => void
}

export function SettingsModal({ open, onClose }: SettingsModalProps) {
  const [activeTab, setActiveTab] = useState<SettingsTabId>("general")
  const { token, user } = useAuth()
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
      hideHeader
    >
      <div className="modal__body settings-modal__body">
        <aside className="settings-modal__aside">
          <button
            type="button"
            className="settings-modal__close"
            onClick={onClose}
            aria-label="Close"
          >
            &times;
          </button>
          <nav
            className="settings-modal__nav"
            aria-label="Settings sections"
            role="navigation"
          >
            {SETTINGS_TABS.map((tab) => {
            const Icon = tab.icon
            return (
              <button
                key={tab.id}
                type="button"
                role="tab"
                aria-selected={activeTab === tab.id}
                aria-controls={`settings-panel-${tab.id}`}
                id={`settings-tab-${tab.id}`}
                className={`settings-modal__nav-item ${activeTab === tab.id ? "settings-modal__nav-item--active" : ""}`}
                onClick={() => setActiveTab(tab.id)}
              >
                <span className="settings-modal__nav-icon" aria-hidden>
                  <Icon />
                </span>
                {tab.label}
              </button>
            )
          })}
          </nav>
        </aside>
        <div className="settings-modal__content">
          <div
            id="settings-panel-general"
            role="tabpanel"
            aria-labelledby="settings-tab-general"
            className="settings-panel"
            hidden={activeTab !== "general"}
          >
            <section className="settings-section">
              <h2 className="settings-panel__heading">General</h2>
              <div className="settings-panel__heading-divider" role="separator" />
              {user ? (
                <div className="settings-general">
                  <div className="settings-general__avatar-row">
                    <UserAvatar user={user} size="md" />
                  </div>
                  <dl className="settings-general__fields">
                    <div className="settings-general__field">
                      <dt className="settings-general__label">Name</dt>
                      <dd className="settings-general__value">
                        {user.name?.trim() || (user.email ? user.email.split("@")[0] : "—")}
                      </dd>
                    </div>
                    <div className="settings-general__field">
                      <dt className="settings-general__label">Email</dt>
                      <dd className="settings-general__value">{user.email}</dd>
                    </div>
                  </dl>
                  <div className="settings-general__divider" role="separator" />
                </div>
              ) : (
                <p className="settings-section__muted">Not signed in.</p>
              )}
            </section>
          </div>
          <div
            id="settings-panel-usage"
            role="tabpanel"
            aria-labelledby="settings-tab-usage"
            className="settings-panel"
            hidden={activeTab !== "usage"}
          >
            <section className="settings-section">
              <h2 className="settings-panel__heading">Usage</h2>
              <div className="settings-panel__heading-divider" role="separator" />
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
      </div>
    </BaseModal>
  )
}
