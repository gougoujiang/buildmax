import { FilesPanel } from "../components/FilesPanel"

interface ExploreProps {
  profileId: string
}

export function Explore({ profileId }: ExploreProps) {
  return (
    <div className="page-explore">
      <h1 className="page-explore__title">Explore</h1>
      <p className="page-explore__subtitle">
        Browse your files. Select a folder in the tree, then open a file to view its content.
      </p>
      <FilesPanel profileId={profileId} />
    </div>
  )
}
