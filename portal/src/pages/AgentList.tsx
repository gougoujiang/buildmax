import { useCallback, useEffect, useState } from "react"
import type { Agent } from "../lib/types"
import { navigate } from "../router"
import { getAgents, createAgent, apiAgentToAgent } from "../lib/api"
import { CreateAgentModal } from "../components/CreateAgentModal"

interface AgentListProps {
  workspaceId: string
  token: string | null
}

export function AgentList({ workspaceId, token }: AgentListProps) {
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [modalOpen, setModalOpen] = useState(false)
  const [creating, setCreating] = useState(false)
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
      .catch((err) =>
        setError(err instanceof Error ? err.message : "Failed to create agent")
      )
      .finally(() => setCreating(false))
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
            className="topbar__workspace-new"
            onClick={() => navigate({ name: "workspace", workspaceId })}
          >
            Back to workspace
          </button>
          <button
            type="button"
            className="topbar__workspace-new agent-list__create-btn"
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
              <article key={a.id} className="agent-card">
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
    </div>
  )
}
