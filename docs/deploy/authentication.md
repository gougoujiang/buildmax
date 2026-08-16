# Authentication

> **Audience:** operators · **Status:** current

BuildMax has no way to send email. Everything below follows from that: accounts
are created by an operator, and login codes are delivered by hand.

For the broader alpha support boundaries, see the
[support matrix](../start/support.md).

## How Someone Signs In

Two commands on the server, and one code you pass along:

```bash
buildmax-server user create alice@example.com
buildmax-server user login-code alice@example.com
```

The second prints a code, once:

```text
Login code for alice@example.com:

  bmxlogin_5e9e03467d578f8c248175343d627e814bc3ed10a8a05655a1c500b27dbd17cd

Valid until 2026-08-15T19:22:58+08:00, and only once.
```

Send it over whatever channel you already trust, and the person enters their
email and that code in the Portal. `--ttl` changes the lifetime, which defaults
to an hour.

Both commands read the same `server.yaml` the server does, so inside a container
they need no extra configuration:

```bash
kubectl exec -n buildmax deploy/buildmax-server -- \
  buildmax-server user login-code alice@example.com
```

### What The Code Is

- **Single-use.** Redeeming it spends it, whether or not the sign-in that
  follows succeeds. Entering the wrong email address burns the code — issue
  another; that is cheaper than leaving a window for someone to retry a code
  they found.
- **Expiring.** An hour by default.
- **Bound to one account.** The code identifies the user, and the email in the
  request must match it. A code cannot sign anyone into a different account.
- **Stored as a SHA-256 hash.** A database backup yields no usable codes, and a
  lost code cannot be read back — issue a new one.

## What Signing In Returns

Two credentials, not one:

| | Lives | Stored on the server | Revocable |
|---|---|---|---|
| **Access token** | 7 days (`access_token_ttl`) | No — a signed JWT | No; it works until it expires |
| **Refresh token** | 30 days (`refresh_token_ttl`) | Yes, as a hash in `user_refresh_token` | Yes, immediately |

The access token goes with every request. The refresh token goes to
`POST /api/token/refresh` and nowhere else, and comes back replaced: each
exchange spends the one presented and issues the next. Because it is a stored
row, `POST /api/logout` can retire it — which is the difference that matters,
since nothing can retire an access token early.

Each login opens its own session. Signing in from a laptop does not disturb a
session on a phone, and logging one out leaves the other alone.

### Reuse Ends The Session

A refresh token presented after it was already exchanged means two copies exist.
The server cannot tell which holder is the legitimate one, so it revokes the
whole session — the honest holder is signed out too. An `auth.refresh_reuse`
audit event records it.

`refresh_rotation_grace` (default 30 seconds) is the one exemption. The CLI and
Desktop share one credentials file across processes, so two of them refreshing
in the same moment is ordinary rather than suspicious; inside that window both
get a usable token. Raising it widens the window in which a stolen token goes
unnoticed.

### What This Does Not Do

Rotating a refresh token does not shorten the access token already issued from
it. A leaked access token works for its full lifetime no matter what happens to
the session behind it — there is no revocation list. Shortening
`access_token_ttl` is the only control over that window, and it costs nothing
but refresh traffic.

## Signup Is Closed By Default

`POST /api/otp/request` refuses `intent: signup` with `403` unless `server.yaml`
sets `allow_signup: true`. Accounts come from `buildmax-server user create`.

Opening it means anyone who can reach the server can create an account. On a
deployment reachable only from a trusted network that may be what you want; on
anything else it is how a server becomes someone else's. The server logs a
warning at startup whenever it is on.

## The Development Fixed Code

`dev_login_otp` in `server.yaml` (or `BUILDMAX_DEV_LOGIN_OTP`) makes one code
work for every account:

```yaml
dev_login_otp: "123456"
```

> **One code signs in every registered account.** Anyone who knows a registered
> email address can authenticate as that user.

It is an authentication bypass, not a credential — it exists so a developer can
click through the Portal without issuing codes. Single-use codes replace it
entirely; leave it unset. The server logs a warning on every startup where it is
enabled.

## What Is Still Missing

Login codes are a bootstrap mechanism, not an identity system. There is no
password, no second factor, no SSO, and no self-service recovery — losing access
means asking an operator for another code. A deployment serving people outside
your organization wants a real identity provider in front of it.

Sessions are revocable but not yet manageable: nothing lists a user's active
sessions, and no command revokes one. An access token cannot be revoked at all.

## The Other Credentials

| Credential | Config | Guards |
|---|---|---|
| **JWT secret** | `jwt_secret` / `BUILDMAX_JWT_SECRET` | Signing for all user access tokens. Required. Generate with `openssl rand -hex 32` and inject at deploy time rather than committing it. |
| **Worker token** | `worker.token` | The `/api/worker/*` routes that workers use to fetch and update their run. A worker with this token can read and update task runs by id. |
| **Webhook keys** | created per user via the API | Inbound `POST /api/webhook`. Stored as a SHA-256 hash; the plaintext is shown once at creation. See [reference/webhook.md](../reference/webhook.md). |

Rotating the JWT secret invalidates every issued access token at once. Refresh
tokens survive it — they are stored rows, not signatures — so clients exchange
theirs and carry on rather than needing new login codes. That is usually what
you want from a key rotation, but it means the secret is no longer the way to
sign everyone out.

There is no operator command to revoke sessions yet. Signing a specific person
out today means deleting their `user_refresh_token` rows in the database and
waiting out any access token they already hold.

## Reporting Problems

Report authentication or authorization vulnerabilities privately as described in
[SECURITY.md](../../SECURITY.md). Do not open a public issue.
