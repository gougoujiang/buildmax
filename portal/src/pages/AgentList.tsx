import { useCallback, useEffect, useState } from "react"
import type { Agent } from "../lib/types"
import { navigate } from "../router"
import { getErrorMessage } from "../lib/errorMessage"
import {
  getAgents,
  createAgent,
  updateAgent,
  deleteAgent,
  createChat,
  apiAgentToAgent,
  apiChatToChat,
} from "../lib/api"
import { useWorkspace } from "../contexts/WorkspaceContext"
import { AgentAvatar } from "../components/UserAvatar"
import { CreateAgentModal } from "../components/CreateAgentModal"
import { EditAgentModal } from "../components/EditAgentModal"
import { NewChatFromAgentModal } from "../components/NewChatFromAgentModal"

interface AgentListProps {
  workspaceId: string
  token: string | null
}

export function AgentList({ workspaceId, token }: AgentListProps) {
  const { refetchWorkspaceChats, setPendingChat } = useWorkspace()
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [modalOpen, setModalOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [editingAgent, setEditingAgent] = useState<Agent | null>(null)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [newChatAgent, setNewChatAgent] = useState<Agent | null>(null)
  const [startingChatAgentId, setStartingChatAgentId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const fetchAgents = useCallback(() => {
    if (!token) return
    setLoading(true)
    getAgents(workspaceId, token)
      .then((list) => setAgents(list.map(apiAgentToAgent)))
      .finally(() => setLoading(false))
  }, [workspaceId, token])

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
    createAgent(workspaceId, values, token)
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
    updateAgent(workspaceId, editingAgent.id, values, token)
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
    deleteAgent(workspaceId, editingAgent.id, token)
      .then(() => {
        setAgents((prev) => prev.filter((a) => a.id !== editingAgent.id))
        setEditingAgent(null)
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to delete agent")))
      .finally(() => setDeleting(false))
  }

  function handleOpenNewChatModal(agent: Agent) {
    setError(null)
    setNewChatAgent(agent)
  }

  function handleStartChatFromAgent(editedInput: string) {
    if (!token || !newChatAgent) return
    setError(null)
    setStartingChatAgentId(newChatAgent.id)
    createChat(workspaceId, { agent_id: newChatAgent.id, input: editedInput }, token)
      .then((chat) => {
        setNewChatAgent(null)
        setPendingChat({ chat: apiChatToChat(chat), initialInput: "" })
        navigate({ name: "chat", workspaceId, chatId: chat.id })
        refetchWorkspaceChats()
      })
      .catch((err) => {
        setError(getErrorMessage(err, "Failed to start chat"))
      })
      .finally(() => setStartingChatAgentId(null))
  }

  return (
    <div className="page-activity">
      <div className="page-activity__head">
        <div>
          <h1 className="page-activity__title">Agents</h1>
          <p className="page-activity__subtitle">
            Create and manage workspace agents (personas / task templates).
          </p>
        </div>
        <div className="page-activity__actions">
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => navigate({ name: "workspace", workspaceId })}
          >
            Back to workspace
          </button>
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
                    className="agent-card__new-chat-btn"
                    onClick={(e) => {
                      e.stopPropagation()
                      handleOpenNewChatModal(a)
                    }}
                    disabled={!token}
                    aria-label={`New chat with ${a.name}`}
                  >
                    New chat
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

      <NewChatFromAgentModal
        open={newChatAgent != null}
        agent={newChatAgent}
        loading={startingChatAgentId !== null}
        error={newChatAgent != null ? error : null}
        onClose={() => {
          setNewChatAgent(null)
          if (newChatAgent) setError(null)
        }}
        onStartChat={handleStartChatFromAgent}
      />
    </div>
  )
}
