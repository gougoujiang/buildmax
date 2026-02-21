import { FilesPanel } from "../components/FilesPanel"

interface ExploreProps {
  workspaceId: string
}

export function Explore({ workspaceId }: ExploreProps) {
  return (
    <div className="page-explore">
      <h1 className="page-explore__title">Explore</h1>
      <p className="page-explore__subtitle">
        Browse workspace structure. Select a folder in the tree, then open a file to view its content.
      </p>
      <FilesPanel workspaceId={workspaceId} />
    </div>
  )
}
