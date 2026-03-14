import { FilesExplorer } from "../features/files"

interface FilesPanelProps {
  workspaceId: string
  className?: string
}

export function FilesPanel({ workspaceId, className }: FilesPanelProps) {
  return <FilesExplorer workspaceId={workspaceId} className={className} />
}
