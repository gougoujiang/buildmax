import type { Task } from "../lib/types"
import { getTasks, apiTaskToTask } from "../lib/api"
import { useAsyncList } from "./useAsyncList"

export function useTasks(
  workspaceId: string,
  token: string | null,
  projectId?: string
): { data: Task[]; refetch: () => void } {
  return useAsyncList(
    () => getTasks(workspaceId, token!, projectId),
    (list) => list.map(apiTaskToTask),
    [token, workspaceId, projectId],
    !!(token && workspaceId && projectId !== undefined)
  )
}
