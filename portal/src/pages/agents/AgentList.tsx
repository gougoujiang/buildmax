import { useCallback, useEffect, useState } from "react"
import type { Agent, AgentRevision } from "../../lib/types"
import { navigate } from "../../router"
import { getErrorMessage } from "../../lib/errorMessage"
import { apiAgentToAgent, apiAgentRevisionToAgentRevision } from "../../lib/api/mappers"
import { createConversation } from "../../features/conversations"
import {
  getAgents,
  createAgent,
  updateAgent,
  deleteAgent,
  getAgentRevisions,
  restoreAgentRevision,
} from "../../features/agents"
import { useApp } from "../../contexts/AppContext"
import { AgentAvatar } from "../../components/UserAvatar"
import { CreateAgentModal } from "../../components/CreateAgentModal"
import { EditAgentModal } from "../../components/EditAgentModal"
import { RevisionHistory } from "../../components/RevisionHistory"
import { NewConversationFromAgent } from "../../components/NewConversationFromAgent"
import { useTeam } from "../../contexts/TeamContext"

interface AgentListProps {
  token: string | null
}

export function AgentList({ token }: AgentListProps) {
  const { setPendingConversation } = useApp()
  const { currentTeamId, currentUserRole } = useTeam()
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [modalOpen, setModalOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [editingAgent, setEditingAgent] = useState<Agent | null>(null)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [newTaskAgent, setNewTaskAgent] = useState<Agent | null>(null)
  const [startingTaskAgentId, setStartingTaskAgentId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [revisions, setRevisions] = useState<AgentRevision[]>([])
  const [revisionsLoading, setRevisionsLoading] = useState(false)
  const [revisionsError, setRevisionsError] = useState<string | null>(null)
  const [restoringRevision, setRestoringRevision] = useState<number | null>(null)
  const canManageAgents = currentUserRole === "owner" || currentUserRole === "admin"

  const fetchAgents = useCallback(() => {
    if (!token || !currentTeamId) {
      setAgents([])
      setLoading(false)
      return
    }
    setLoading(true)
    getAgents(currentTeamId, token)
      .then((list) => {
        setAgents(list.map(apiAgentToAgent))
      })
      .finally(() => setLoading(false))
  }, [token, currentTeamId])

  useEffect(() => {
    fetchAgents()
  }, [fetchAgents])

  const loadRevisions = useCallback(
    (agentId: string) => {
      if (!token || !currentTeamId) return
      setRevisionsLoading(true)
      setRevisionsError(null)
      getAgentRevisions(currentTeamId, agentId, token)
        .then((res) => setRevisions(res.revisions.map(apiAgentRevisionToAgentRevision)))
        .catch((err) => setRevisionsError(getErrorMessage(err, "Failed to load history")))
        .finally(() => setRevisionsLoading(false))
    },
    [token, currentTeamId],
  )

  useEffect(() => {
    if (editingAgent == null) {
      setRevisions([])
      setRevisionsError(null)
      return
    }
    loadRevisions(editingAgent.id)
  }, [editingAgent, loadRevisions])

  function handleRestoreRevision(revision: number) {
    if (!token || !currentTeamId || editingAgent == null) return
    setRevisionsError(null)
    setRestoringRevision(revision)
    restoreAgentRevision(currentTeamId, editingAgent.id, revision, token)
      .then((restored) => {
        const mapped = apiAgentToAgent(restored)
        setAgents((prev) => prev.map((a) => (a.id === mapped.id ? mapped : a)))
        setEditingAgent(mapped)
      })
      .catch((err) => setRevisionsError(getErrorMessage(err, "Failed to restore revision")))
      .finally(() => setRestoringRevision(null))
  }

  function handleCreateAgent(values: {
    name: string
    description?: string
    instructions?: string
  }) {
    if (!token || !currentTeamId) return
    setError(null)
    setCreating(true)
    createAgent(currentTeamId, values, token)
      .then((created) => {
        setAgents((prev) => [...prev, apiAgentToAgent(created)])
        setModalOpen(false)
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to create agent")))
      .finally(() => setCreating(false))
  }

  function handleSaveAgent(values: {
    name: string
    description?: string
    instructions?: string
  }) {
    if (!token || !currentTeamId || editingAgent == null) return
    setError(null)
    setSaving(true)
    updateAgent(currentTeamId, editingAgent.id, values, token)
      .then((updated) => {
        setAgents((prev) =>
          prev.map((a) => (a.id === editingAgent.id ? apiAgentToAgent(updated) : a))
        )
        setEditingAgent(null)
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to update agent")))
      .finally(() => setSaving(false))
  }

  function handleDeleteAgent() {
    if (!token || !currentTeamId || editingAgent == null) return
    setError(null)
    setDeleting(true)
    deleteAgent(currentTeamId, editingAgent.id, token)
      .then(() => {
        setAgents((prev) => prev.filter((a) => a.id !== editingAgent.id))
        setEditingAgent(null)
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to delete agent")))
      .finally(() => setDeleting(false))
  }

  function handleOpenNewTaskModal(agent: Agent) {
    setError(null)
    setNewTaskAgent(agent)
  }

  function handleStartTaskFromAgent(editedInput: string) {
    if (!token || !currentTeamId || !newTaskAgent) return
    setError(null)
    setStartingTaskAgentId(newTaskAgent.id)
    createConversation(currentTeamId, { channel: "portal" }, token)
      .then((created) => {
        setNewTaskAgent(null)
        setPendingConversation({
          conversationId: created.conversation_id,
          initialMessage: editedInput,
        })
        navigate({ name: "conversation", conversationId: created.conversation_id })
      })
      .catch((err) => {
        setError(getErrorMessage(err, "Failed to start conversation"))
      })
      .finally(() => setStartingTaskAgentId(null))
  }

  return (
    <div className="page-activity">
      <div className="page-activity__head">
        <div>
          <h1 className="page-activity__title">Agents</h1>
          <p className="page-activity__subtitle">
            Create and manage team agents (personas / task templates).
          </p>
        </div>
        <div className="page-activity__actions">
          {canManageAgents ? (
            <button
              type="button"
              className="page-activity__action-btn agent-list__create-btn"
              onClick={() => {
                setError(null)
                setModalOpen(true)
              }}
              aria-label="Create agent"
            >
              Create agent
            </button>
          ) : null}
        </div>
      </div>

      {!canManageAgents ? (
        <p className="page-activity__empty">
          You can start conversations with team agents, but only team owners and admins can create or edit them.
        </p>
      ) : null}

      <section className="agent-list">
        {loading ? (
          <p className="page-activity__empty">Loading…</p>
        ) : agents.length === 0 ? (
          <p className="page-activity__empty agent-list__empty">
            {canManageAgents
              ? "No agents yet. Click \"Create agent\" to add one."
              : "No agents are available in this team yet. Team owners and admins can add one when you're ready to share a reusable agent."}
          </p>
        ) : (
          <div className="agent-list__grid">
            {agents.map((a) => (
              <article
                key={a.id}
                className="agent-card"
                role="button"
                tabIndex={0}
                aria-label={canManageAgents ? `Edit agent ${a.name}` : `${a.name}`}
                onClick={() => {
                  if (!canManageAgents) return
                  setError(null)
                  setEditingAgent(a)
                }}
                onKeyDown={(e) => {
                  if (!canManageAgents) return
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault()
                    setError(null)
                    setEditingAgent(a)
                  }
                }}
              >
                <header className="agent-card__header">
                  <AgentAvatar size="md" className="agent-card__avatar" />
                  <div className="agent-card__title-row">
                    <h3 className="agent-card__name">{a.name}</h3>
                    {canManageAgents ? <span className="agent-card__edit-hint" aria-hidden>Edit</span> : null}
                  </div>
                </header>
                {a.description ? (
                  <p className="agent-card__description">{a.description}</p>
                ) : null}
                {a.instructions ? (
                  <div className="agent-card__instructions-wrap">
                    <span className="agent-card__instructions-label">Instructions</span>
                    <p className="agent-card__instructions" title={a.instructions}>
                      {a.instructions}
                    </p>
                  </div>
                ) : null}
                <div className="agent-card__actions">
                  <button
                    type="button"
                    className="agent-card__new-task-btn"
                    onClick={(e) => {
                      e.stopPropagation()
                      handleOpenNewTaskModal(a)
                    }}
                    disabled={!token}
                    aria-label={`New conversation with ${a.name}`}
                  >
                    New Conversation
                  </button>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>

      <CreateAgentModal
        open={modalOpen}
        loading={creating}
        error={error}
        onClose={() => {
          setModalOpen(false)
          setError(null)
        }}
        onCreate={handleCreateAgent}
      />

      <EditAgentModal
        open={editingAgent != null}
        agent={editingAgent}
        loading={saving}
        error={error}
        deleting={deleting}
        onClose={() => {
          setEditingAgent(null)
          setError(null)
        }}
        onSave={handleSaveAgent}
        onDelete={handleDeleteAgent}
        history={
          <RevisionHistory
            title="History"
            entries={revisions.map((rev) => ({
              id: rev.id,
              revision: rev.revision,
              createdBy: rev.createdBy,
              createdLabel: rev.createdLabel,
              summary: rev.instructions,
            }))}
            currentRevision={editingAgent?.revision ?? 0}
            loading={revisionsLoading}
            error={revisionsError}
            canRestore={canManageAgents}
            restoringRevision={restoringRevision}
            onRestore={handleRestoreRevision}
          />
        }
      />

      <NewConversationFromAgent
        open={newTaskAgent != null}
        agent={newTaskAgent}
        loading={startingTaskAgentId !== null}
        error={newTaskAgent != null ? error : null}
        onClose={() => {
          setNewTaskAgent(null)
          if (newTaskAgent) setError(null)
        }}
        onStart={handleStartTaskFromAgent}
      />
    </div>
  )
}
