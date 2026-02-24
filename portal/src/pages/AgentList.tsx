import { useCallback, useEffect, useState } from "react"
import type { Agent } from "../lib/types"
import { navigate } from "../router"
import { getErrorMessage } from "../lib/errorMessage"
import {
  getAgents,
  createAgent,
  updateAgent,
  deleteAgent,
  apiAgentToAgent,
} from "../lib/api"
import { CreateAgentModal } from "../components/CreateAgentModal"
import { EditAgentModal } from "../components/EditAgentModal"

interface AgentListProps {
  workspaceId: string
  token: string | null
}

export function AgentList({ workspaceId, token }: AgentListProps) {
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [modalOpen, setModalOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [editingAgent, setEditingAgent] = useState<Agent | null>(null)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
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
                <h3 className="agent-card__name">{a.name}</h3>
                {a.description ? (
                  <p className="agent-card__description">{a.description}</p>
                ) : null}
                {a.instructions ? (
                  <p className="agent-card__instructions" title={a.instructions}>
                    {a.instructions}
                  </p>
                ) : null}
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
    </div>
  )
}
