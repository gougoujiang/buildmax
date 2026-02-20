import type { Task } from "../lib/types"
import { getTasks, apiTaskToTask } from "../lib/api"
import { useAsyncList } from "./useAsyncList"

export function useWorkspaceTasks(
  workspaceId: string,
  token: string | null
): { data: Task[]; refetch: () => void } {
  return useAsyncList(
    () => getTasks(workspaceId, token!),
    (list) => list.map(apiTaskToTask),
    [token, workspaceId],
    !!(token && workspaceId)
  )
}
