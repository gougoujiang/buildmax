import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react"
import type { LoginResponse, LoginUser } from "../lib/api"
import { TOKEN_REFRESHED_EVENT, UNAUTHORIZED_EVENT } from "../lib/api"
import { revokeSession } from "../features/auth/api"
import {
  clearSession,
  currentAccessToken,
  expiresAtFrom,
  readStoredUser,
  writeSession,
  writeStoredUser,
} from "../lib/api/session"
import { clearStoredCurrentTeamId } from "../lib/storage/currentTeamStorage"

interface AuthState {
  token: string | null
  user: LoginUser | null
}

interface AuthContextValue extends AuthState {
  login: (res: LoginResponse) => void
  logout: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

function loadStored(): AuthState {
  const token = currentAccessToken()
  const user = readStoredUser<LoginUser>()
  if (!token || !user) return { token: null, user: null }
  return { token, user }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>(loadStored)

  const login = useCallback((res: LoginResponse) => {
    // access_token is the current name; token is what the server called it
    // before the credentials were split, and older servers send only that.
    const accessToken = res.access_token ?? res.token
    writeSession({
      accessToken,
      refreshToken: res.refresh_token ?? null,
      expiresAt: expiresAtFrom(res.expires_in),
    })
    writeStoredUser(res.user)
    setState({ token: accessToken, user: res.user })
  }, [])

  const logout = useCallback(() => {
    // Tell the server first: clearing local state only makes this browser
    // forget the session, while the refresh token stays usable for weeks.
    // Best effort — a failed call must not strand someone in a session they
    // asked to leave.
    void revokeSession()
    clearSession()
    clearStoredCurrentTeamId()
    setState({ token: null, user: null })
  }, [])

  useEffect(() => {
    function onUnauthorized() {
      logout()
    }
    window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized)
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized)
  }, [logout])

  useEffect(() => {
    // A refresh can happen inside any request. Without this the context would
    // keep handing out the replaced token, and every consumer that reconnects
    // on a token change — the WebSocket above all — would keep using the old
    // one until the next reload.
    function onRefreshed(event: Event) {
      const accessToken = (event as CustomEvent<{ accessToken?: string }>).detail?.accessToken
      if (!accessToken) return
      setState((prev) => (prev.token === accessToken ? prev : { ...prev, token: accessToken }))
    }
    window.addEventListener(TOKEN_REFRESHED_EVENT, onRefreshed)
    return () => window.removeEventListener(TOKEN_REFRESHED_EVENT, onRefreshed)
  }, [])

  const value: AuthContextValue = {
    ...state,
    login,
    logout,
  }

  return (
    <AuthContext.Provider value={value}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error("useAuth must be used within AuthProvider")
  return ctx
}
