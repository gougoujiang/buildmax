import type { ApiWorkspace } from "../lib/api"
import type { LoginUser } from "../lib/api"

export function useWorkspaces(token: string | null, user: LoginUser | null): {
  data: ApiWorkspace[]
  loading: boolean
  error: string | null
  refetch: () => Promise<void>
} {
  const data =
    token && user
      ? [
          {
            id: user.id,
            name: user.name?.trim() || user.email || "Personal",
            owner_user_id: user.id,
          },
        ]
      : []
  return {
    data,
    loading: false,
    error: null,
    refetch: async () => {},
  }
}
