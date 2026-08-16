import { useCallback, useEffect, useState } from "react"
import type { ApiAuditEvent } from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { getAuditEvents } from "./api"
import { actorLabel, describeEvent, formatEventTime } from "./describe"

const PAGE_SIZE = 50

interface SpaceAuditSectionProps {
  teamId: string | null
  token: string | null
  currentUserIsOwner: boolean
  currentUserId?: string
}

function AuditRow({ event, currentUserId }: { event: ApiAuditEvent; currentUserId?: string }) {
  const described = describeEvent(event)
  return (
    <li className={described.denied ? "audit-row audit-row--denied" : "audit-row"}>
      <div className="audit-row__main">
        <span className="audit-row__actor">{actorLabel(event, currentUserId)}</span>
        <span className="audit-row__summary">{described.summary}</span>
      </div>
      <div className="audit-row__meta">
        {described.target ? <span className="audit-row__target">{described.target}</span> : null}
        <time className="audit-row__time">{formatEventTime(event.created_at)}</time>
      </div>
    </li>
  )
}

/**
 * SpaceAuditSection lists a team's audit trail.
 *
 * Owner only, matching the server. The trail names who was refused, which is
 * administrative rather than collaborative information.
 */
export function SpaceAuditSection({
  teamId,
  token,
  currentUserIsOwner,
  currentUserId,
}: SpaceAuditSectionProps) {
  const [events, setEvents] = useState<ApiAuditEvent[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(
    (offset: number) => {
      if (!teamId || !token) return
      setLoading(true)
      setError(null)
      getAuditEvents(teamId, token, { limit: PAGE_SIZE, offset })
        .then((res) => {
          setEvents((prev) => (offset === 0 ? res.events : [...prev, ...res.events]))
          setTotal(res.total)
        })
        .catch((err) => setError(getErrorMessage(err, "Failed to load the audit trail")))
        .finally(() => setLoading(false))
    },
    [teamId, token]
  )

  useEffect(() => {
    if (!currentUserIsOwner) return
    load(0)
  }, [currentUserIsOwner, load])

  if (!currentUserIsOwner) {
    return (
      <section className="settings-section">
        <h2 className="settings-section__title">Audit trail</h2>
        <p className="settings-section__hint">
          Only a space owner can read the audit trail. It records who was refused a request, which
          is not something the rest of a team needs to see.
        </p>
      </section>
    )
  }

  return (
    <section className="settings-section">
      <h2 className="settings-section__title">Audit trail</h2>
      <p className="settings-section__hint">
        Sign-ins, membership changes, model changes, and refused requests. It records that an
        action happened and who performed it — never prompts, generated content, or credentials.
      </p>

      {error ? (
        <p className="settings-section__error" role="alert">
          {error}
        </p>
      ) : null}

      {!error && events.length === 0 && !loading ? (
        <p className="page-activity__empty">Nothing recorded yet.</p>
      ) : null}

      {events.length > 0 ? (
        <ul className="audit-list">
          {events.map((event) => (
            <AuditRow key={event.audit_event_id} event={event} currentUserId={currentUserId} />
          ))}
        </ul>
      ) : null}

      {loading ? <p className="page-activity__empty">Loading…</p> : null}

      {events.length < total ? (
        <button
          type="button"
          className="page-activity__action-btn"
          onClick={() => load(events.length)}
          disabled={loading}
        >
          Show older ({total - events.length} more)
        </button>
      ) : null}
    </section>
  )
}
