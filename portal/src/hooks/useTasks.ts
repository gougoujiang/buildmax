import type { Task } from "../lib/types"
import { getErrorMessage } from "../lib/errorMessage"
import { apiTaskToTask } from "../lib/api"
import { getTask } from "../features/tasks"
import { useFetch } from "./useFetch"

export function useTasks(
  profileId: string,
  token: string | null,
  taskId?: string,
  enabled = true
): { data: Task[]; loading: boolean; error: string | null; refetch: () => Promise<void> } {
  const result = useFetch(
    () => getTask(taskId!, token!),
    [token, taskId],
    {
      enabled: enabled && !!(token && profileId && taskId),
      errorMessage: (e) => getErrorMessage(e, "Failed to load task"),
    }
  )
  return {
    data: result.data ? [apiTaskToTask(result.data)] : [],
    loading: result.loading,
    error: result.error,
    refetch: async () => {
      result.refetch()
    },
  }
}
