import { useCallback, useEffect, useRef, useState } from "react"

/**
 * Fetches a list when deps are valid (enabled). Clears when disabled. refetch() re-runs the fetch.
 * Uses the same fetch+map logic for both the effect and refetch to avoid duplication.
 */
export function useAsyncList<T, U>(
  fetchFn: () => Promise<T[]>,
  map: (raw: T[]) => U[],
  deps: unknown[],
  enabled: boolean,
  options?: { setLoading?: (loading: boolean) => void }
): { data: U[]; refetch: () => void } {
  const [data, setData] = useState<U[]>([])
  const fetchFnRef = useRef(fetchFn)
  const mapRef = useRef(map)
  fetchFnRef.current = fetchFn
  mapRef.current = map
  const setLoading = options?.setLoading

  const runFetch = useCallback(() => {
    setLoading?.(true)
    fetchFnRef
      .current()
      .then((raw) => setData(mapRef.current(raw)))
      .catch(() => setData([]))
      .finally(() => setLoading?.(false))
  }, [setLoading])

  useEffect(() => {
    if (!enabled) {
      setData([])
      setLoading?.(false)
      return
    }
    runFetch()
    // eslint-disable-next-line react-hooks/exhaustive-deps -- deps are intentional
  }, [enabled, runFetch, ...deps])

  const refetch = useCallback(() => {
    if (!enabled) return
    runFetch()
  }, [enabled, runFetch])

  return { data, refetch }
}
