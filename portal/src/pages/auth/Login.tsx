import { useState } from "react"
import { getErrorMessage } from "../../lib/errorMessage"
import { login, loginWithPassword } from "../../features/auth"
import { useAuth } from "../../contexts/AuthContext"

/**
 * Two ways in, for two different jobs.
 *
 * A password is the everyday one. A login code is how a new account is claimed
 * and how a forgotten password is recovered — BuildMax has no mail channel, so
 * an operator issues that code by hand and passes it along. There is no "send
 * me a code" button, because nothing would send it.
 */
export function Login() {
  const { login: setAuth } = useAuth()
  const [mode, setMode] = useState<"password" | "code">("password")
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [otp, setOtp] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      // Trimmed to match canSubmit, and because both fields are pasted: a code
      // copied out of a terminal carries the indentation of its line. The
      // server trims too; this keeps the button's idea of "filled in" and the
      // request the same.
      const res =
        mode === "password"
          ? await loginWithPassword(email.trim(), password)
          : await login(email.trim(), otp.trim())
      setAuth(res)
    } catch (err) {
      setError(getErrorMessage(err, "Sign in failed"))
    } finally {
      setLoading(false)
    }
  }

  function switchTo(next: "password" | "code") {
    setMode(next)
    setError(null)
    setPassword("")
    setOtp("")
  }

  const canSubmit = email.trim() !== "" && (mode === "password" ? password !== "" : otp.trim() !== "")

  return (
    <div className="login-page">
      <div className="login-page__card">
        <h1 className="login-page__title">BuildMax</h1>
        <p className="login-page__subtitle">
          {mode === "password"
            ? "Sign in to continue"
            : "Sign in with a login code from your administrator"}
        </p>
        <form onSubmit={handleSubmit} className="login-page__form">
          <label className="login-page__label" htmlFor="login-email">
            Email
          </label>
          <input
            id="login-email"
            type="email"
            className="login-page__input"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            autoComplete="email"
            disabled={loading}
          />

          {mode === "password" ? (
            <>
              <label className="login-page__label" htmlFor="login-password">
                Password
              </label>
              <input
                id="login-password"
                type="password"
                className="login-page__input"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                autoComplete="current-password"
                disabled={loading}
              />
            </>
          ) : (
            <>
              <label className="login-page__label" htmlFor="login-otp">
                Login code
              </label>
              <input
                id="login-otp"
                type="text"
                className="login-page__input"
                value={otp}
                onChange={(e) => setOtp(e.target.value)}
                placeholder="bmxlogin_…"
                autoComplete="one-time-code"
                required
                disabled={loading}
              />
            </>
          )}

          {error && (
            <p className="login-page__error" role="alert">
              {error}
            </p>
          )}
          <button
            type="submit"
            className="login-page__submit"
            disabled={loading || !canSubmit}
          >
            {loading ? "Signing in…" : "Sign in"}
          </button>
          <button
            type="button"
            className="login-page__link"
            onClick={() => switchTo(mode === "password" ? "code" : "password")}
            disabled={loading}
          >
            {mode === "password"
              ? "Forgot your password, or have a login code?"
              : "Sign in with a password"}
          </button>
        </form>
      </div>
      {mode === "code" && (
        <p className="login-page__footer">
          Ask an administrator for a code. Once you are in, set a password from
          account settings.
        </p>
      )}
    </div>
  )
}
