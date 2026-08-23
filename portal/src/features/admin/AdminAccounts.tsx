import { useCallback, useEffect, useRef, useState } from "react"
import type { ApiAdminUser, ApiAdminUserDetail } from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"
import {
  createAdminUser,
  getAdminUser,
  issueAdminLoginCode,
  listAdminUsers,
  revokeAdminUserSessions,
  setAdminUserDisabled,
} from "./api"

const PAGE_SIZE = 50

function accountState(user: ApiAdminUser): { label: string; disabled: boolean } {
  if (user.disabled_at) return { label: "Disabled", disabled: true }
  if (!user.has_password) return { label: "No password yet", disabled: false }
  return { label: "Active", disabled: false }
}

function whenever(rfc3339?: string): string {
  return rfc3339 ? new Date(rfc3339).toLocaleString() : "never"
}

/**
 * AdminAccounts is the page an operator opens on a joiner or a leaver day.
 *
 * The destructive actions state what they will do before they do it. Disabling
 * revokes sessions and stops queued work; a login code is shown once and is
 * recoverable nowhere. Both are said in the confirm, not discovered afterwards.
 */
export function AdminAccounts({ token }: { token: string | null }) {
  const [users, setUsers] = useState<ApiAdminUser[]>([])
  const [total, setTotal] = useState(0)
  const [query, setQuery] = useState("")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [selected, setSelected] = useState<ApiAdminUserDetail | null>(null)
  const detailRef = useRef<HTMLElement | null>(null)

  // The detail panel renders below the list, so on a long list it opens off
  // screen and the click reads as having done nothing.
  useEffect(() => {
    if (selected) detailRef.current?.scrollIntoView({ behavior: "smooth", block: "start" })
  }, [selected])
  const [newEmail, setNewEmail] = useState("")
  const [busy, setBusy] = useState(false)
  const [loginCode, setLoginCode] = useState<string | null>(null)

  const load = useCallback(
    (q: string) => {
      if (!token) return
      setLoading(true)
      setError(null)
      listAdminUsers(token, { q, limit: PAGE_SIZE })
        .then((res) => {
          setUsers(res.users)
          setTotal(res.total)
        })
        .catch((err) => setError(getErrorMessage(err, "Failed to load accounts")))
        .finally(() => setLoading(false))
    },
    [token],
  )

  useEffect(() => {
    load("")
  }, [load])

  function openDetail(userId: string) {
    if (!token) return
    setLoginCode(null)
    getAdminUser(token, userId)
      .then(setSelected)
      .catch((err) => setError(getErrorMessage(err, "Failed to load the account")))
  }

  async function act<T>(run: () => Promise<T>, done: (result: T) => string): Promise<void> {
    if (!token) return
    setBusy(true)
    setError(null)
    setNotice(null)
    try {
      const result = await run()
      setNotice(done(result))
      load(query)
      if (selected) openDetail(selected.id)
    } catch (err) {
      setError(getErrorMessage(err, "The action did not complete"))
    } finally {
      setBusy(false)
    }
  }

  function confirmDisable(user: ApiAdminUser): boolean {
    return window.confirm(
      `Disable ${user.email}?\n\n` +
        "Every credential this account holds stops working immediately: password, " +
        "login code, refresh token, the access token it is already carrying, and its " +
        "webhook keys. Live sessions are revoked and queued work will not start.\n\n" +
        "This is not deletion. Enabling reverses the state and nothing else — " +
        "sessions stay revoked.",
    )
  }

  return (
    <div className="admin-sections">
      <section className="settings-page__section">
        <div className="settings-page__section-head">
          <div>
            <h2 className="settings-page__section-title">Accounts</h2>
            <p className="settings-page__section-copy">
              {total} account{total === 1 ? "" : "s"} in this deployment.
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
            placeholder="Search by email"
            aria-label="Search accounts by email"
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
        {notice ? <p className="admin-notice">{notice}</p> : null}

        {loading ? (
          <p className="admin-empty">Loading…</p>
        ) : users.length === 0 ? (
          <p className="admin-empty">No accounts match.</p>
        ) : (
          <ul className="admin-list">
            {users.map((user) => {
              const state = accountState(user)
              return (
                <li key={user.id} className="admin-list__row">
                  <button
                    type="button"
                    className="admin-list__main admin-list__main--action"
                    onClick={() => openDetail(user.id)}
                  >
                    {user.email}
                  </button>
                  <span
                    className={
                      state.disabled ? "admin-pill admin-pill--bad" : "admin-pill"
                    }
                  >
                    {state.label}
                  </span>
                  <span className="admin-list__meta">
                    last signed in {whenever(user.last_login_at)}
                  </span>
                </li>
              )
            })}
          </ul>
        )}
      </section>

      <section className="settings-page__section">
        <div className="settings-page__section-head">
          <div>
            <h2 className="settings-page__section-title">Create an account</h2>
            <p className="settings-page__section-copy">
              Creating an account gives nobody access. Issue a login code afterwards and
              deliver it over a channel you trust — BuildMax has no mail channel.
            </p>
          </div>
        </div>
        <form
          className="admin-toolbar"
          onSubmit={(e) => {
            e.preventDefault()
            const email = newEmail.trim()
            if (!email) return
            act(
              () => createAdminUser(token!, email),
              (user) => `Created ${user.email}. It has no password yet.`,
            ).then(() => setNewEmail(""))
          }}
        >
          <input
            className="admin-input"
            type="email"
            value={newEmail}
            placeholder="name@example.com"
            aria-label="Email for the new account"
            onChange={(e) => setNewEmail(e.target.value)}
          />
          <button type="submit" className="admin-button" disabled={busy || !newEmail.trim()}>
            Create
          </button>
        </form>
      </section>

      {selected ? (
        <section className="settings-page__section" ref={detailRef}>
          <div className="settings-page__section-head">
            <div>
              <h2 className="settings-page__section-title">{selected.email}</h2>
              <p className="settings-page__section-copy">
                {selected.id} · created {whenever(selected.created_at)} ·{" "}
                {selected.session_count} live session
                {selected.session_count === 1 ? "" : "s"}
              </p>
            </div>
            <button type="button" className="admin-button" onClick={() => setSelected(null)}>
              Close
            </button>
          </div>

          {selected.system_roles.length > 0 ? (
            <p className="admin-notice">
              Holds {selected.system_roles.join(", ")} — this account can operate the
              deployment.
            </p>
          ) : null}

          <div className="admin-facts">
            <div className="admin-fact">
              <span className="admin-fact__label">Spaces</span>
              <span className="admin-fact__value">
                {selected.teams.length === 0
                  ? "none"
                  : selected.teams.map((team) => `${team.name} (${team.role})`).join(", ")}
              </span>
            </div>
          </div>
          <p className="admin-scope-note">
            Spaces are listed by name and role only. Reaching what is in one still
            requires membership.
          </p>

          {loginCode ? (
            <div className="admin-code" role="status">
              <p className="admin-code__label">
                Shown once. It is stored nowhere it can be read back, so a lost code means
                issuing another.
              </p>
              <code className="admin-code__value">{loginCode}</code>
            </div>
          ) : null}

          <div className="admin-actions">
            <button
              type="button"
              className="admin-button"
              disabled={busy || Boolean(selected.disabled_at)}
              onClick={() => {
                if (
                  !window.confirm(
                    `Issue a single-use login code for ${selected.email}?\n\n` +
                      "It is shown once and recoverable nowhere. Deliver it over a channel " +
                      "you trust.",
                  )
                )
                  return
                act(
                  () => issueAdminLoginCode(token!, selected.id),
                  (res) => {
                    setLoginCode(res.code)
                    return `Code issued, valid until ${whenever(res.expires_at)}.`
                  },
                )
              }}
            >
              Issue a login code
            </button>

            <button
              type="button"
              className="admin-button"
              disabled={busy}
              onClick={() => {
                if (
                  !window.confirm(
                    `Sign ${selected.email} out of every device?\n\n` +
                      "Their stored sessions are revoked. An access token they already " +
                      "hold keeps working until it expires.",
                  )
                )
                  return
                act(
                  () => revokeAdminUserSessions(token!, selected.id),
                  (res) => `Revoked ${res.revoked} session token${res.revoked === 1 ? "" : "s"}.`,
                )
              }}
            >
              Revoke sessions
            </button>

            {selected.disabled_at ? (
              <button
                type="button"
                className="admin-button admin-button--primary"
                disabled={busy}
                onClick={() =>
                  act(
                    () => setAdminUserDisabled(token!, selected.id, false),
                    (user) => `${user.email} can sign in again.`,
                  )
                }
              >
                Enable
              </button>
            ) : (
              <button
                type="button"
                className="admin-button admin-button--danger"
                disabled={busy}
                onClick={() => {
                  if (!confirmDisable(selected)) return
                  act(
                    () => setAdminUserDisabled(token!, selected.id, true),
                    (user) =>
                      `${user.email} is disabled. ${user.sessions_revoked} session token` +
                      `${user.sessions_revoked === 1 ? "" : "s"} revoked.`,
                  )
                }}
              >
                Disable
              </button>
            )}
          </div>
        </section>
      ) : null}
    </div>
  )
}
