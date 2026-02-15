import type { ActivityItem } from "./mockActivity"

interface RecentActivityProps {
  items: ActivityItem[]
}

export function RecentActivity({ items }: RecentActivityProps) {
  return (
    <section className="recent-activity">
      <h2 className="recent-activity__heading">Recent Activity</h2>
      <ul className="recent-activity__list">
        {items.map((item, i) => (
          <li key={i} className="recent-activity__item">
            <span className="recent-activity__title">{item.title}</span>
            <span className="recent-activity__time">({item.time})</span>
          </li>
        ))}
      </ul>
    </section>
  )
}
