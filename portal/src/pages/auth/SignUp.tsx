import { useState } from "react"
import { getErrorMessage } from "../../lib/errorMessage"
import { requestOtp, login } from "../../features/auth"
import { useAuth } from "../../contexts/AuthContext"

export function SignUp() {
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
      await requestOtp(email, "signup")
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
      setAuth(res)
    } catch (err) {
      setError(getErrorMessage(err, "Sign up failed"))
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
            <label className="login-page__label" htmlFor="signup-otp">
              OTP
            </label>
            <input
              id="signup-otp"
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
              {loading ? "Signing up…" : "Sign up"}
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
          Already have an account? <a href="#/login">Sign in</a>
        </p>
      </div>
    )
  }

  return (
    <div className="login-page">
      <div className="login-page__card">
        <h1 className="login-page__title">BuildMax</h1>
        <p className="login-page__subtitle">Create an account</p>
        <form onSubmit={handleGetOtp} className="login-page__form">
          <label className="login-page__label" htmlFor="signup-email">
            Email
          </label>
          <input
            id="signup-email"
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
        Already have an account? <a href="#/login">Sign in</a>
      </p>
    </div>
  )
}
