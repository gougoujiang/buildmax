import { useCallback, useEffect, useMemo, useState } from "react"
import { BaseModal } from "@buildmax/gui"
import { useAuth } from "../contexts/AuthContext"
import { useTeam } from "../contexts/TeamContext"
import { addTeamMember, getTeamMembers, getTeamUsage, removeTeamMember } from "../features/teams/api"
import type { ApiTeamMember, ApiUsage } from "../lib/api/types"
import { getErrorMessage } from "../lib/errorMessage"

function memberDisplayName(member: ApiTeamMember, currentUserId?: string): string {
  if (member.user_id === currentUserId) return "Me"
  if (member.user_name && member.user_name.trim() !== "") return member.user_name
  if (member.user_email && member.user_email.trim() !== "") return member.user_email
  return member.user_id
}

export function TeamSettings() {
  const { token, user } = useAuth()
  const {
    currentTeam,
    currentTeamId,
  } = useTeam()
  const [members, setMembers] = useState<ApiTeamMember[]>([])
  const [usage, setUsage] = useState<ApiUsage | null>(null)
  const [loadingMembers, setLoadingMembers] = useState(false)
  const [loadingUsage, setLoadingUsage] = useState(false)
  const [savingMember, setSavingMember] = useState(false)
  const [removingUserId, setRemovingUserId] = useState<string | null>(null)
  const [addMemberOpen, setAddMemberOpen] = useState(false)
  const [email, setEmail] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [addMemberError, setAddMemberError] = useState<string | null>(null)

  const loadMembers = useCallback(async () => {
    if (!token || !currentTeamId) {
      setMembers([])
      return
    }
    setLoadingMembers(true)
    setError(null)
    try {
      setMembers(await getTeamMembers(currentTeamId, token))
    } catch (err) {
      setError(getErrorMessage(err, "Failed to load team members"))
    } finally {
      setLoadingMembers(false)
    }
  }, [token, currentTeamId])

  const loadUsage = useCallback(async () => {
    if (!token || !currentTeamId) {
      setUsage(null)
      return
    }
    setLoadingUsage(true)
    setError(null)
    try {
      setUsage(await getTeamUsage(currentTeamId, token))
    } catch (err) {
      setError(getErrorMessage(err, "Failed to load team usage"))
    } finally {
      setLoadingUsage(false)
    }
  }, [token, currentTeamId])

  useEffect(() => {
    void loadMembers()
  }, [loadMembers])

  useEffect(() => {
    void loadUsage()
  }, [loadUsage])

  const currentUserMember = useMemo(
    () => members.find((member) => member.user_id === user?.id) ?? null,
    [members, user?.id],
  )
  const currentUserIsOwner = currentUserMember?.role === "owner"
  const isPersonalSpace = Boolean(currentTeam?.personalForUserId)

  async function handleAddMember() {
    if (!token || !currentTeamId || !email.trim() || savingMember) return
    setSavingMember(true)
    setAddMemberError(null)
    try {
      await addTeamMember(currentTeamId, { email: email.trim() }, token)
      setEmail("")
      setAddMemberOpen(false)
      await loadMembers()
    } catch (err) {
      setAddMemberError(getErrorMessage(err, "Failed to add member"))
    } finally {
      setSavingMember(false)
    }
  }

  async function handleRemoveMember(memberUserId: string) {
    if (!token || !currentTeamId || removingUserId) return
    setRemovingUserId(memberUserId)
    setError(null)
    try {
      await removeTeamMember(currentTeamId, memberUserId, token)
      await loadMembers()
    } catch (err) {
      setError(getErrorMessage(err, "Failed to remove member"))
    } finally {
      setRemovingUserId(null)
    }
  }

  return (
    <div className="team-settings-page">
      <div className="page-activity__head">
        <div>
          <h1 className="page-activity__title">
            Team Settings / {currentTeam?.name ?? "Current Team"}
          </h1>
          <p className="page-activity__subtitle">
            Manage members and collaboration access for the current team.
          </p>
        </div>
      </div>

      {error ? (
        <p className="settings-section__error" role="alert">
          {error}
        </p>
      ) : null}

      <div className="team-settings-page__grid">
        <section className="team-settings-page__panel team-settings-page__wide">
          <div className="team-settings-page__section-head">
            <h2 className="issues-page__section-title">Current team</h2>
            <span className="team-settings-page__badge">
              {isPersonalSpace ? "Personal" : "Team"}
            </span>
          </div>
          <dl className="team-settings-page__summary">
            <div>
              <dt>Members</dt>
              <dd>{loadingMembers ? "Loading..." : members.length}</dd>
            </div>
            <div>
              <dt>Your role</dt>
              <dd>{currentUserMember?.role ?? "member"}</dd>
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

        <section className="team-settings-page__panel team-settings-page__wide">
          <div className="team-settings-page__section-head">
            <div>
              <h2 className="issues-page__section-title">Members</h2>
              <p className="team-settings-page__muted">
                Owners can invite teammates and remove members from this team.
              </p>
            </div>
            <div className="team-settings-page__member-head-actions">
              <span className="page-activity__meta">{members.length} members</span>
              {currentUserIsOwner ? (
                <button
                  type="button"
                  className="page-activity__action-btn"
                  onClick={() => {
                    setAddMemberError(null)
                    setAddMemberOpen(true)
                  }}
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
                      {memberDisplayName(member, user?.id)}
                    </span>
                    <span className="team-settings-page__member-meta">
                      {member.user_email ?? member.user_id}
                    </span>
                  </div>
                  <div className="team-settings-page__member-actions">
                    <span className="team-settings-page__role">{member.role}</span>
                    {currentUserIsOwner && member.user_id !== user?.id ? (
                      <button
                        type="button"
                        className="team-settings-page__remove-btn"
                        disabled={removingUserId === member.user_id}
                        onClick={() => void handleRemoveMember(member.user_id)}
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

      </div>
      <BaseModal
        open={addMemberOpen}
        title="Add Member"
        titleId="team-settings-add-member-title"
        onClose={() => {
          if (savingMember) return
          setAddMemberOpen(false)
          setEmail("")
          setAddMemberError(null)
        }}
      >
        <div className="modal__body">
          <div className="team-settings-page__dialog">
            <p className="team-settings-page__muted">
              Invite a teammate to {currentTeam?.name ?? "this team"} by email.
            </p>
            <input
              className="issues-page__input"
              type="email"
              value={email}
              onChange={(e) => {
                setEmail(e.target.value)
                setAddMemberError(null)
              }}
              placeholder="teammate@example.com"
              autoFocus
            />
            {addMemberError ? (
              <p className="modal__error" role="alert">
                {addMemberError}
              </p>
            ) : null}
            <div className="team-settings-page__dialog-actions">
              <button
                type="button"
                className="team-settings-page__secondary-btn"
                disabled={savingMember}
                onClick={() => {
                  setAddMemberOpen(false)
                  setEmail("")
                  setAddMemberError(null)
                }}
              >
                Cancel
              </button>
              <button
                type="button"
                className="page-activity__action-btn"
                disabled={savingMember || !email.trim()}
                onClick={() => void handleAddMember()}
              >
                {savingMember ? "Adding..." : "Add Member"}
              </button>
            </div>
          </div>
        </div>
      </BaseModal>
    </div>
  )
}
