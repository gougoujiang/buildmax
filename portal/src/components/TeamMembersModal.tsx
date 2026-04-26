import { useEffect, useMemo, useState } from "react"
import { BaseModal } from "@buildmax/gui"
import { useAuth } from "../contexts/AuthContext"
import { useTeam } from "../contexts/TeamContext"
import { addTeamMember, createTeam, getTeamMembers, removeTeamMember } from "../features/teams/api"
import type { ApiTeamMember } from "../lib/api/types"
import { getErrorMessage } from "../lib/errorMessage"

interface TeamMembersModalProps {
  open: boolean
  onClose: () => void
}

function memberDisplayName(member: ApiTeamMember, currentUserId?: string): string {
  if (member.user_id === currentUserId) return "Me"
  if (member.user_name && member.user_name.trim() !== "") return member.user_name
  if (member.user_email && member.user_email.trim() !== "") return member.user_email
  return member.user_id
}

export function TeamMembersModal({ open, onClose }: TeamMembersModalProps) {
  const { token, user } = useAuth()
  const { currentTeamId, currentTeam, refetchTeams } = useTeam()
  const [members, setMembers] = useState<ApiTeamMember[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [creatingTeam, setCreatingTeam] = useState(false)
  const [removingUserId, setRemovingUserId] = useState<string | null>(null)
  const [email, setEmail] = useState("")
  const [teamName, setTeamName] = useState("")
  const [error, setError] = useState<string | null>(null)

  async function loadMembers() {
    if (!token || !currentTeamId) {
      setMembers([])
      return
    }
    setLoading(true)
    setError(null)
    try {
      setMembers(await getTeamMembers(currentTeamId, token))
    } catch (err) {
      setError(getErrorMessage(err, "Failed to load members"))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (!open) {
      setEmail("")
      setTeamName("")
      setError(null)
      return
    }
    void loadMembers()
  }, [open, token, currentTeamId])

  const currentUserIsOwner = useMemo(
    () => members.some((member) => member.user_id === user?.id && member.role === "owner"),
    [members, user?.id]
  )

  async function handleAddMember() {
    if (!token || !currentTeamId || !email.trim() || saving) return
    setSaving(true)
    setError(null)
    try {
      await addTeamMember(currentTeamId, { email: email.trim() }, token)
      setEmail("")
      await loadMembers()
    } catch (err) {
      setError(getErrorMessage(err, "Failed to add member"))
    } finally {
      setSaving(false)
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

  async function handleCreateTeam() {
    if (!token || !teamName.trim() || creatingTeam) return
    setCreatingTeam(true)
    setError(null)
    try {
      const created = await createTeam({ name: teamName.trim() }, token)
      setTeamName("")
      await refetchTeams(created.id)
    } catch (err) {
      setError(getErrorMessage(err, "Failed to create team"))
    } finally {
      setCreatingTeam(false)
    }
  }

  return (
    <BaseModal
      open={open}
      title="Members"
      titleId="team-members-modal-title"
      onClose={onClose}
      className="modal--large"
    >
      <div className="modal__body">
        <div className="team-members-modal">
          <div className="team-members-modal__header">
            <h3 className="team-members-modal__title">{currentTeam?.name ?? "Current space"}</h3>
            <p className="team-members-modal__subtitle">
              Manage who can collaborate in this space.
            </p>
          </div>

          <div className="team-members-modal__section">
            <div className="team-members-modal__section-head">
              <span className="team-members-modal__section-title">Members</span>
              <span className="page-activity__meta">{members.length}</span>
            </div>
            {loading ? (
              <p className="page-activity__empty">Loading members…</p>
            ) : members.length === 0 ? (
              <p className="page-activity__empty">No members yet.</p>
            ) : (
              <ul className="team-members-modal__list">
                {members.map((member) => (
                  <li key={member.user_id} className="team-members-modal__item">
                    <div className="team-members-modal__item-main">
                      <span className="team-members-modal__item-name">
                        {memberDisplayName(member, user?.id)}
                      </span>
                      <span className="team-members-modal__item-meta">
                        {member.user_email ?? member.user_id}
                      </span>
                    </div>
                    <div className="team-members-modal__item-actions">
                      <span className="team-members-modal__role">{member.role}</span>
                      {currentUserIsOwner && member.user_id !== user?.id ? (
                        <button
                          type="button"
                          className="team-members-modal__remove-btn"
                          disabled={removingUserId === member.user_id}
                          onClick={() => void handleRemoveMember(member.user_id)}
                        >
                          {removingUserId === member.user_id ? "Removing…" : "Remove"}
                        </button>
                      ) : null}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="team-members-modal__section">
            <div className="team-members-modal__section-head">
              <span className="team-members-modal__section-title">Create a new space</span>
            </div>
            <div className="team-members-modal__invite">
              <input
                className="issues-page__input"
                type="text"
                value={teamName}
                onChange={(e) => setTeamName(e.target.value)}
                placeholder="e.g. Design, Ops, Research"
              />
              <button
                type="button"
                className="page-activity__action-btn"
                disabled={creatingTeam || !teamName.trim()}
                onClick={() => void handleCreateTeam()}
              >
                {creatingTeam ? "Creating…" : "Create space"}
              </button>
            </div>
          </div>

          <div className="team-members-modal__section">
            <div className="team-members-modal__section-head">
              <span className="team-members-modal__section-title">Invite by email</span>
            </div>
            {!currentUserIsOwner && members.length > 0 ? (
              <p className="page-activity__empty">Only owners can add members.</p>
            ) : (
              <div className="team-members-modal__invite">
                <input
                  className="issues-page__input"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="teammate@example.com"
                />
                <button
                  type="button"
                  className="page-activity__action-btn"
                  disabled={saving || !email.trim()}
                  onClick={() => void handleAddMember()}
                >
                  {saving ? "Adding…" : "Add member"}
                </button>
              </div>
            )}
          </div>

          {error ? (
            <p className="modal__error" role="alert">
              {error}
            </p>
          ) : null}
        </div>
      </div>
    </BaseModal>
  )
}
