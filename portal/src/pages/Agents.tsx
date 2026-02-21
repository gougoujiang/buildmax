import { useCallback, useEffect, useState } from "react"
import type { Task, ViewArtifactParams } from "../lib/types"
import { taskStatusIcon } from "../lib/taskStatus"
import { navigate } from "../router"
import { getTasksPaginated, apiTaskToTask } from "../lib/api"

const PAGE_SIZE = 20

interface AgentsProps {
  workspaceId: string
  token: string | null
  onViewArtifact?: (params: ViewArtifactParams) => void
}

export function Agents({
  workspaceId,
  token,
}: AgentsProps) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)

  const fetchPage = useCallback(
    (off: number, append: boolean) => {
      if (!token) return
      setLoading(true)
      getTasksPaginated(workspaceId, token, {
        limit: PAGE_SIZE,
        offset: off,
        executedOnly: true,
      })
        .then((res) => {
          const next = res.tasks.map(apiTaskToTask)
          setTotal(res.total)
          setTasks((prev) => (append ? [...prev, ...next] : next))
        })
        .finally(() => setLoading(false))
    },
    [workspaceId, token]
  )

  useEffect(() => {
    setOffset(0)
  }, [workspaceId])

  useEffect(() => {
    fetchPage(offset, offset > 0)
  }, [offset, fetchPage])

  const hasMore = total > 0 && tasks.length < total
  const loadMore = () => setOffset((o) => o + PAGE_SIZE)

  return (
    <div className="page-activity">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", flexWrap: "wrap", gap: "0.75rem" }}>
        <div>
          <h1 className="page-activity__title">Agents</h1>
          <p className="page-activity__subtitle">
            Executed tasks in this workspace (most recent first).
          </p>
        </div>
        <button
          type="button"
          className="topbar__workspace-new"
          onClick={() => navigate({ name: "agentList", workspaceId })}
        >
          Manage agents
        </button>
      </div>
      {loading && offset === 0 ? (
        <p className="page-activity__empty">Loading…</p>
      ) : tasks.length === 0 ? (
        <p className="page-activity__empty">No executed tasks yet.</p>
      ) : (
        <>
          <ul className="page-activity__list">
            {tasks.map((task) => (
              <li key={task.id} className="page-activity__item">
                <button
                  type="button"
                  className="page-activity__link"
                  onClick={() =>
                    navigate({
                      name: "task",
                      workspaceId,
                      taskId: task.id,
                    })
                  }
                >
                  <span className="page-activity__icon">
                    {taskStatusIcon(task.status)}
                  </span>
                  <span className="page-activity__content">
                    <span className="page-activity__task-title">{task.title}</span>
                    <span className="page-activity__meta">{task.timeLabel}</span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
          {hasMore && (
            <div style={{ marginTop: "1rem" }}>
              <button
                type="button"
                className="topbar__workspace-new"
                onClick={loadMore}
                disabled={loading}
              >
                {loading ? "Loading…" : "Load more"}
              </button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
