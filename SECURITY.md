# Security Policy

## Supported Versions

BuildMax is currently in alpha. Security fixes are applied to `main` and the
most recent release line only. Earlier releases and unmaintained forks may not
receive fixes.

| Version | Supported |
|---|---|
| `main` | Yes |
| Latest release | Yes |
| Older releases | No |

## Reporting a Vulnerability

Do not open a public issue for a suspected vulnerability, leaked credential, or
security-sensitive configuration.

Report privately through GitHub: open the repository's **Security** tab and
choose **Report a vulnerability**. If that form is not available to you, do not
post the report publicly — contact the repository owner through GitHub instead.
Include:

- affected version or commit
- a minimal reproduction or proof of concept
- impact and any prerequisites
- suggested mitigation, when known

Please avoid accessing data that is not yours, disrupting service, or making
changes to third-party systems while investigating. We will acknowledge a
report within seven calendar days, then assess its impact and coordinate a fix
and disclosure with the reporter. Resolution time depends on severity and the
complexity of a safe fix; the acknowledgement is a target, not a commercial
support SLA.

## Known Limitations

These are documented gaps, not vulnerabilities. Please do not file reports for
them; they are tracked as roadmap work.

- **Bootstrap-level authentication.** The server has no mail channel, so signing
  in means an operator issuing a single-use, expiring, per-account code with
  `buildmax-server user login-code`. Self-registration is closed by default.
  There is no password, second factor, SSO, or self-service recovery; a
  deployment serving people outside your organization needs a real identity
  provider in front of it. The optional `dev_login_otp` setting is a fixed code
  that authenticates any registered email address — a development convenience,
  off by default, and a full bypass when enabled.
- **Sandboxing is off by default.** The bash sandbox exists but is not enabled
  on any surface by default, and worker hardening is incomplete. See
  `docs/design/sandbox-boundaries.md` §13.1.

## Deployment Responsibilities

BuildMax can invoke tools and commands through configured agent runtimes.
Operators are responsible for choosing the runtime permissions, sandbox policy,
network policy, workspace access, and credentials appropriate for their
environment. Do not commit API keys, tokens, passwords, private certificates,
or production configuration to this repository. Rotate any credential that is
ever committed or otherwise exposed.
