import { useState } from 'react';
import { getApp } from './lib/app';

const DEFAULT_SERVER_URL = 'http://localhost:5678';

/**
 * Sign in to a server, or skip it.
 *
 * A server is optional: the agent runs on this machine either way, and local
 * mode is the single-user install — the same thing `buildmax` does in a
 * terminal without `buildmax login`. Signing in adds the account: managed
 * models and a team's work.
 *
 * Of the two ways in, a password is the everyday one. A login code is how a new
 * account is claimed and how a forgotten password is recovered — BuildMax has
 * no mail channel, so an operator issues that code by hand.
 */
export default function LoginPage({ onLogin }) {
  const [mode, setMode] = useState('password');
  const [serverURL, setServerURL] = useState(DEFAULT_SERVER_URL);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [otp, setOtp] = useState('');
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);

  async function handleUseLocalMode() {
    if (loading || !app) return;
    setError(null);
    setLoading(true);
    try {
      const status = await app.UseLocalMode();
      if (onLogin) onLogin(status);
    } catch (err) {
      setError(err?.message ?? String(err));
    } finally {
      setLoading(false);
    }
  }

  const app = getApp();
  const credential = mode === 'password' ? password : otp.trim();

  async function handleSubmit(e) {
    e.preventDefault();
    if (!email.trim() || !credential || loading || !app) return;
    setError(null);
    setLoading(true);
    try {
      const status =
        mode === 'password'
          ? await app.DoLoginWithPassword(serverURL, email.trim(), password)
          : await app.DoLogin(serverURL, email.trim(), otp.trim());
      if (onLogin) onLogin(status);
    } catch (err) {
      setError(err?.message ?? String(err));
    } finally {
      setLoading(false);
    }
  }

  function switchMode() {
    setMode(mode === 'password' ? 'code' : 'password');
    setError(null);
    setPassword('');
    setOtp('');
  }

  return (
    <div className="login-page">
      <div className="login-page__card">
        <h1 className="login-page__title">BuildMax</h1>
        <p className="login-page__subtitle">
          {mode === 'password'
            ? 'Sign in to a BuildMax server, or use this machine on its own'
            : 'Sign in with a login code from your administrator'}
        </p>
        <form onSubmit={handleSubmit} className="login-page__form">
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

          {mode === 'password' ? (
            <>
              <label className="login-page__label" htmlFor="login-password">Password</label>
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
              <label className="login-page__label" htmlFor="login-otp">Login code</label>
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
          <button
            type="submit"
            className="login-page__submit"
            disabled={loading || !email.trim() || !credential}
          >
            {loading ? 'Signing in…' : 'Sign in'}
          </button>
          <button
            type="button"
            className="login-page__link"
            onClick={switchMode}
            disabled={loading}
          >
            {mode === 'password'
              ? 'Forgot your password, or have a login code?'
              : 'Sign in with a password'}
          </button>
        </form>
        <div className="login-page__divider"><span>or</span></div>
        <button
          type="button"
          className="login-page__local"
          onClick={handleUseLocalMode}
          disabled={loading}
        >
          Use BuildMax without a server
        </button>
        <p className="login-page__local-hint">
          The agent runs on this machine, with the models in your settings.yaml.
          You can connect to a server later.
        </p>
      </div>
    </div>
  );
}
