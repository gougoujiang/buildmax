import { FilesExplorer } from "../features/files"

interface FilesPanelProps {
  className?: string
}

export function FilesPanel({ className }: FilesPanelProps) {
  return <FilesExplorer className={className} />
}
