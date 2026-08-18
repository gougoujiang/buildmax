import { useEffect, useState } from "react"
import { useAuth } from "../../contexts/AuthContext"
import { getAdminMe } from "./api"

export interface AdminAccess {
  isAdmin: boolean
  /** True until the first answer arrives, so nothing flashes into view. */
  loading: boolean
}

/**
 * useAdminAccess asks the server whether this person may operate the deployment.
 *
 * Any rejection means no. A 403 is the expected answer for almost everyone and
 * is not an error worth surfacing — and treating a network failure as "not an
 * administrator" is the safe direction: it hides an entry, it never opens one.
 * The server refuses regardless of what this returns.
 */
export function useAdminAccess(): AdminAccess {
  const { token } = useAuth()
  const [isAdmin, setIsAdmin] = useState(false)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!token) {
      setIsAdmin(false)
      setLoading(false)
      return
    }
    let cancelled = false
    setLoading(true)
    getAdminMe(token)
      .then(() => {
        if (!cancelled) setIsAdmin(true)
      })
      .catch(() => {
        if (!cancelled) setIsAdmin(false)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [token])

  return { isAdmin, loading }
}
