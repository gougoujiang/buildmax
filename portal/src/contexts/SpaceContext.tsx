import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react"
import { getSpaces } from "../features/spaces/api"
import { getSpaceMembers } from "../features/spaces/api"
import type { ApiSpaceMember } from "../lib/api/types"
import {
  clearStoredCurrentSpaceId,
  getStoredCurrentSpaceId,
  setStoredCurrentSpaceId,
} from "../lib/storage/currentSpaceStorage"
import { useAuth } from "./AuthContext"

export interface SpaceSummary {
  id: string
  name: string
  personalForUserId?: string | null
}

interface SpaceContextValue {
  spaces: SpaceSummary[]
  currentSpaceId: string | null
  currentSpace: SpaceSummary | null
  currentSpaceMembers: ApiSpaceMember[]
  currentUserRole: string | null
  loading: boolean
  setCurrentSpaceId: (spaceId: string) => void
  refetchSpaces: (preferredSpaceId?: string | null) => Promise<void>
}

const SpaceContext = createContext<SpaceContextValue | null>(null)

function chooseCurrentSpace(spaces: SpaceSummary[]): string | null {
  if (spaces.length === 0) return null
  const stored = getStoredCurrentSpaceId()
  if (stored && spaces.some((space) => space.id === stored)) return stored
  return spaces[0].id
}

function normalizeSpaceName(space: { name: string; personal_for_user_id?: string | null }): string {
  if (space.personal_for_user_id && space.name.trim() === "My Space") {
    return "My Space"
  }
  return space.name
}

export function SpaceProvider({ children }: { children: ReactNode }) {
  const { token, user } = useAuth()
  const [spaces, setSpaces] = useState<SpaceSummary[]>([])
  const [currentSpaceId, setCurrentSpaceIdState] = useState<string | null>(getStoredCurrentSpaceId)
  const [currentSpaceMembers, setCurrentSpaceMembers] = useState<ApiSpaceMember[]>([])
  const [loading, setLoading] = useState(false)

  const refetchSpaces = useCallback(async (preferredSpaceId?: string | null) => {
    if (!token) {
      setSpaces([])
      setCurrentSpaceIdState(null)
      setCurrentSpaceMembers([])
      clearStoredCurrentSpaceId()
      return
    }

    setLoading(true)
    try {
      const nextSpaces = await getSpaces(token)
      const mapped = nextSpaces.map((space) => ({
        id: space.id,
        name: normalizeSpaceName(space),
        personalForUserId: space.personal_for_user_id ?? null,
      }))
      setSpaces(mapped)
      const nextCurrentSpaceId =
        preferredSpaceId && mapped.some((space) => space.id === preferredSpaceId)
          ? preferredSpaceId
          : chooseCurrentSpace(mapped)
      setCurrentSpaceIdState(nextCurrentSpaceId)
      setStoredCurrentSpaceId(nextCurrentSpaceId)
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => {
    void refetchSpaces()
  }, [refetchSpaces])

  useEffect(() => {
    if (!token || !currentSpaceId) {
      setCurrentSpaceMembers([])
      return
    }
    void getSpaceMembers(currentSpaceId, token)
      .then((members) => setCurrentSpaceMembers(members))
      .catch(() => setCurrentSpaceMembers([]))
  }, [token, currentSpaceId])

  const setCurrentSpaceId = useCallback(
    (spaceId: string) => {
      if (!spaces.some((space) => space.id === spaceId)) return
      setCurrentSpaceIdState(spaceId)
      setStoredCurrentSpaceId(spaceId)
    },
    [spaces]
  )

  const currentSpace = useMemo(
    () => spaces.find((space) => space.id === currentSpaceId) ?? null,
    [spaces, currentSpaceId]
  )

  const currentUserRole = useMemo(
    () => currentSpaceMembers.find((member) => member.user_id === user?.id)?.role ?? null,
    [currentSpaceMembers, user?.id],
  )

  const value: SpaceContextValue = {
    spaces,
    currentSpaceId,
    currentSpace,
    currentSpaceMembers,
    currentUserRole,
    loading,
    setCurrentSpaceId,
    refetchSpaces,
  }

  return (
    <SpaceContext.Provider value={value}>
      {children}
    </SpaceContext.Provider>
  )
}

export function useSpace(): SpaceContextValue {
  const ctx = useContext(SpaceContext)
  if (!ctx) throw new Error("useSpace must be used within SpaceProvider")
  return ctx
}
