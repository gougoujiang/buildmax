import type { ReactNode } from "react"

export interface EmptyStateAction {
  label: string
  onClick: () => void
}

export interface EmptyStateProps {
  /** A specific explanation, not a generic "No items." (see the design's Ready empty row). */
  message: string
  /** A valid creation or navigation action, when one exists for this collection. */
  action?: EmptyStateAction
  children?: ReactNode
}

/** Shared presenter for the Ready empty state: a request that succeeded with no objects. */
export function EmptyState({ message, action, children }: EmptyStateProps) {
  return (
    <div className="state-empty">
      <p className="state-empty__message">{message}</p>
      {children}
      {action && (
        <button type="button" className="btn btn--primary" onClick={action.onClick}>
          {action.label}
        </button>
      )}
    </div>
  )
}
