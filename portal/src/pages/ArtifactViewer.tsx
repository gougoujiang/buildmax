import type { Artifact } from "../lib/types"

interface ArtifactViewerProps {
  artifact: Artifact
}

export function ArtifactViewer({ artifact }: ArtifactViewerProps) {
  return (
    <div className="page-artifact">
      <header className="page-artifact__header">
        <h1 className="page-artifact__title">{artifact.title}</h1>
        <span className="page-artifact__kind">{artifact.kind}</span>
      </header>

      <section className="page-artifact__content">
        <pre className="page-artifact__preview">{artifact.preview}</pre>
      </section>
    </div>
  )
}
