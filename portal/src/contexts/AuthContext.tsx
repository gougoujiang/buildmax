import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react"
import type { LoginUser } from "../lib/api"
import { UNAUTHORIZED_EVENT } from "../lib/api"
import { clearStoredCurrentTeamId } from "../lib/storage/currentTeamStorage"

const TOKEN_KEY = "buildmax_token"
const USER_KEY = "buildmax_user"

interface AuthState {
  token: string | null
  user: LoginUser | null
}

interface AuthContextValue extends AuthState {
  login: (token: string, user: LoginUser) => void
  logout: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

function loadStored(): AuthState {
  try {
    const token = localStorage.getItem(TOKEN_KEY)
    const userRaw = localStorage.getItem(USER_KEY)
    if (!token || !userRaw) return { token: null, user: null }
    const user = JSON.parse(userRaw) as LoginUser
    return { token, user }
  } catch {
    return { token: null, user: null }
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>(loadStored)

  const login = useCallback((token: string, user: LoginUser) => {
    localStorage.setItem(TOKEN_KEY, token)
    localStorage.setItem(USER_KEY, JSON.stringify(user))
    setState({ token, user })
  }, [])

  const logout = useCallback(() => {
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(USER_KEY)
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
