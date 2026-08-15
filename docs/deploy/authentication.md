# Authentication

> **Audience:** operators · **Status:** current — this describes a known gap

**BuildMax server authentication is not production-ready. Read this before
exposing a server.**

For the broader alpha support boundaries, see the
[support matrix](../start/support.md).

## Current State

`POST /api/login` verifies a one-time password, but BuildMax has **no OTP
delivery channel** — nothing sends the code to an email address or phone. As a
result:

- with no OTP verifier configured, `POST /api/login` is **disabled** and
  returns `503`
- the only supported verifier is a single fixed development code

Once a user has logged in, the rest of the user API is ordinary JWT bearer auth,
and team membership is the authorization boundary for every team-scoped route.

## The Development Code

Set `dev_login_otp` in `server.yaml`, or `BUILDMAX_DEV_LOGIN_OTP` in the
environment:

```yaml
# <BUILDMAX_HOME>/server.yaml
dev_login_otp: "123456"
```

Understand exactly what this does:

> **One code signs in every registered account.** Anyone who knows a registered
> email address can authenticate as that user.

It is a full authentication bypass, not a weak password. Use it only on a
laptop or an otherwise trusted network. The server logs a warning on every
startup where it is enabled. Leave it unset in any deployment reachable by
untrusted users — the resulting `503` is the safe state.

Putting the Portal on the public internet requires wiring a real identity
provider first.

## The Other Two Credentials

| Credential | Config | Guards |
|---|---|---|
| **JWT secret** | `jwt_secret` / `BUILDMAX_JWT_SECRET` | Signing for all user API tokens. Required. Generate with `openssl rand -hex 32` and inject at deploy time rather than committing it. |
| **Worker token** | `worker.token` | The `/api/worker/*` routes that workers use to fetch and update their run. A worker with this token can read and update task runs by id. |
| **Webhook keys** | created per user via the API | Inbound `POST /api/webhook`. Stored as a SHA-256 hash; the plaintext is shown once at creation. See [reference/webhook.md](../reference/webhook.md). |

Rotating the JWT secret invalidates every issued token, which is the intended
way to force everyone to sign in again.

## Reporting Problems

Report authentication or authorization vulnerabilities privately as described in
[SECURITY.md](../../SECURITY.md). Do not open a public issue.
