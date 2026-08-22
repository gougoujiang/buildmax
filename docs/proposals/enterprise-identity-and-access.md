# Enterprise Identity And Access

> **Audience:** contributors, operators, and security reviewers · **Status:** proposal — under discussion
>
> **Opened:** 2026-08-16

Related: [../ROADMAP.md](../ROADMAP.md) P4, [team governance design](../design/team-governance.md), and [deployment authentication](../deploy/authentication.md).

## Problem

BuildMax currently uses operator-created accounts and single-use login codes.
That is an explicit alpha bootstrap mechanism, suitable for trusted private
use but not for an organization that needs central account lifecycle control.
Team roles already form the resource boundary, so an enterprise identity path
must map a verified corporate identity into those existing boundaries without
turning local CLI and direct-model use into a Server requirement.

## Goals

- Let an operator connect a private deployment to a corporate identity system.
- Define how identities, teams, and existing `owner`, `admin`, and `member`
  roles relate.
- Make login, access removal, and session invalidation observable and
  testable.
- Preserve the current bootstrap flow for an operator establishing the first
  identity connection.

## Non-Goals

- Public multi-tenant SaaS signup.
- A BuildMax-owned password system, email delivery service, or identity
  directory.
- Custom per-resource ACLs in the first enterprise identity slice.
- Replacing team membership as the authorization boundary.

## Options To Evaluate

| Option | Strength | Main concern |
|---|---|---|
| Require an authenticating reverse proxy | Fast for a single controlled deployment | Identity claims and logout semantics vary by proxy; authorization can become implicit |
| Native OpenID Connect integration | Standard browser flow and explicit claim handling | Requires callback, session, issuer, and key-rotation design |
| SAML-only integration | Common in established enterprises | Higher implementation complexity and a less useful first integration for modern self-hosting |
| OIDC first, with SCIM provisioning later | Separates interactive login from account lifecycle | Needs a clear interim joiner/leaver process |

The likely direction is native OIDC first, with an operator bootstrap account
and documented proxy compatibility. This is not a decision yet; it needs
operator feedback from the identity providers private deployments actually
use.

## Questions To Resolve

- Which claims are stable enough to identify a user: issuer plus subject,
  verified email, or both?
- Is just-in-time account creation permitted, and how does it assign a team?
- How are deprovisioning, role changes, and group-to-team mapping applied?
- Which sessions must become invalid when membership changes or a user leaves?
- What minimum service-account or API-token model is needed for automation?

## Evidence Needed For A Decision

- A threat model covering issuer trust, claim changes, callback handling, and
  account linking.
- An end-to-end test against a standards-compliant OIDC provider.
- A migration story from login-code accounts to externally managed identities.
- A documented fallback for an operator locked out of the identity provider.

## Likely Destination If Accepted

An accepted decision would add an identity plan under `docs/design/`, a P4 or
later roadmap item, configuration reference material, and implementation Issues
for login, provisioning, and authorization tests.
