interface ActivityPageProps {
  workspaceId: string
}

export function ActivityPage({ workspaceId: _workspaceId }: ActivityPageProps) {
  return (
    <div className="page-activity">
      <h1 className="page-activity__title">Activity</h1>
      <p className="page-activity__subtitle">Coming soon.</p>
    </div>
  )
}
