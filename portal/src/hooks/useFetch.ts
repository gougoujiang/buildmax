import { useCallback, useEffect, useRef, useState } from "react"
import { getErrorMessage } from "../lib/errorMessage"

export interface UseFetchOptions {
  /** When false, no fetch runs and data/error are cleared. Default true. */
  enabled?: boolean
  /** Map caught error to message. Default: err.message or "Request failed". */
  errorMessage?: (err: unknown) => string
}

export interface UseFetchResult<T> {
  data: T | null
  loading: boolean
  error: string | null
  refetch: () => void
}

/**
 * Fetches when deps change (and enabled). Cancels on unmount or when deps change.
 * refetch() re-runs the fetch with the latest fetchFn.
 */
export function useFetch<T>(
  fetchFn: () => Promise<T>,
  deps: unknown[],
  options: UseFetchOptions = {}
): UseFetchResult<T> {
  const { enabled = true, errorMessage = (e) => getErrorMessage(e, "Request failed") } = options
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const fetchFnRef = useRef(fetchFn)
  fetchFnRef.current = fetchFn
  const errorMessageRef = useRef(errorMessage)
  errorMessageRef.current = errorMessage

  const runFetch = useCallback(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    fetchFnRef
      .current()
      .then((value) => {
        if (!cancelled) setData(value)
      })
      .catch((err) => {
        if (!cancelled) setError(errorMessageRef.current(err))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    if (!enabled) {
      setData(null)
      setLoading(false)
      setError(null)
      return
    }
    return runFetch()
    // eslint-disable-next-line react-hooks/exhaustive-deps -- deps are intentional
  }, [enabled, runFetch, ...deps])

  const refetch = useCallback(() => {
    if (!enabled) return
    runFetch()
  }, [enabled, runFetch])

  return { data, loading, error, refetch }
}
