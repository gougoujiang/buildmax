import { FilesPanel } from "../../components/FilesPanel"

interface ExploreProps {
  spaceId: string
}

export function Explore({ spaceId }: ExploreProps) {
  return (
    <div className="page-explore">
      <h1 className="page-explore__title">Workspace Files</h1>
      <p className="page-explore__subtitle">
        This space&apos;s working files — mutable inputs and in-progress state that agent
        runs read and write. Once work is done, publish a durable, unchanging copy as an
        Artifact instead.
      </p>
      <FilesPanel spaceId={spaceId} />
    </div>
  )
}
