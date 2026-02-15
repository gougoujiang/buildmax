interface ExplorePageProps {
  workspaceId: string
}

export function ExplorePage({ workspaceId: _workspaceId }: ExplorePageProps) {
  return (
    <div className="page-explore">
      <h1 className="page-explore__title">Explore</h1>
      <p className="page-explore__subtitle">Coming soon.</p>
    </div>
  )
}
