import { FilesExplorer } from "../features/files"

interface FilesPanelProps {
  profileId: string
  className?: string
}

export function FilesPanel({ profileId, className }: FilesPanelProps) {
  return <FilesExplorer profileId={profileId} className={className} />
}
