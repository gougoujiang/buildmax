import { useCallback, useEffect, useRef, useState } from "react"
import { getErrorMessage } from "../lib/errorMessage"

export interface UseAsyncListOptions {
  /** Optional callback when loading state changes (e.g. for parent state). */
  setLoading?: (loading: boolean) => void
  /** Map caught error to message. Default: getErrorMessage(err, "Request failed"). */
  errorMessage?: (err: unknown) => string
}

/**
 * Fetches a list when deps are valid (enabled). Clears when disabled. refetch() re-runs the fetch.
 * Returns data, loading, error so callers can show loading and error states.
 */
export function useAsyncList<T, U>(
  fetchFn: () => Promise<T[]>,
  map: (raw: T[]) => U[],
  deps: unknown[],
  enabled: boolean,
  options?: UseAsyncListOptions
): { data: U[]; loading: boolean; error: string | null; refetch: () => Promise<void> } {
  const [data, setData] = useState<U[]>([])
  const [loading, setLoadingState] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const fetchFnRef = useRef(fetchFn)
  const mapRef = useRef(map)
  const errorMessageRef = useRef(options?.errorMessage ?? ((e: unknown) => getErrorMessage(e, "Request failed")))
  fetchFnRef.current = fetchFn
  mapRef.current = map
  errorMessageRef.current = options?.errorMessage ?? ((e: unknown) => getErrorMessage(e, "Request failed"))

  const runFetch = useCallback((): Promise<void> => {
    setLoadingState(true)
    setError(null)
    options?.setLoading?.(true)
    return fetchFnRef
      .current()
      .then((raw) => {
        setData(mapRef.current(raw))
        setError(null)
      })
      .catch((err) => {
        setData([])
        setError(errorMessageRef.current(err))
      })
      .finally(() => {
        setLoadingState(false)
        options?.setLoading?.(false)
      })
  }, [options?.setLoading])

  useEffect(() => {
    if (!enabled) {
      setData([])
      setError(null)
      setLoadingState(false)
      options?.setLoading?.(false)
      return
    }
    runFetch()
    // eslint-disable-next-line react-hooks/exhaustive-deps -- deps are intentional
  }, [enabled, runFetch, ...deps])

  const refetch = useCallback((): Promise<void> => {
    if (!enabled) return Promise.resolve()
    return runFetch()
  }, [enabled, runFetch])

  return { data, loading, error, refetch }
}
