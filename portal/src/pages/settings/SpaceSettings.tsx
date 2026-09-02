import { useEffect, useState } from "react"
import {
  SPACE_NAV,
  SpaceInviteMemberDialog,
  SpaceMembersSection,
  SpaceOverviewSection,
  type SpaceSection,
  useSettingsData,
} from "./shared"
import { navigate } from "../../router"
import { SpaceAuditSection } from "../../features/audit"
import { TeamPlugins } from "../../features/teamPlugins"
import { TeamSandboxDefaults } from "../../features/teamSandbox"
import { TeamSecrets } from "../../features/teamSecrets"
import { useAuth } from "../../contexts/AuthContext"
import { useTeam } from "../../contexts/TeamContext"

export function SpaceSettings({ section }: { section: SpaceSection }) {
  const [inviteOpen, setInviteOpen] = useState(section === "memberNew")
  const {
    user,
    teamUsage,
    members,
    teamUsageLoading,
    membersLoading,
    pageError,
    email,
    inviteRole,
    inviteError,
    savingInvite,
    removingUserId,
    invitations,
    invitationsLoading,
    revokingInvitationId,
    changingRoleUserId,
    roleError,
    issuingLoginCodeUserId,
    issuedLoginCode,
    loginCodeError,
    currentUserMember,
    currentUserIsOwner,
    currentUserRole,
    isPersonalSpace,
    currentTeamName,
    setEmail,
    setInviteRole,
    handleInviteMember,
    handleRevokeInvitation,
    handleRemoveMember,
    handleChangeRole,
    handleTransferOwnership,
    handleIssueLoginCode,
  } = useSettingsData()
  const { token } = useAuth()
  const { currentTeamId } = useTeam()

  useEffect(() => {
    setInviteOpen(section === "memberNew")
    if (section !== "memberNew") {
      setEmail("")
      setInviteRole("member")
    }
  }, [section, setEmail, setInviteRole])

  function closeInviteDialog() {
    setInviteOpen(false)
    setEmail("")
    setInviteRole("member")
    if (section === "memberNew") {
      navigate({ name: "space", section: "members" })
    }
  }

  async function submitInvite() {
    const ok = await handleInviteMember()
    if (ok) {
      setInviteOpen(false)
    }
  }

  return (
    <div className="settings-page">
      <div className="page-activity__head">
        <div>
          <h1 className="page-activity__title">Space</h1>
          <p className="page-activity__subtitle">
            Current space information, quota visibility, and member management.
          </p>
        </div>
      </div>

      {pageError ? (
        <p className="settings-section__error" role="alert">
          {pageError}
        </p>
      ) : null}

      <div className="settings-page__tabs" aria-label="Space sections" role="tablist">
          {SPACE_NAV.map((item) => {
            const Icon = item.icon
            const active = item.id === (section === "memberNew" ? "members" : section)
            return (
              <button
                key={item.id}
                type="button"
                role="tab"
                aria-selected={active}
                className={`settings-page__tab ${active ? "settings-page__tab--active" : ""}`}
                onClick={() => navigate({ name: "space", section: item.id })}
              >
                <span className="settings-page__tab-icon" aria-hidden>
                  <Icon />
                </span>
                <span className="settings-page__tab-label">{item.label}</span>
              </button>
            )
          })}
      </div>

      <div className="settings-page__content">
        {section === "overview" ? (
          <SpaceOverviewSection
            currentTeamName={currentTeamName}
            isPersonalSpace={isPersonalSpace}
            loadingMembers={membersLoading}
            loadingUsage={teamUsageLoading}
            members={members}
            usage={teamUsage}
            currentUserRole={currentUserMember?.role ?? null}
          />
        ) : null}
        {section === "plugins" ? (
          <>
            <TeamPlugins
              token={token}
              teamId={currentTeamId}
              // Changing an activation is owner-or-admin, the authority the
              // team's other shared automation already needs. Reading is not.
              canManage={
                currentUserMember?.role === "owner" || currentUserMember?.role === "admin"
              }
            />
            <TeamSandboxDefaults
              token={token}
              teamId={currentTeamId}
              canManage={
                currentUserMember?.role === "owner" || currentUserMember?.role === "admin"
              }
            />
          </>
        ) : null}
        {section === "secrets" ? (
          <TeamSecrets
            token={token}
            teamId={currentTeamId}
            // Secrets are owner-only: value authority stays with the owner
            // until BuildMax has finer team grants. See
            // docs/design/team-secrets.md §10.
            canManage={currentUserMember?.role === "owner"}
          />
        ) : null}
        {section === "audit" ? (
          <SpaceAuditSection
            teamId={currentTeamId}
            token={token}
            currentUserIsOwner={currentUserIsOwner}
            currentUserId={user?.id}
          />
        ) : null}
        {section === "members" ? (
          <SpaceMembersSection
            currentTeamName={currentTeamName}
            currentUserIsOwner={currentUserIsOwner}
            currentUserRole={currentUserRole}
            loadingMembers={membersLoading}
            members={members}
            userId={user?.id}
            removingUserId={removingUserId}
            onRemoveMember={handleRemoveMember}
            invitations={invitations}
            invitationsLoading={invitationsLoading}
            revokingInvitationId={revokingInvitationId}
            onRevokeInvitation={handleRevokeInvitation}
            changingRoleUserId={changingRoleUserId}
            roleError={roleError}
            onChangeRole={handleChangeRole}
            onTransferOwnership={handleTransferOwnership}
            issuingLoginCodeUserId={issuingLoginCodeUserId}
            issuedLoginCode={issuedLoginCode}
            loginCodeError={loginCodeError}
            onIssueLoginCode={handleIssueLoginCode}
          />
        ) : null}
      </div>
      <SpaceInviteMemberDialog
        open={inviteOpen}
        onClose={closeInviteDialog}
        currentTeamName={currentTeamName}
        currentUserRole={currentUserRole}
        saving={savingInvite}
        email={email}
        role={inviteRole}
        error={inviteError}
        onEmailChange={setEmail}
        onRoleChange={setInviteRole}
        onSubmit={submitInvite}
      />
    </div>
  )
}
