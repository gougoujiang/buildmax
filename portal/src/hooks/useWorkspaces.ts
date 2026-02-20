import { useState } from "react"
import { getWorkspaces, type ApiWorkspace } from "../lib/api"
import { useAsyncList } from "./useAsyncList"

export function useWorkspaces(token: string | null): {
  data: ApiWorkspace[]
  loading: boolean
  refetch: () => void
} {
  const [loading, setLoading] = useState(true)
  const { data, refetch } = useAsyncList(
    () => getWorkspaces(token!),
    (x) => x,
    [token],
    !!token,
    { setLoading }
  )
  return { data, loading, refetch }
}
