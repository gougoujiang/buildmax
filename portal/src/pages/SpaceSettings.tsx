import { useEffect, useState } from "react"
import {
  SPACE_NAV,
  SpaceAddMemberDialog,
  SpaceMembersSection,
  SpaceOverviewSection,
  type SpaceSection,
  useSettingsData,
} from "./settings/shared"
import { navigate } from "../router"

export function SpaceSettings({ section }: { section: SpaceSection }) {
  const [addMemberOpen, setAddMemberOpen] = useState(section === "memberNew")
  const {
    user,
    teamUsage,
    members,
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
  } = useSettingsData()

  useEffect(() => {
    setAddMemberOpen(section === "memberNew")
    if (section !== "memberNew") {
      setEmail("")
    }
  }, [section, setEmail])

  function closeAddMemberDialog() {
    setAddMemberOpen(false)
    setEmail("")
    if (section === "memberNew") {
      navigate({ name: "space", section: "members" })
    }
  }

  async function submitAddMember() {
    const ok = await handleAddMember()
    if (ok) {
      setAddMemberOpen(false)
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
        {section === "members" ? (
          <SpaceMembersSection
            currentTeamName={currentTeamName}
            currentUserIsOwner={currentUserIsOwner}
            loadingMembers={membersLoading}
            members={members}
            userId={user?.id}
            removingUserId={removingUserId}
            onRemoveMember={handleRemoveMember}
          />
        ) : null}
      </div>
      <SpaceAddMemberDialog
        open={addMemberOpen}
        onClose={closeAddMemberDialog}
        currentTeamName={currentTeamName}
        currentUserIsOwner={currentUserIsOwner}
        saving={savingMember}
        email={email}
        error={addMemberError}
        onEmailChange={setEmail}
        onSubmit={submitAddMember}
      />
    </div>
  )
}
