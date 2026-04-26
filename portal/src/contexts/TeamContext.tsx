import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react"
import { getTeams } from "../features/teams/api"
import { getTeamMembers } from "../features/teams/api"
import type { ApiTeamMember } from "../lib/api/types"
import {
  clearStoredCurrentTeamId,
  getStoredCurrentTeamId,
  setStoredCurrentTeamId,
} from "../features/teams/storage"
import { useAuth } from "./AuthContext"

export interface TeamSummary {
  id: string
  name: string
  personalForUserId?: string | null
}

interface TeamContextValue {
  teams: TeamSummary[]
  currentTeamId: string | null
  currentTeam: TeamSummary | null
  currentTeamMembers: ApiTeamMember[]
  currentUserRole: string | null
  loading: boolean
  setCurrentTeamId: (teamId: string) => void
  refetchTeams: (preferredTeamId?: string | null) => Promise<void>
}

const TeamContext = createContext<TeamContextValue | null>(null)

function chooseCurrentTeam(teams: TeamSummary[]): string | null {
  if (teams.length === 0) return null
  const stored = getStoredCurrentTeamId()
  if (stored && teams.some((team) => team.id === stored)) return stored
  return teams[0].id
}

function normalizeTeamName(team: { name: string; personal_for_user_id?: string | null }): string {
  if (team.personal_for_user_id && team.name.trim() === "My Space") {
    return "My Space"
  }
  return team.name
}

export function TeamProvider({ children }: { children: ReactNode }) {
  const { token, user } = useAuth()
  const [teams, setTeams] = useState<TeamSummary[]>([])
  const [currentTeamId, setCurrentTeamIdState] = useState<string | null>(getStoredCurrentTeamId)
  const [currentTeamMembers, setCurrentTeamMembers] = useState<ApiTeamMember[]>([])
  const [loading, setLoading] = useState(false)

  const refetchTeams = useCallback(async (preferredTeamId?: string | null) => {
    if (!token) {
      setTeams([])
      setCurrentTeamIdState(null)
      setCurrentTeamMembers([])
      clearStoredCurrentTeamId()
      return
    }

    setLoading(true)
    try {
      const nextTeams = await getTeams(token)
      const mapped = nextTeams.map((team) => ({
        id: team.id,
        name: normalizeTeamName(team),
        personalForUserId: team.personal_for_user_id ?? null,
      }))
      setTeams(mapped)
      const nextCurrentTeamId =
        preferredTeamId && mapped.some((team) => team.id === preferredTeamId)
          ? preferredTeamId
          : chooseCurrentTeam(mapped)
      setCurrentTeamIdState(nextCurrentTeamId)
      setStoredCurrentTeamId(nextCurrentTeamId)
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => {
    void refetchTeams()
  }, [refetchTeams])

  useEffect(() => {
    if (!token || !currentTeamId) {
      setCurrentTeamMembers([])
      return
    }
    void getTeamMembers(currentTeamId, token)
      .then((members) => setCurrentTeamMembers(members))
      .catch(() => setCurrentTeamMembers([]))
  }, [token, currentTeamId])

  const setCurrentTeamId = useCallback(
    (teamId: string) => {
      if (!teams.some((team) => team.id === teamId)) return
      setCurrentTeamIdState(teamId)
      setStoredCurrentTeamId(teamId)
    },
    [teams]
  )

  const currentTeam = useMemo(
    () => teams.find((team) => team.id === currentTeamId) ?? null,
    [teams, currentTeamId]
  )

  const currentUserRole = useMemo(
    () => currentTeamMembers.find((member) => member.user_id === user?.id)?.role ?? null,
    [currentTeamMembers, user?.id],
  )

  const value: TeamContextValue = {
    teams,
    currentTeamId,
    currentTeam,
    currentTeamMembers,
    currentUserRole,
    loading,
    setCurrentTeamId,
    refetchTeams,
  }

  return (
    <TeamContext.Provider value={value}>
      {children}
    </TeamContext.Provider>
  )
}

export function useTeam(): TeamContextValue {
  const ctx = useContext(TeamContext)
  if (!ctx) throw new Error("useTeam must be used within TeamProvider")
  return ctx
}
