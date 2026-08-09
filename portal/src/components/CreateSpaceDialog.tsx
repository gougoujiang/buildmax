import { useState } from "react"
import { BaseModal } from "@buildmax/gui"
import { useAuth } from "../contexts/AuthContext"
import { useTeam } from "../contexts/TeamContext"
import { createTeam } from "../features/teams/api"
import { getErrorMessage } from "../lib/errorMessage"

interface CreateSpaceDialogProps {
  open: boolean
  onClose: () => void
}

export function CreateSpaceDialog({ open, onClose }: CreateSpaceDialogProps) {
  const { token } = useAuth()
  const { refetchTeams } = useTeam()
  const [teamName, setTeamName] = useState("")
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleCreate() {
    if (!token || !teamName.trim() || creating) return
    setCreating(true)
    setError(null)
    try {
      const created = await createTeam({ name: teamName.trim() }, token)
      setTeamName("")
      await refetchTeams(created.id)
      onClose()
    } catch (err) {
      setError(getErrorMessage(err, "Failed to create space"))
    } finally {
      setCreating(false)
    }
  }

  return (
    <BaseModal
      open={open}
      title="Create Space"
      titleId="create-space-dialog-title"
      onClose={() => {
        if (creating) return
        setTeamName("")
        setError(null)
        onClose()
      }}
    >
      <div className="modal__body">
        <div className="team-settings-page__dialog">
          <p className="team-settings-page__muted">
            Create a new shared space for agents, workflows, issues, and conversations.
          </p>
          <input
            className="issues-page__input"
            type="text"
            value={teamName}
            onChange={(e) => setTeamName(e.target.value)}
            placeholder="e.g. Design, Ops, Research"
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
              disabled={creating}
              onClick={() => {
                setTeamName("")
                setError(null)
                onClose()
              }}
            >
              Cancel
            </button>
            <button
              type="button"
              className="page-activity__action-btn"
              disabled={creating || !teamName.trim()}
              onClick={() => void handleCreate()}
            >
              {creating ? "Creating..." : "Create Space"}
            </button>
          </div>
        </div>
      </div>
    </BaseModal>
  )
}
