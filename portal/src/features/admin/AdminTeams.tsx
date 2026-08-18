import { useCallback, useEffect, useRef, useState } from "react"
import type { ApiAdminTeam, ApiAdminTeamDetail } from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import { getAdminTeam, listAdminTeams } from "./api"

const PAGE_SIZE = 50

/**
 * AdminTeams shows every space in the deployment as metadata.
 *
 * There is deliberately nothing here to click through into. An administrator
 * learns that a space exists, how large it is, and what it is using; reaching
 * what is in it still requires membership. A link that 403s would read as a bug
 * rather than as a boundary, so there is no link.
 */
export function AdminTeams({ token }: { token: string | null }) {
  const [teams, setTeams] = useState<ApiAdminTeam[]>([])
  const [total, setTotal] = useState(0)
  const [query, setQuery] = useState("")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<ApiAdminTeamDetail | null>(null)
  const detailRef = useRef<HTMLElement | null>(null)

  // The detail panel renders below the list, so on a long list it opens off
  // screen and the click reads as having done nothing.
  useEffect(() => {
    if (selected) detailRef.current?.scrollIntoView({ behavior: "smooth", block: "start" })
  }, [selected])

  const load = useCallback(
    (q: string) => {
      if (!token) return
      setLoading(true)
      setError(null)
      listAdminTeams(token, { q, limit: PAGE_SIZE })
        .then((res) => {
          setTeams(res.teams)
          setTotal(res.total)
        })
        .catch((err) => setError(getErrorMessage(err, "Failed to load spaces")))
        .finally(() => setLoading(false))
    },
    [token],
  )

  useEffect(() => {
    load("")
  }, [load])

  return (
    <div className="admin-sections">
      <section className="settings-page__section">
        <div className="settings-page__section-head">
          <div>
            <h2 className="settings-page__section-title">Spaces</h2>
            <p className="settings-page__section-copy">
              {total} space{total === 1 ? "" : "s"}, including one personal space per
              account. Metadata only — never their contents.
            </p>
          </div>
        </div>

        <form
          className="admin-toolbar"
          onSubmit={(e) => {
            e.preventDefault()
            load(query)
          }}
        >
          <input
            className="admin-input"
            type="search"
            value={query}
            placeholder="Search by name"
            aria-label="Search spaces by name"
            onChange={(e) => setQuery(e.target.value)}
          />
          <button type="submit" className="admin-button" disabled={loading}>
            Search
          </button>
        </form>

        {error ? (
          <p className="settings-section__error" role="alert">
            {error}
          </p>
        ) : null}

        {loading ? (
          <p className="admin-empty">Loading…</p>
        ) : teams.length === 0 ? (
          <p className="admin-empty">No spaces match.</p>
        ) : (
          <ul className="admin-list">
            {teams.map((team) => (
              <li key={team.team_id} className="admin-list__row">
                <button
                  type="button"
                  className="admin-list__main admin-list__main--action"
                  onClick={() => {
                    if (!token) return
                    getAdminTeam(token, team.team_id)
                      .then(setSelected)
                      .catch((err) => setError(getErrorMessage(err, "Failed to load the space")))
                  }}
                >
                  {team.name}
                </button>
                {team.personal ? <span className="admin-pill">personal</span> : null}
                <span className="admin-list__meta">
                  {team.member_count} member{team.member_count === 1 ? "" : "s"}
                  {team.quota_tier ? ` · ${team.quota_tier}` : ""}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>

      {selected ? (
        <section className="settings-page__section" ref={detailRef}>
          <div className="settings-page__section-head">
            <div>
              <h2 className="settings-page__section-title">{selected.name}</h2>
              <p className="settings-page__section-copy">{selected.team_id}</p>
            </div>
            <button type="button" className="admin-button" onClick={() => setSelected(null)}>
              Close
            </button>
          </div>

          {selected.usage ? (
            <div className="admin-facts">
              <div className="admin-fact">
                <span className="admin-fact__label">Tier</span>
                <span className="admin-fact__value">{selected.usage.tier || "unknown"}</span>
              </div>
              <div className="admin-fact">
                <span className="admin-fact__label">Runs this period</span>
                <span className="admin-fact__value">
                  {selected.usage.run_count}
                  {selected.usage.max_runs_per_period !== undefined
                    ? ` / ${selected.usage.max_runs_per_period}`
                    : ""}
                </span>
              </div>
              <div className="admin-fact">
                <span className="admin-fact__label">Tokens this period</span>
                <span className="admin-fact__value">
                  {selected.usage.total_tokens}
                  {selected.usage.max_tokens_per_period !== undefined
                    ? ` / ${selected.usage.max_tokens_per_period}`
                    : ""}
                </span>
              </div>
              <div className="admin-fact">
                <span className="admin-fact__label">Period</span>
                <span className="admin-fact__value">{selected.usage.period_days} days</span>
              </div>
            </div>
          ) : (
            <p className="admin-empty">This deployment reports no quota.</p>
          )}

          <ul className="admin-list">
            {selected.members.map((member) => (
              <li key={member.user_id} className="admin-list__row">
                <span className="admin-list__main">{member.email || member.user_id}</span>
                <span className="admin-pill">{member.role}</span>
              </li>
            ))}
          </ul>
          <p className="admin-scope-note">
            Members and capacity, not work. Issues, conversations, files, artifacts, and
            run traces stay behind membership.
          </p>
        </section>
      ) : null}
    </div>
  )
}
