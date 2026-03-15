import { useCallback, useEffect, useRef, useState } from "react"
import type { Artifact } from "../lib/types"
import { apiArtifactToArtifact } from "../lib/api"
import { getArtifacts } from "../features/artifacts"

export interface UseArtifactsOptions {
  chatId?: string
  enabled?: boolean
}

export function useArtifacts(
  profileId: string,
  token: string | null,
  options?: UseArtifactsOptions
): { data: Artifact[]; refetch: (overrides?: UseArtifactsOptions) => void } {
  const optionsRef = useRef(options)
  optionsRef.current = options
  const [data, setData] = useState<Artifact[]>([])
  const enabled = options?.enabled ?? true

  const runFetch = useCallback(
    (overrides?: UseArtifactsOptions) => {
      const opts = overrides ?? optionsRef.current
      const fetchEnabled = opts?.enabled ?? true
      if (!fetchEnabled || !token || !profileId) {
        setData([])
        return
      }
      const query =
        opts?.chatId !== undefined ? { chatId: opts.chatId } : undefined
      getArtifacts(profileId, token, query)
        .then((list) => setData(list.map(apiArtifactToArtifact)))
        .catch(() => setData([]))
    },
    [profileId, token]
  )

  useEffect(() => {
    if (!enabled || !token || !profileId) {
      setData([])
      return
    }
    runFetch()
  }, [enabled, token, profileId, options?.chatId, runFetch])

  const refetch = useCallback(
    (overrides?: UseArtifactsOptions) => {
      runFetch(overrides)
    },
    [runFetch]
  )

  return { data, refetch }
}
