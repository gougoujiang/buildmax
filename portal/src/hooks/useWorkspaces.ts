import { getErrorMessage } from "../lib/errorMessage"
import type { ApiWorkspace } from "../lib/api"
import { getWorkspaces } from "../features/workspaces"
import { useAsyncList } from "./useAsyncList"

export function useWorkspaces(token: string | null): {
  data: ApiWorkspace[]
  loading: boolean
  error: string | null
  refetch: () => Promise<void>
} {
  const { data, loading, error, refetch } = useAsyncList(
    () => getWorkspaces(token!),
    (x) => x,
    [token],
    !!token,
    { errorMessage: (e) => getErrorMessage(e, "Failed to load workspaces") }
  )
  return { data, loading, error, refetch }
}
