import { useCallback, useEffect, useState } from "react"
import type { Agent } from "../lib/types"
import { navigate } from "../router"
import { getErrorMessage } from "../lib/errorMessage"
import { apiAgentToAgent } from "../lib/api"
import { createConversation } from "../features/conversations"
import {
  getAgents,
  createAgent,
  updateAgent,
  deleteAgent,
} from "../features/agents"
import { useApp } from "../contexts/AppContext"
import { AgentAvatar } from "../components/UserAvatar"
import { CreateAgentModal } from "../components/CreateAgentModal"
import { EditAgentModal } from "../components/EditAgentModal"
import { NewTaskFromAgentModal } from "../components/NewTaskFromAgentModal"

interface AgentListProps {
  profileId: string
  token: string | null
}

export function AgentList({ profileId, token }: AgentListProps) {
  const { setPendingConversation } = useApp()
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

  const fetchAgents = useCallback(() => {
    if (!token) return
    setLoading(true)
    getAgents(profileId, token)
      .then((list) => setAgents(list.map(apiAgentToAgent)))
      .finally(() => setLoading(false))
  }, [profileId, token])

  useEffect(() => {
    fetchAgents()
  }, [fetchAgents])

  function handleCreateAgent(values: {
    name: string
    description?: string
    instructions?: string
  }) {
    if (!token) return
    setError(null)
    setCreating(true)
    createAgent(profileId, values, token)
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
    if (!token || editingAgent == null) return
    setError(null)
    setSaving(true)
    updateAgent(profileId, editingAgent.id, values, token)
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
    if (!token || editingAgent == null) return
    setError(null)
    setDeleting(true)
    deleteAgent(profileId, editingAgent.id, token)
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
    if (!token || !newTaskAgent) return
    setError(null)
    setStartingTaskAgentId(newTaskAgent.id)
    createConversation(profileId, { channel: "portal" }, token)
      .then((created) => {
        setNewTaskAgent(null)
        setPendingConversation({
          conversationId: created.conversation_id,
          initialMessage: editedInput,
        })
        navigate({ name: "conversation", profileId, conversationId: created.conversation_id })
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
            Create and manage personal agents (personas / task templates).
          </p>
        </div>
        <div className="page-activity__actions">
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
        </div>
      </div>

      <section className="agent-list">
        {loading ? (
          <p className="page-activity__empty">Loading…</p>
        ) : agents.length === 0 ? (
          <p className="page-activity__empty agent-list__empty">
            No agents yet. Click &quot;Create agent&quot; to add one.
          </p>
        ) : (
          <div className="agent-list__grid">
            {agents.map((a) => (
              <article
                key={a.id}
                className="agent-card"
                role="button"
                tabIndex={0}
                aria-label={`Edit agent ${a.name}`}
                onClick={() => {
                  setError(null)
                  setEditingAgent(a)
                }}
                onKeyDown={(e) => {
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
                    <span className="agent-card__edit-hint" aria-hidden>Edit</span>
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
                    aria-label={`New task with ${a.name}`}
                  >
                    New task
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
      />

      <NewTaskFromAgentModal
        open={newTaskAgent != null}
        agent={newTaskAgent}
        loading={startingTaskAgentId !== null}
        error={newTaskAgent != null ? error : null}
        onClose={() => {
          setNewTaskAgent(null)
          if (newTaskAgent) setError(null)
        }}
        onStartTask={handleStartTaskFromAgent}
      />
    </div>
  )
}
