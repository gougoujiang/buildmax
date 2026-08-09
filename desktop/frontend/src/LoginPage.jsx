import { useState } from 'react';

const DEFAULT_SERVER_URL = 'http://localhost:5678';

function getApp() {
  if (typeof window === 'undefined') return null;
  const go = window.go;
  if (!go) return null;
  return go.desktop?.App ?? go.main?.App ?? go.App ?? null;
}

export default function LoginPage({ onLogin }) {
  const [step, setStep] = useState('email');
  const [serverURL, setServerURL] = useState(DEFAULT_SERVER_URL);
  const [email, setEmail] = useState('');
  const [otp, setOtp] = useState('');
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);

  const app = getApp();

  async function handleGetOtp(e) {
    e.preventDefault();
    if (!email.trim() || loading || !app) return;
    setError(null);
    setLoading(true);
    try {
      await app.RequestOTP(serverURL, email.trim(), 'login');
      setStep('otp');
    } catch (err) {
      setError(err?.message ?? String(err));
    } finally {
      setLoading(false);
    }
  }

  async function handleSubmitOtp(e) {
    e.preventDefault();
    if (!otp.trim() || loading || !app) return;
    setError(null);
    setLoading(true);
    try {
      const status = await app.DoLogin(serverURL, email.trim(), otp.trim());
      if (onLogin) onLogin(status);
    } catch (err) {
      setError(err?.message ?? String(err));
    } finally {
      setLoading(false);
    }
  }

  if (step === 'otp') {
    return (
      <div className="login-page">
        <div className="login-page__card">
          <h1 className="login-page__title">BuildMax</h1>
          <p className="login-page__subtitle">Enter OTP sent to {email}</p>
          <form onSubmit={handleSubmitOtp} className="login-page__form">
            <label className="login-page__label" htmlFor="login-otp">OTP</label>
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
            {error && <p className="login-page__error" role="alert">{error}</p>}
            <button type="submit" className="login-page__submit" disabled={loading || !otp.trim()}>
              {loading ? 'Signing in…' : 'Sign in'}
            </button>
            <button
              type="button"
              className="login-page__link"
              onClick={() => { setStep('email'); setOtp(''); setError(null); }}
            >
              Use a different email
            </button>
          </form>
        </div>
      </div>
    );
  }

  return (
    <div className="login-page">
      <div className="login-page__card">
        <h1 className="login-page__title">BuildMax</h1>
        <p className="login-page__subtitle">Sign in to continue</p>
        <form onSubmit={handleGetOtp} className="login-page__form">
          <label className="login-page__label" htmlFor="login-email">Email</label>
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
          <button
            type="button"
            className="login-page__advanced-toggle"
            onClick={() => setShowAdvanced((v) => !v)}
          >
            {showAdvanced ? '▾' : '▸'} Server
          </button>
          {showAdvanced && (
            <>
              <label className="login-page__label" htmlFor="login-server">Server URL</label>
              <input
                id="login-server"
                type="text"
                className="login-page__input"
                value={serverURL}
                onChange={(e) => setServerURL(e.target.value)}
                placeholder={DEFAULT_SERVER_URL}
                disabled={loading}
              />
            </>
          )}
          {error && <p className="login-page__error" role="alert">{error}</p>}
          <button type="submit" className="login-page__submit" disabled={loading || !email.trim()}>
            {loading ? 'Sending…' : 'Get OTP'}
          </button>
        </form>
      </div>
    </div>
  );
}
