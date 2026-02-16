import { useCallback, useEffect, useState } from "react"
import type { Project, Task } from "../lib/types"
import type { Route } from "../lib/types"
import {
  getWorkspaces,
  getProjects,
  getTasks,
  apiProjectToProject,
  apiTaskToTask,
  type ApiWorkspace,
} from "../lib/api"

export interface UseWorkspaceDataResult {
  workspaces: ApiWorkspace[]
  projects: Project[]
  tasks: Task[]
  workspaceTasks: Task[]
  loadingWorkspaces: boolean
  refetchWorkspaces: () => void
  refetchProjects: () => void
  refetchTasks: () => void
  refetchWorkspaceTasks: () => void
}

export function useWorkspaceData(
  token: string | null,
  route: Route
): UseWorkspaceDataResult {
  const [workspaces, setWorkspaces] = useState<ApiWorkspace[]>([])
  const [loadingWorkspaces, setLoadingWorkspaces] = useState(true)
  const [projects, setProjects] = useState<Project[]>([])
  const [tasks, setTasks] = useState<Task[]>([])
  const [workspaceTasks, setWorkspaceTasks] = useState<Task[]>([])

  const projectIdFromRoute = "projectId" in route ? route.projectId : undefined

  const refetchWorkspaces = useCallback(() => {
    if (!token) return
    getWorkspaces(token)
      .then(setWorkspaces)
      .catch(() => setWorkspaces([]))
  }, [token])

  useEffect(() => {
    if (!token) {
      setWorkspaces([])
      setLoadingWorkspaces(false)
      return
    }
    setLoadingWorkspaces(true)
    getWorkspaces(token)
      .then(setWorkspaces)
      .catch(() => setWorkspaces([]))
      .finally(() => setLoadingWorkspaces(false))
  }, [token])

  const refetchProjects = useCallback(() => {
    if (!token || !route.workspaceId) return
    getProjects(route.workspaceId, token)
      .then((list) => setProjects(list.map(apiProjectToProject)))
      .catch(() => setProjects([]))
  }, [token, route.workspaceId])

  useEffect(() => {
    if (!token || !route.workspaceId) {
      setProjects([])
      return
    }
    getProjects(route.workspaceId, token)
      .then((list) => setProjects(list.map(apiProjectToProject)))
      .catch(() => setProjects([]))
  }, [token, route.workspaceId])

  const refetchTasks = useCallback(() => {
    if (!token || !route.workspaceId || !projectIdFromRoute) return
    getTasks(route.workspaceId, token, projectIdFromRoute)
      .then((list) => setTasks(list.map(apiTaskToTask)))
      .catch(() => setTasks([]))
  }, [token, route.workspaceId, projectIdFromRoute])

  useEffect(() => {
    if (!token || !route.workspaceId || !projectIdFromRoute) {
      setTasks([])
      return
    }
    getTasks(route.workspaceId, token, projectIdFromRoute)
      .then((list) => setTasks(list.map(apiTaskToTask)))
      .catch(() => setTasks([]))
  }, [token, route.workspaceId, projectIdFromRoute])

  const refetchWorkspaceTasks = useCallback(() => {
    if (!token || !route.workspaceId) return
    getTasks(route.workspaceId, token)
      .then((list) => setWorkspaceTasks(list.map(apiTaskToTask)))
      .catch(() => setWorkspaceTasks([]))
  }, [token, route.workspaceId])

  useEffect(() => {
    if (!token || !route.workspaceId) {
      setWorkspaceTasks([])
      return
    }
    getTasks(route.workspaceId, token)
      .then((list) => setWorkspaceTasks(list.map(apiTaskToTask)))
      .catch(() => setWorkspaceTasks([]))
  }, [token, route.workspaceId])

  return {
    workspaces,
    projects,
    tasks,
    workspaceTasks,
    loadingWorkspaces,
    refetchWorkspaces,
    refetchProjects,
    refetchTasks,
    refetchWorkspaceTasks,
  }
}
