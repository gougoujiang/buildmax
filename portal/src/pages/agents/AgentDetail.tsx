import { useCallback, useEffect, useMemo, useState } from "react"
import type { Agent, AgentRevision } from "../../lib/types"
import type { ApiSecret, ApiTask } from "../../lib/api/types"
import { navigate } from "../../router"
import { getErrorMessage } from "../../lib/errorMessage"
import { apiAgentToAgent, apiAgentRevisionToAgentRevision, apiTaskToTask } from "../../lib/api/mappers"
import {
  getAgent,
  updateAgent,
  deleteAgent,
  getAgentRevisions,
  restoreAgentRevision,
  listAgentModels,
  type AgentDefinitionInput,
} from "../../features/agents"
import { createAgentTask, listAgentTasks } from "../../features/tasks"
import { listSecrets } from "../../features/teamSecrets/api"
import { listActivations } from "../../features/teamPlugins/api"
import { listPlugins } from "../../features/plugins/api"
import { nameablePlugins } from "../../features/plugins/nameablePlugins"
import { runStatusLabel, runStatusTone, taskRunFailed, taskRunFinished } from "../../features/conversations/thread"
import { AgentAvatar } from "../../components/UserAvatar"
import { AgentConfigForm } from "../../components/AgentConfigForm"
import { RevisionHistory } from "../../components/RevisionHistory"
import { RunAgentModal } from "../../components/RunAgentModal"
import { consumptionHealthCount } from "../../components/SecretConsumptionEditor"
import { useApp } from "../../contexts/AppContext"
import { useTeam } from "../../contexts/TeamContext"

interface AgentDetailProps {
  token: string | null
  agentId: string
}

type Tab = "overview" | "config" | "runs" | "revisions"

const TABS: { id: Tab; label: string }[] = [
  { id: "overview", label: "Overview" },
  { id: "config", label: "Configuration" },
  { id: "runs", label: "Runs" },
  { id: "revisions", label: "Revisions" },
]

export function AgentDetail({ token, agentId }: AgentDetailProps) {
  const { currentTeamId, currentUserRole } = useTeam()
  const { setEntityLabel } = useApp()
  const canManage = currentUserRole === "owner" || currentUserRole === "admin"

  const [agent, setAgent] = useState<Agent | null>(null)
  const [secrets, setSecrets] = useState<ApiSecret[]>([])
  const [availablePlugins, setAvailablePlugins] = useState<string[]>([])
  const [availableModels, setAvailableModels] = useState<string[]>([])
  const [tasks, setTasks] = useState<ApiTask[]>([])
  const [revisions, setRevisions] = useState<AgentRevision[]>([])
  const [tab, setTab] = useState<Tab>("overview")

  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [runOpen, setRunOpen] = useState(false)
  const [starting, setStarting] = useState(false)
  const [runError, setRunError] = useState<string | null>(null)
  const [revisionsLoading, setRevisionsLoading] = useState(false)
  const [revisionsError, setRevisionsError] = useState<string | null>(null)
  const [restoringRevision, setRestoringRevision] = useState<number | null>(null)

  const load = useCallback(async () => {
    if (!token || !currentTeamId) {
      setAgent(null)
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    try {
      const [agentApi, tasksApi, revisionsApi] = await Promise.all([
        getAgent(currentTeamId, agentId, token),
        listAgentTasks(currentTeamId, agentId, token),
        getAgentRevisions(currentTeamId, agentId, token),
      ])
      setAgent(apiAgentToAgent(agentApi))
      setTasks(tasksApi.tasks)
      setRevisions(revisionsApi.revisions.map(apiAgentRevisionToAgentRevision))
    } catch (err) {
      setError(getErrorMessage(err, "Failed to load agent"))
    } finally {
      setLoading(false)
    }
  }, [token, currentTeamId, agentId])

  useEffect(() => {
    void load()
  }, [load])

  // Publish the loaded name so the breadcrumb reads "Agents / <name>" instead of
  // the opaque id, and updates in place after a rename.
  useEffect(() => {
    if (agent) setEntityLabel(agent.id, agent.name)
  }, [agent, setEntityLabel])

  // Secrets and the nameable plugin set feed the config editor and the secret
  // health check. Only owners/admins may list them; a member gets empty options
  // rather than a blocked page.
  useEffect(() => {
    if (!token || !currentTeamId || !canManage) {
      setSecrets([])
      setAvailablePlugins([])
      return
    }
    listSecrets(token, currentTeamId)
      .then((res) => setSecrets(res.secrets ?? []))
      .catch(() => setSecrets([]))
    Promise.all([
      listActivations(token, currentTeamId).catch(() => null),
      listPlugins(token).catch(() => null),
    ])
      .then(([activations, catalog]) =>
        setAvailablePlugins(nameablePlugins(activations, catalog?.plugins ?? null)),
      )
      .catch(() => setAvailablePlugins([]))
    // The model catalog is deployment-wide, so it is fetched independently of
    // the team-scoped plugin and secret options; an empty list leaves the
    // picker at just the deployment default.
    listAgentModels(token)
      .then(setAvailableModels)
      .catch(() => setAvailableModels([]))
  }, [token, currentTeamId, canManage])

  const loadRevisions = useCallback(() => {
    if (!token || !currentTeamId) return
    setRevisionsLoading(true)
    setRevisionsError(null)
    getAgentRevisions(currentTeamId, agentId, token)
      .then((res) => setRevisions(res.revisions.map(apiAgentRevisionToAgentRevision)))
      .catch((err) => setRevisionsError(getErrorMessage(err, "Failed to load history")))
      .finally(() => setRevisionsLoading(false))
  }, [token, currentTeamId, agentId])

  function handleSave(definition: AgentDefinitionInput) {
    if (!token || !currentTeamId || !agent) return
    setSaving(true)
    setSaveError(null)
    updateAgent(currentTeamId, agent.id, definition, token)
      .then((updated) => {
        setAgent(apiAgentToAgent(updated))
        loadRevisions()
      })
      .catch((err) => setSaveError(getErrorMessage(err, "Failed to update agent")))
      .finally(() => setSaving(false))
  }

  function handleDelete() {
    if (!token || !currentTeamId || !agent) return
    setDeleting(true)
    setSaveError(null)
    deleteAgent(currentTeamId, agent.id, token)
      .then(() => navigate({ name: "agents" }))
      .catch((err) => {
        setSaveError(getErrorMessage(err, "Failed to delete agent"))
        setDeleting(false)
      })
  }

  function handleRestoreRevision(revision: number) {
    if (!token || !currentTeamId || !agent) return
    setRevisionsError(null)
    setRestoringRevision(revision)
    restoreAgentRevision(currentTeamId, agent.id, revision, token)
      .then((restored) => {
        setAgent(apiAgentToAgent(restored))
        loadRevisions()
      })
      .catch((err) => setRevisionsError(getErrorMessage(err, "Failed to restore revision")))
      .finally(() => setRestoringRevision(null))
  }

  function handleStartRun(input: string) {
    if (!token || !currentTeamId || !agent) return
    setStarting(true)
    setRunError(null)
    createAgentTask(currentTeamId, agent.id, input, token)
      .then((created) => {
        setRunOpen(false)
        navigate({ name: "task", taskId: created.id })
      })
      .catch((err) => setRunError(getErrorMessage(err, "Failed to run agent")))
      .finally(() => setStarting(false))
  }

  const stats = useMemo(() => {
    const finished = tasks.filter((t) => taskRunFinished(t.status))
    const failed = finished.filter((t) => taskRunFailed(t.status)).length
    const succeeded = finished.length - failed
    const running = tasks.some((t) => !taskRunFinished(t.status))
    const successRate = finished.length > 0 ? `${Math.round((succeeded / finished.length) * 100)}%` : "—"
    return { total: tasks.length, successRate, running, lastRun: tasks[0] ? apiTaskToTask(tasks[0]).timeLabel : "—" }
  }, [tasks])

  const secretWarnings = agent ? consumptionHealthCount(agent.secretConsumption, secrets) : 0

  function renderRunsTable(rows: ApiTask[]) {
    if (rows.length === 0) return <p className="page-activity__empty">No executions yet.</p>
    return (
      <table className="agent-runs">
        <thead>
          <tr>
            <th>Task</th>
            <th>Status</th>
            <th>When</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((t) => {
            const ui = apiTaskToTask(t)
            const tone = runStatusTone(t.status)
            return (
              <tr key={t.id} onClick={() => navigate({ name: "task", taskId: t.id })} tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === "Enter") navigate({ name: "task", taskId: t.id })
                }}>
                <td className="agent-runs__title">{ui.title}</td>
                <td>
                  <span className={`agent-runs__status agent-runs__status--${tone}`}>{runStatusLabel(t.status)}</span>
                </td>
                <td className="agent-runs__when">{ui.timeLabel}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
    )
  }

  return (
    <div className="page-activity">
      <div className="page-activity__head">
        <div className="agent-detail__ident">
          <AgentAvatar size="md" className="agent-detail__avatar" />
          <div>
            <h1 className="page-activity__title">
              {agent?.name ?? "Agent"}
              {stats.running ? <span className="agent-detail__running">running</span> : null}
            </h1>
            {agent?.description ? <p className="agent-detail__desc">{agent.description}</p> : null}
          </div>
        </div>
        <div className="page-activity__actions">
          <button type="button" className="page-activity__action-btn" onClick={() => navigate({ name: "agents" })}>
            Back to Agents
          </button>
          {canManage ? (
            <button type="button" className="page-activity__action-btn" onClick={() => setTab("config")}>
              Edit config
            </button>
          ) : null}
          <button
            type="button"
            className="page-activity__action-btn"
            disabled={!agent}
            onClick={() => {
              setRunError(null)
              setRunOpen(true)
            }}
          >
            Run agent
          </button>
        </div>
      </div>

      {error ? <p className="page-activity__empty">{error}</p> : null}

      {loading ? (
        <p className="page-activity__empty">Loading…</p>
      ) : agent == null ? (
        !error ? <p className="page-activity__empty">Agent not found.</p> : null
      ) : (
        <>
          <nav className="agent-detail__tabs" aria-label="Agent sections">
            {TABS.map((t) => (
              <button
                key={t.id}
                type="button"
                className={
                  t.id === tab ? "agent-detail__tab agent-detail__tab--active" : "agent-detail__tab"
                }
                aria-current={t.id === tab}
                onClick={() => setTab(t.id)}
              >
                {t.label}
                {t.id === "runs" && tasks.length > 0 ? (
                  <span className="agent-detail__tab-count">{tasks.length}</span>
                ) : null}
                {t.id === "revisions" && agent.revision > 0 ? (
                  <span className="agent-detail__tab-count">{agent.revision}</span>
                ) : null}
              </button>
            ))}
          </nav>

          {tab === "overview" ? (
            <section className="agent-detail__panel">
              {secretWarnings > 0 ? (
                <div className="agent-detail__banner" role="alert">
                  <span>
                    ⚠ {secretWarnings} secret grant{secretWarnings === 1 ? "" : "s"} no longer resolve.
                  </span>
                  {canManage ? (
                    <button type="button" className="page-activity__action-btn" onClick={() => setTab("config")}>
                      Fix in config
                    </button>
                  ) : null}
                </div>
              ) : null}
              <div className="agent-detail__stats">
                <div className="agent-detail__stat">
                  <span className="agent-detail__stat-label">Total runs</span>
                  <span className="agent-detail__stat-value">{stats.total}</span>
                </div>
                <div className="agent-detail__stat">
                  <span className="agent-detail__stat-label">Success rate</span>
                  <span className="agent-detail__stat-value">{stats.successRate}</span>
                </div>
                <div className="agent-detail__stat">
                  <span className="agent-detail__stat-label">Last run</span>
                  <span className="agent-detail__stat-value agent-detail__stat-value--sm">{stats.lastRun}</span>
                </div>
              </div>
              <div className="agent-detail__section-head">
                <h2 className="issues-page__section-title">Recent runs</h2>
                {tasks.length > 3 ? (
                  <button type="button" className="page-activity__action-btn" onClick={() => setTab("runs")}>
                    View all
                  </button>
                ) : null}
              </div>
              {renderRunsTable(tasks.slice(0, 3))}
            </section>
          ) : null}

          {tab === "config" ? (
            <section className="agent-detail__panel">
              <AgentConfigForm
                agent={agent}
                secrets={secrets}
                availablePlugins={availablePlugins}
                availableModels={availableModels}
                canManage={canManage}
                saving={saving}
                deleting={deleting}
                error={saveError}
                onSave={handleSave}
                onDelete={handleDelete}
              />
            </section>
          ) : null}

          {tab === "runs" ? (
            <section className="agent-detail__panel">
              <p className="page-activity__subtitle">Each run is a durable Task thread. Select one to open it.</p>
              {renderRunsTable(tasks)}
            </section>
          ) : null}

          {tab === "revisions" ? (
            <section className="agent-detail__panel">
              <RevisionHistory
                title="Configuration history"
                entries={revisions.map((rev) => ({
                  id: rev.id,
                  revision: rev.revision,
                  createdBy: rev.createdBy,
                  createdLabel: rev.createdLabel,
                  summary: rev.instructions,
                }))}
                currentRevision={agent.revision}
                loading={revisionsLoading}
                error={revisionsError}
                canRestore={canManage}
                restoringRevision={restoringRevision}
                onRestore={handleRestoreRevision}
              />
            </section>
          ) : null}
        </>
      )}

      <RunAgentModal
        open={runOpen}
        agent={agent}
        loading={starting}
        error={runError}
        onClose={() => {
          setRunOpen(false)
          setRunError(null)
        }}
        onStart={handleStartRun}
      />
    </div>
  )
}
