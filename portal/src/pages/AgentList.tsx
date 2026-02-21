import { useCallback, useEffect, useState } from "react"
import type { Agent } from "../lib/types"
import { navigate } from "../lib/router"
import { getAgents, createAgent, apiAgentToAgent } from "../lib/api"

interface AgentListProps {
  workspaceId: string
  token: string | null
}

export function AgentList({ workspaceId, token }: AgentListProps) {
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [instructions, setInstructions] = useState("")
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

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!token || !name.trim()) return
    setError(null)
    setCreating(true)
    createAgent(
      workspaceId,
      { name: name.trim(), description: description.trim() || undefined, instructions: instructions.trim() || undefined },
      token
    )
      .then((created) => {
        setAgents((prev) => [...prev, apiAgentToAgent(created)])
        setName("")
        setDescription("")
        setInstructions("")
      })
      .catch((err) => setError(err instanceof Error ? err.message : "Failed to create agent"))
      .finally(() => setCreating(false))
  }

  return (
    <div className="page-activity">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", flexWrap: "wrap", gap: "0.75rem" }}>
        <div>
          <h1 className="page-activity__title">Agents</h1>
          <p className="page-activity__subtitle">
            Create and manage workspace agents (personas / task templates).
          </p>
        </div>
        <button
          type="button"
          className="topbar__workspace-new"
          onClick={() => navigate({ name: "agents", workspaceId })}
        >
          Back to executed tasks
        </button>
      </div>

      <section style={{ marginTop: "1.5rem" }}>
        <h2 style={{ margin: "0 0 0.5rem", fontSize: "1rem" }}>Create agent</h2>
        <form onSubmit={handleSubmit} style={{ maxWidth: "28rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
          <div>
            <label htmlFor="agent-name" style={{ display: "block", marginBottom: "0.25rem", fontSize: "0.9rem" }}>Name *</label>
            <input
              id="agent-name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              placeholder="e.g. Code reviewer"
              style={{ width: "100%", padding: "0.4rem 0.5rem", font: "inherit" }}
            />
          </div>
          <div>
            <label htmlFor="agent-desc" style={{ display: "block", marginBottom: "0.25rem", fontSize: "0.9rem" }}>Description</label>
            <input
              id="agent-desc"
              type="text"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Short description"
              style={{ width: "100%", padding: "0.4rem 0.5rem", font: "inherit" }}
            />
          </div>
          <div>
            <label htmlFor="agent-instructions" style={{ display: "block", marginBottom: "0.25rem", fontSize: "0.9rem" }}>Instructions</label>
            <textarea
              id="agent-instructions"
              value={instructions}
              onChange={(e) => setInstructions(e.target.value)}
              placeholder="System instructions for this agent"
              rows={3}
              style={{ width: "100%", padding: "0.4rem 0.5rem", font: "inherit", resize: "vertical" }}
            />
          </div>
          {error && <p style={{ margin: 0, color: "#c00", fontSize: "0.9rem" }}>{error}</p>}
          <button
            type="submit"
            className="topbar__workspace-new"
            disabled={creating || !name.trim()}
          >
            {creating ? "Creating…" : "Create agent"}
          </button>
        </form>
      </section>

      <section style={{ marginTop: "1.5rem" }}>
        <h2 style={{ margin: "0 0 0.5rem", fontSize: "1rem" }}>Agents in this workspace</h2>
        {loading ? (
          <p className="page-activity__empty">Loading…</p>
        ) : agents.length === 0 ? (
          <p className="page-activity__empty">No agents yet. Create one above.</p>
        ) : (
          <ul className="page-activity__list">
            {agents.map((a) => (
              <li key={a.id} className="page-activity__item">
                <div className="page-activity__content" style={{ padding: "0.5rem 0" }}>
                  <span className="page-activity__task-title">{a.name}</span>
                  {a.description && (
                    <span className="page-activity__meta" style={{ display: "block", marginTop: "0.2rem" }}>
                      {a.description}
                    </span>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
