import type { ReactNode } from "react"

function cn(...classes: Array<string | false | null | undefined>) {
  return classes.filter(Boolean).join(" ")
}

export interface RecentListItem {
  id: string
  title: string
  meta?: string
}

export interface RecentListProps {
  items: RecentListItem[]
  activeId?: string | null
  onSelect: (id: string) => void
  moreActionLabel?: string
  onMoreAction?: () => void
  className?: string
  itemClassName?: string
  moreActionClassName?: string
  renderEmpty?: ReactNode
}

export function RecentList({
  items,
  activeId,
  onSelect,
  moreActionLabel,
  onMoreAction,
  className,
  itemClassName,
  moreActionClassName,
  renderEmpty,
}: RecentListProps) {
  if (items.length === 0 && renderEmpty) {
    return <>{renderEmpty}</>
  }

  return (
    <div className={cn("bm-recent-list", className)}>
      <ul className="bm-recent-list__items">
        {items.map((item) => (
          <li key={item.id} className="bm-recent-list__item">
            <button
              type="button"
              className={cn(
                "bm-recent-list__link",
                activeId === item.id && "bm-recent-list__link--active",
                itemClassName
              )}
              onClick={() => onSelect(item.id)}
            >
              <span className="bm-recent-list__title">{item.title}</span>
              {item.meta ? <span className="bm-recent-list__meta">{item.meta}</span> : null}
            </button>
          </li>
        ))}
      </ul>
      {moreActionLabel && onMoreAction ? (
        <button
          type="button"
          className={cn("bm-recent-list__more", moreActionClassName)}
          onClick={onMoreAction}
        >
          {moreActionLabel}
        </button>
      ) : null}
    </div>
  )
}
