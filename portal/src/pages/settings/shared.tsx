import { useCallback, useEffect, useMemo, useState, type ComponentType } from "react"
import type { ApiTeamMember, ApiUsage } from "../../lib/api/types"
import type { LoginUser } from "../../lib/api"
import { useAuth } from "../../contexts/AuthContext"
import { useTeam } from "../../contexts/TeamContext"
import { getUsage } from "../../features/usage"
import { addTeamMember, getTeamMembers, getTeamUsage, removeTeamMember } from "../../features/teams/api"
import { getErrorMessage } from "../../lib/errorMessage"
import { navigate } from "../../router"
import { UserAvatar } from "../../components/UserAvatar"
import { WebhookKeysSection } from "../../components/WebhookKeysSection"
import SettingsIcon from "../../icons/settings.svg?react"
import UsageIcon from "../../icons/usage.svg?react"
import ToolboxIcon from "../../icons/toolbox.svg?react"
import AgentsIcon from "../../icons/agents.svg?react"
import IssueIcon from "../../icons/issue.svg?react"
import { BaseModal } from "@buildmax/gui"

export type AccountSection = "general" | "usage" | "webhook"
export type SpaceSection = "overview" | "members" | "memberNew"

interface SettingsNavItem<T extends string> {
  id: T
  label: string
  icon: ComponentType<{ className?: string }>
}

export const ACCOUNT_NAV: SettingsNavItem<Exclude<AccountSection, never>>[] = [
  { id: "general", label: "General", icon: SettingsIcon },
  { id: "usage", label: "Usage", icon: UsageIcon },
  { id: "webhook", label: "Webhook", icon: ToolboxIcon },
]

export const SPACE_NAV: SettingsNavItem<Exclude<SpaceSection, "memberNew">>[] = [
  { id: "overview", label: "Overview", icon: IssueIcon },
  { id: "members", label: "Members", icon: AgentsIcon },
]

function memberDisplayName(member: ApiTeamMember, currentUserId?: string): string {
  if (member.user_id === currentUserId) return "Me"
  if (member.user_name && member.user_name.trim() !== "") return member.user_name
  if (member.user_email && member.user_email.trim() !== "") return member.user_email
  return member.user_id
}

export function SettingsGeneralSection({ user }: { user: LoginUser | null }) {
  return (
    <section className="settings-page__section">
      <div className="settings-page__section-head">
        <div>
          <h2 className="settings-page__section-title">General</h2>
          <p className="settings-page__section-copy">
            Account details for the currently signed-in user.
          </p>
        </div>
      </div>
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
        </div>
      ) : (
        <p className="settings-section__muted">Not signed in.</p>
      )}
    </section>
  )
}

export function SettingsUsageSection({
  loading,
  error,
  usage,
}: {
  loading: boolean
  error: string | null
  usage: ApiUsage | null
}) {
  return (
    <section className="settings-page__section">
      <div className="settings-page__section-head">
        <div>
          <h2 className="settings-page__section-title">Usage</h2>
          <p className="settings-page__section-copy">
            Personal usage and plan limits for your account.
          </p>
        </div>
      </div>
      {loading && <p className="settings-section__muted">Loading usage…</p>}
      {error ? (
        <p className="settings-section__error" role="alert">
          {error === "usage not available" ? "Usage not available." : error}
        </p>
      ) : null}
      {!loading && !error && usage ? (
        <div className="settings-usage">
          {usage.tier ? (
            <p className="settings-usage__row">
              <span className="settings-usage__label">Tier</span>
              <span>{usage.tier}</span>
            </p>
          ) : null}
          <p className="settings-usage__row">
            <span className="settings-usage__label">Runs</span>
            <span>
              {usage.run_count}
              {usage.max_runs_per_period != null ? ` / ${usage.max_runs_per_period}` : ""}
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
          {usage.period_days > 0 ? (
            <p className="settings-usage__row settings-usage__period">
              Rolling {usage.period_days} days
            </p>
          ) : null}
        </div>
      ) : null}
    </section>
  )
}

export function AccountWebhookSection({ token }: { token: string | null }) {
  return (
    <section className="settings-page__section">
      <div className="settings-page__section-head">
        <div>
          <h2 className="settings-page__section-title">Webhook</h2>
          <p className="settings-page__section-copy">
            Manage API keys for incoming automation triggers.
          </p>
        </div>
      </div>
      <WebhookKeysSection token={token} />
    </section>
  )
}

export function SpaceOverviewSection({
  currentTeamName,
  isPersonalSpace,
  loadingMembers,
  loadingUsage,
  members,
  usage,
  currentUserRole,
}: {
  currentTeamName: string
  isPersonalSpace: boolean
  loadingMembers: boolean
  loadingUsage: boolean
  members: ApiTeamMember[]
  usage: ApiUsage | null
  currentUserRole: string | null
}) {
  return (
    <section className="settings-page__section">
      <div className="settings-page__section-head">
        <div>
          <h2 className="settings-page__section-title">Space</h2>
          <p className="settings-page__section-copy">
            Overview and quota details for the current shared workspace.
          </p>
        </div>
        <span className="team-settings-page__badge">{isPersonalSpace ? "Personal" : "Team"}</span>
      </div>
      <dl className="team-settings-page__summary">
        <div>
          <dt>Space name</dt>
          <dd>{currentTeamName}</dd>
        </div>
        <div>
          <dt>Members</dt>
          <dd>{loadingMembers ? "Loading..." : members.length}</dd>
        </div>
        <div>
          <dt>Your role</dt>
          <dd>{currentUserRole ?? "member"}</dd>
        </div>
        <div>
          <dt>Quota tier</dt>
          <dd>{loadingUsage ? "Loading..." : usage?.tier ?? "Unavailable"}</dd>
        </div>
        <div>
          <dt>Runs this period</dt>
          <dd>
            {loadingUsage
              ? "Loading..."
              : usage?.max_runs_per_period != null
                ? `${usage.run_count} / ${usage.max_runs_per_period}`
                : (usage?.run_count ?? "Unavailable")}
          </dd>
        </div>
        <div>
          <dt>Tokens this period</dt>
          <dd>
            {loadingUsage
              ? "Loading..."
              : usage?.max_tokens_per_period != null
                ? `${usage.total_tokens.toLocaleString()} / ${usage.max_tokens_per_period.toLocaleString()}`
                : (usage?.total_tokens != null ? usage.total_tokens.toLocaleString() : "Unavailable")}
          </dd>
        </div>
      </dl>
      {usage ? (
        <p className="team-settings-page__muted">
          Current usage window: last {usage.period_days} days.
        </p>
      ) : null}
    </section>
  )
}

export function SpaceMembersSection({
  currentTeamName,
  currentUserIsOwner,
  loadingMembers,
  members,
  userId,
  removingUserId,
  onRemoveMember,
}: {
  currentTeamName: string
  currentUserIsOwner: boolean
  loadingMembers: boolean
  members: ApiTeamMember[]
  userId?: string
  removingUserId: string | null
  onRemoveMember: (memberUserId: string) => Promise<void>
}) {
  return (
    <section className="settings-page__section">
      <div className="settings-page__section-head">
        <div>
          <h2 className="settings-page__section-title">Members</h2>
          <p className="settings-page__section-copy">
            Owners can invite teammates and manage who has access to {currentTeamName}.
          </p>
        </div>
        <div className="team-settings-page__member-head-actions">
          <span className="page-activity__meta">{members.length} members</span>
          {currentUserIsOwner ? (
            <button
              type="button"
              className="page-activity__action-btn"
              onClick={() => navigate({ name: "space", section: "memberNew" })}
            >
              Add Member
            </button>
          ) : null}
        </div>
      </div>

      {loadingMembers ? (
        <p className="page-activity__empty">Loading members...</p>
      ) : members.length === 0 ? (
        <p className="page-activity__empty">No members yet.</p>
      ) : (
        <ul className="team-settings-page__member-list">
          {members.map((member) => (
            <li key={member.user_id} className="team-settings-page__member">
              <div className="team-settings-page__member-main">
                <span className="team-settings-page__member-name">
                  {memberDisplayName(member, userId)}
                </span>
                <span className="team-settings-page__member-meta">
                  {member.user_email ?? member.user_id}
                </span>
              </div>
              <div className="team-settings-page__member-actions">
                <span className="team-settings-page__role">{member.role}</span>
                {currentUserIsOwner && member.user_id !== userId ? (
                  <button
                    type="button"
                    className="team-settings-page__remove-btn"
                    disabled={removingUserId === member.user_id}
                    onClick={() => void onRemoveMember(member.user_id)}
                  >
                    {removingUserId === member.user_id ? "Removing..." : "Remove"}
                  </button>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

export function SpaceAddMemberDialog({
  open,
  onClose,
  currentTeamName,
  currentUserIsOwner,
  saving,
  email,
  error,
  onEmailChange,
  onSubmit,
}: {
  open: boolean
  onClose: () => void
  currentTeamName: string
  currentUserIsOwner: boolean
  saving: boolean
  email: string
  error: string | null
  onEmailChange: (value: string) => void
  onSubmit: () => Promise<void>
}) {
  if (!currentUserIsOwner) {
    return null
  }

  return (
    <BaseModal
      open={open}
      title="Add Member"
      titleId="space-add-member-dialog-title"
      onClose={() => {
        if (saving) return
        onClose()
      }}
    >
      <div className="modal__body">
        <div className="team-settings-page__dialog">
          <p className="team-settings-page__muted">
            Invite a teammate to {currentTeamName} by email.
          </p>
          <label className="settings-page__field-label" htmlFor="settings-member-email">
            Teammate email
          </label>
          <input
            id="settings-member-email"
            className="issues-page__input"
            type="email"
            value={email}
            onChange={(e) => onEmailChange(e.target.value)}
            placeholder="teammate@example.com"
            autoFocus
          />
          {error ? (
            <p className="modal__error" role="alert">
              {error}
            </p>
          ) : null}
          <div className="team-settings-page__dialog-actions">
            <button
              type="button"
              className="team-settings-page__secondary-btn"
              disabled={saving}
              onClick={onClose}
            >
              Cancel
            </button>
            <button
              type="button"
              className="page-activity__action-btn"
              disabled={saving || !email.trim()}
              onClick={() => void onSubmit()}
            >
              {saving ? "Adding..." : "Add Member"}
            </button>
          </div>
        </div>
      </div>
    </BaseModal>
  )
}

export function useSettingsData() {
  const { token, user } = useAuth()
  const { currentTeam, currentTeamId } = useTeam()
  const [usage, setUsage] = useState<ApiUsage | null>(null)
  const [teamUsage, setTeamUsage] = useState<ApiUsage | null>(null)
  const [members, setMembers] = useState<ApiTeamMember[]>([])
  const [usageLoading, setUsageLoading] = useState(false)
  const [teamUsageLoading, setTeamUsageLoading] = useState(false)
  const [membersLoading, setMembersLoading] = useState(false)
  const [pageError, setPageError] = useState<string | null>(null)
  const [email, setEmail] = useState("")
  const [addMemberError, setAddMemberError] = useState<string | null>(null)
  const [savingMember, setSavingMember] = useState(false)
  const [removingUserId, setRemovingUserId] = useState<string | null>(null)

  const loadMembers = useCallback(async () => {
    if (!token || !currentTeamId) {
      setMembers([])
      return
    }
    setMembersLoading(true)
    setPageError(null)
    try {
      setMembers(await getTeamMembers(currentTeamId, token))
    } catch (err) {
      setPageError(getErrorMessage(err, "Failed to load team members"))
    } finally {
      setMembersLoading(false)
    }
  }, [token, currentTeamId])

  const loadTeamUsage = useCallback(async () => {
    if (!token || !currentTeamId) {
      setTeamUsage(null)
      return
    }
    setTeamUsageLoading(true)
    setPageError(null)
    try {
      setTeamUsage(await getTeamUsage(currentTeamId, token))
    } catch (err) {
      setPageError(getErrorMessage(err, "Failed to load team usage"))
    } finally {
      setTeamUsageLoading(false)
    }
  }, [token, currentTeamId])

  useEffect(() => {
    if (!token) {
      setUsage(null)
      return
    }
    setUsageLoading(true)
    getUsage(token)
      .then((data) => {
        setUsage(data)
      })
      .catch((err) => {
        setPageError(getErrorMessage(err, "Failed to load usage"))
      })
      .finally(() => {
        setUsageLoading(false)
      })
  }, [token])

  useEffect(() => {
    void loadMembers()
  }, [loadMembers])

  useEffect(() => {
    void loadTeamUsage()
  }, [loadTeamUsage])

  const currentUserMember = useMemo(
    () => members.find((member) => member.user_id === user?.id) ?? null,
    [members, user?.id],
  )
  const currentUserIsOwner = currentUserMember?.role === "owner"
  const isPersonalSpace = Boolean(currentTeam?.personalForUserId)
  const currentTeamName = currentTeam?.name ?? "Current Space"

  async function handleAddMember(): Promise<boolean> {
    if (!token || !currentTeamId || !email.trim() || savingMember) return false
    setSavingMember(true)
    setAddMemberError(null)
    try {
      await addTeamMember(currentTeamId, { email: email.trim() }, token)
      setEmail("")
      await loadMembers()
      navigate({ name: "space", section: "members" })
      return true
    } catch (err) {
      setAddMemberError(getErrorMessage(err, "Failed to add member"))
      return false
    } finally {
      setSavingMember(false)
    }
  }

  async function handleRemoveMember(memberUserId: string) {
    if (!token || !currentTeamId || removingUserId) return
    setRemovingUserId(memberUserId)
    setPageError(null)
    try {
      await removeTeamMember(currentTeamId, memberUserId, token)
      await loadMembers()
    } catch (err) {
      setPageError(getErrorMessage(err, "Failed to remove member"))
    } finally {
      setRemovingUserId(null)
    }
  }

  return {
    token,
    user,
    usage,
    teamUsage,
    members,
    usageLoading,
    teamUsageLoading,
    membersLoading,
    pageError,
    email,
    addMemberError,
    savingMember,
    removingUserId,
    currentUserMember,
    currentUserIsOwner,
    isPersonalSpace,
    currentTeamName,
    setEmail,
    handleAddMember,
    handleRemoveMember,
  }
}
