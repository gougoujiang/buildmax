# Authentication

> **Audience:** operators · **Status:** current

People sign in with an email address and a password. BuildMax has no way to
send email, and everything unusual below follows from that: accounts are created
by an operator, and the one-time codes that claim an account or reset a
forgotten password are delivered by hand.

For the broader alpha support boundaries, see the
[support matrix](../start/support.md).

## Creating An Account

Two commands on the server, and one code you pass along:

```bash
buildmax-server user create alice@example.com
buildmax-server user login-code alice@example.com
```

A new account has no password. The second command prints a code, once:

```text
Login code for alice@example.com:

  bmxlogin_5e9e03467d578f8c248175343d627e814bc3ed10a8a05655a1c500b27dbd17cd

Valid until 2026-08-15T19:22:58+08:00, and only once.
```

Send it over whatever channel you already trust. The person signs in with it —
"Forgot your password, or have a login code?" on the sign-in form — and then
sets a password from account settings. After that they sign in normally and you
are not involved again. `--ttl` changes the code's lifetime, which defaults to
an hour.

To set a password yourself instead, pipe one in rather than passing it as an
argument, which would put it in shell history and in the process list:

```bash
echo -n 'correct horse battery staple' | \
  buildmax-server user set-password alice@example.com
```

Letting the person set their own is better: the password then exists only where
they put it.

Both commands read the same `server.yaml` the server does, so inside a container
they need no extra configuration:

```bash
kubectl exec -n buildmax deploy/buildmax-server -- \
  buildmax-server user login-code alice@example.com
```

### What The Code Is

A code is not a weaker password. It is what an operator vouches for, spent once,
on the way to a password:

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

## Self-Registration Is Closed, And Has No UI

`POST /api/otp/request` refuses `intent: signup` with `403` unless `server.yaml`
sets `allow_signup: true`. Accounts come from `buildmax-server user create`, and
the Portal offers no sign-up form.

Even with `allow_signup: true`, self-registration only creates the account: the
new account has no password, and there is no way to send its owner anything, so
an operator still has to issue a login code. That is why there is no form for
it.

Nothing verifies that whoever types an address controls it, which is the real
reason this stays closed. On a deployment reachable only from a trusted network
open registration may be what you want; on anything else it is how someone
claims a colleague's address. The server logs a warning at startup whenever it
is on.

## Passwords

Stored as an argon2id hash with a per-account salt, so a database dump yields no
usable passwords and nothing that can be looked up in a precomputed table. The
hashing parameters travel inside each stored hash, which means raising them
later applies to new passwords without invalidating existing ones.

The only rule is length: **at least 12 characters**, at most 1024. There is no
"one digit and one symbol" requirement, because composition rules push people
toward short predictable passwords that satisfy them.

Changing a password requires the current one. Setting the *first* password does
not, because someone who just redeemed a login code has none — that is the
recovery flow finishing. A session by itself is deliberately not enough to
change an existing password: an access token cannot be revoked before it
expires, so allowing it would turn a stolen token into a permanent takeover.

Changing a password does **not** sign existing sessions out. Revoke those
separately if that is the intent.

> **Login is not rate limited.** Nothing throttles password attempts, so a
> server anyone can reach can be brute-forced online. The 12-character minimum
> and a memory-hard hash make each guess expensive, but they are not a
> substitute for throttling. Put a rate limiter in front of a deployment that
> untrusted networks can reach. A unified rate-limiting capability is planned
> and not built.

## What Is Still Missing

There is no second factor, no SSO, and no self-service recovery: a forgotten
password means asking an operator for a login code. Nothing verifies that an
email address belongs to the person using it — addresses are identifiers here,
not proof. A deployment serving people outside your organization wants a real
identity provider in front of it; OIDC is planned and not built.

Login attempts are not throttled. See the note under [Passwords](#passwords).

Sessions are revocable but not yet manageable: nothing lists a user's active
sessions, and no command revokes one. An access token cannot be revoked at all.

## The Other Credentials

| Credential | Config | Guards |
|---|---|---|
| **JWT secret** | `jwt_secret` / `BUILDMAX_JWT_SECRET` | Signing for all user access tokens. Required. Generate with `openssl rand -hex 32` and inject at deploy time rather than committing it. |
| **Run token** | minted per run, delivered as `BUILDMAX_RUN_TOKEN` | The `/api/worker/*` routes. Signed with the JWT secret, it names one run's user, team, and task, and authorizes that run alone. Not an operator setting — the scheduler issues one for every dispatched run. Lifetime is `worker.run_token_ttl`; there is no renewal, so it must outlast your longest run. |
| **Worker token** | `worker.token` | Deprecated. Still accepted on `/api/worker/*` for one release, for the upgrade window where a server that has not restarted dispatches a worker expecting a run token. It names no run, so a holder can read any team's task input and write any run's result — which is why it is going away. |
| **Webhook keys** | created per user via the API | Inbound `POST /api/webhook`. Stored as a SHA-256 hash; the plaintext is shown once at creation. See [reference/webhook.md](../reference/webhook.md). |

Rotating the JWT secret invalidates every issued access token at once. Refresh
tokens survive it — they are stored rows, not signatures — so clients exchange
theirs and carry on rather than needing new login codes. That is usually what
you want from a key rotation, but it means the secret is no longer the way to
sign everyone out.

A run token cannot be revoked before it expires either, for the same reason: it
is a signature, not a row. What bounds it instead is scope — one run — and run
status, since the inference route refuses a run that is no longer executing.

There is no operator command to revoke sessions yet. Signing a specific person
out today means deleting their `user_refresh_token` rows in the database and
waiting out any access token they already hold.

## Reporting Problems

Report authentication or authorization vulnerabilities privately as described in
[SECURITY.md](../../SECURITY.md). Do not open a public issue.
