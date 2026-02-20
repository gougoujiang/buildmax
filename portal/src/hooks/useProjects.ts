import type { Project } from "../lib/types"
import { getProjects, apiProjectToProject } from "../lib/api"
import { useAsyncList } from "./useAsyncList"

export function useProjects(
  workspaceId: string,
  token: string | null
): { data: Project[]; refetch: () => void } {
  return useAsyncList(
    () => getProjects(workspaceId, token!),
    (list) => list.map(apiProjectToProject),
    [token, workspaceId],
    !!(token && workspaceId)
  )
}
