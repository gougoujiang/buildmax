import { FilesExplorer } from "../features/files"

interface FilesPanelProps {
  spaceId: string
  className?: string
}

export function FilesPanel({ spaceId, className }: FilesPanelProps) {
  return <FilesExplorer spaceId={spaceId} className={className} />
}
