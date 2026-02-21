import { useCallback, useEffect, useRef, useState } from "react"
import type { Artifact } from "../lib/types"
import { getArtifacts, apiArtifactToArtifact } from "../lib/api"

export interface UseArtifactsOptions {
  projectId?: string
  chatId?: string
}

export function useArtifacts(
  workspaceId: string,
  token: string | null,
  options?: UseArtifactsOptions
): { data: Artifact[]; refetch: (overrides?: UseArtifactsOptions) => void } {
  const optionsRef = useRef(options)
  optionsRef.current = options
  const [data, setData] = useState<Artifact[]>([])

  const runFetch = useCallback(
    (overrides?: UseArtifactsOptions) => {
      if (!token || !workspaceId) {
        setData([])
        return
      }
      const opts = overrides ?? optionsRef.current
      const query =
        opts?.projectId !== undefined || opts?.chatId !== undefined
          ? { projectId: opts?.projectId, chatId: opts?.chatId }
          : undefined
      getArtifacts(workspaceId, token, query)
        .then((list) => setData(list.map(apiArtifactToArtifact)))
        .catch(() => setData([]))
    },
    [workspaceId, token]
  )

  useEffect(() => {
    if (!token || !workspaceId) {
      setData([])
      return
    }
    runFetch()
  }, [token, workspaceId, options?.projectId, options?.chatId, runFetch])

  const refetch = useCallback(
    (overrides?: UseArtifactsOptions) => {
      runFetch(overrides)
    },
    [runFetch]
  )

  return { data, refetch }
}
