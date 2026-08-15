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

## The Other Credentials

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
