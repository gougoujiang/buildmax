import type { Workspace, Project, Task } from "../lib/types"

// --- Mock workspaces (UUID ids); mutable so "New workspace" can add entries ---

const INITIAL_WORKSPACES: Workspace[] = [
  { id: "00000000-0000-4000-8000-000000000001", name: "Default" },
  { id: "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d", name: "Sales Team" },
  { id: "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e", name: "Engineering" },
  { id: "c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f", name: "Marketing" },
]

const WORKSPACES: Workspace[] = [...INITIAL_WORKSPACES]

// --- Mock projects (at least 2 per workspace; ids unique globally) ---

export const MOCK_PROJECTS: Project[] = [
  // Sales Team
  {
    id: "p1",
    workspaceId: INITIAL_WORKSPACES[1].id,
    name: "Monthly Sales Report",
    status: "active",
    updatedAtLabel: "Updated 2h ago",
  },
  {
    id: "p2",
    workspaceId: INITIAL_WORKSPACES[1].id,
    name: "Pricing Strategy Draft",
    status: "active",
    updatedAtLabel: "Updated 1d ago",
  },
  {
    id: "p3",
    workspaceId: INITIAL_WORKSPACES[1].id,
    name: "Customer QBR Pack",
    status: "paused",
    updatedAtLabel: "Updated 1w ago",
  },
  // Engineering
  {
    id: "p4",
    workspaceId: INITIAL_WORKSPACES[2].id,
    name: "API Gateway Refactor",
    status: "active",
    updatedAtLabel: "Updated 3h ago",
  },
  {
    id: "p5",
    workspaceId: INITIAL_WORKSPACES[2].id,
    name: "Test Coverage Dashboard",
    status: "active",
    updatedAtLabel: "Updated yesterday",
  },
  // Marketing
  {
    id: "p6",
    workspaceId: INITIAL_WORKSPACES[3].id,
    name: "Q2 Campaign Plan",
    status: "active",
    updatedAtLabel: "Updated 5h ago",
  },
  {
    id: "p7",
    workspaceId: INITIAL_WORKSPACES[3].id,
    name: "Social Content Calendar",
    status: "paused",
    updatedAtLabel: "Updated 2d ago",
  },
]

// --- Mock tasks ---

export const MOCK_TASKS: Task[] = [
  { id: "t1", projectId: "p1", title: "Generated sales report", status: "success", timeLabel: "Today 10:42 AM", summary: "Created a monthly sales report with revenue and growth charts." },
  { id: "t2", projectId: "p1", title: "Imported February data", status: "success", timeLabel: "Today 10:40 AM", summary: "Added the new February dataset for analysis." },
  { id: "t3", projectId: "p2", title: "Drafted initial pricing model", status: "success", timeLabel: "Yesterday 3:15 PM", summary: "Created a pricing comparison table with competitor benchmarks." },
  { id: "t4", projectId: "p3", title: "Compiled Q4 review slides", status: "failed", timeLabel: "Feb 10, 1:20 PM", summary: "Attempted to compile slides but missing data source." },
  { id: "t5", projectId: "p4", title: "Migrated auth middleware", status: "success", timeLabel: "Today 9:00 AM", summary: "Moved auth checks into shared middleware." },
  { id: "t6", projectId: "p4", title: "Added rate limiting", status: "success", timeLabel: "Yesterday 4:30 PM", summary: "Implemented per-IP rate limits on gateway." },
  { id: "t7", projectId: "p5", title: "Generated coverage report", status: "success", timeLabel: "Feb 12, 2:00 PM", summary: "Coverage now at 78% for core packages." },
  { id: "t8", projectId: "p6", title: "Drafted campaign brief", status: "success", timeLabel: "Today 11:00 AM", summary: "Created Q2 campaign objectives and channels." },
  { id: "t9", projectId: "p7", title: "Scheduled March posts", status: "success", timeLabel: "Feb 14, 10:00 AM", summary: "Added 12 posts to the content calendar." },
]

// --- Workspace helpers ---

export function listWorkspaces(): Workspace[] {
  return [...WORKSPACES]
}

export function getWorkspaceById(id: string): Workspace | undefined {
  return WORKSPACES.find((w) => w.id === id)
}

function generateWorkspaceId(): string {
  return crypto.randomUUID()
}

export function createWorkspace(name: string): Workspace {
  const workspace: Workspace = { id: generateWorkspaceId(), name }
  WORKSPACES.push(workspace)
  return workspace
}

// --- Lookup helpers ---

export function getProjectById(projectId: string): Project | undefined {
  return MOCK_PROJECTS.find((p) => p.id === projectId)
}

export function listProjectsForWorkspace(workspaceId: string): Project[] {
  return MOCK_PROJECTS.filter((p) => p.workspaceId === workspaceId)
}

export function listTasksForWorkspace(workspaceId: string): Task[] {
  const projectIds = listProjectsForWorkspace(workspaceId).map((p) => p.id)
  return MOCK_TASKS.filter((t) => t.projectId != null && projectIds.includes(t.projectId))
}

export function listTasksForProject(projectId: string): Task[] {
  return MOCK_TASKS.filter((t) => t.projectId === projectId)
}

export function getTaskById(
  projectId: string,
  taskId: string,
): Task | undefined {
  return MOCK_TASKS.find((t) => t.projectId === projectId && t.id === taskId)
}
