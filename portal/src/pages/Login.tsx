import { useState } from "react"
import { getErrorMessage } from "../lib/errorMessage"
import { requestOtp, login } from "../features/auth"
import { useAuth } from "../contexts/AuthContext"

export function Login() {
  const { login: setAuth } = useAuth()
  const [step, setStep] = useState<"email" | "otp">("email")
  const [email, setEmail] = useState("")
  const [otp, setOtp] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  async function handleGetOtp(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      await requestOtp(email, "login")
      setStep("otp")
    } catch (err) {
      setError(getErrorMessage(err, "Request failed"))
    } finally {
      setLoading(false)
    }
  }

  async function handleSubmitOtp(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      const res = await login(email, otp)
      setAuth(res.token, res.user)
    } catch (err) {
      setError(getErrorMessage(err, "Login failed"))
    } finally {
      setLoading(false)
    }
  }

  if (step === "otp") {
    return (
      <div className="login-page">
        <div className="login-page__card">
          <h1 className="login-page__title">BuildMax</h1>
          <p className="login-page__subtitle">Enter OTP sent to {email}</p>
          <form onSubmit={handleSubmitOtp} className="login-page__form">
            <label className="login-page__label" htmlFor="login-otp">
              OTP
            </label>
            <input
              id="login-otp"
              type="text"
              inputMode="numeric"
              autoComplete="one-time-code"
              className="login-page__input"
              value={otp}
              onChange={(e) => setOtp(e.target.value)}
              placeholder="123456"
              disabled={loading}
            />
            {error && (
              <p className="login-page__error" role="alert">
                {error}
              </p>
            )}
            <button
              type="submit"
              className="login-page__submit"
              disabled={loading}
            >
              {loading ? "Signing in…" : "Sign in"}
            </button>
            <button
              type="button"
              className="login-page__link"
              onClick={() => {
                setStep("email")
                setOtp("")
                setError(null)
              }}
            >
              Use a different email
            </button>
          </form>
        </div>
        <p className="login-page__footer">
          <a href="#/signup">Sign up</a> for a new account
        </p>
      </div>
    )
  }

  return (
    <div className="login-page">
      <div className="login-page__card">
        <h1 className="login-page__title">BuildMax</h1>
        <p className="login-page__subtitle">Sign in to continue</p>
        <form onSubmit={handleGetOtp} className="login-page__form">
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
          {error && (
            <p className="login-page__error" role="alert">
              {error}
            </p>
          )}
          <button
            type="submit"
            className="login-page__submit"
            disabled={loading}
          >
            {loading ? "Sending…" : "Get OTP"}
          </button>
        </form>
      </div>
      <p className="login-page__footer">
        Don&apos;t have an account? <a href="#/signup">Sign up</a>
      </p>
    </div>
  )
}
