import type { ApiAuditEvent } from "../../lib/api/types"

/** How one event should read in the trail. */
export interface AuditEventDescription {
  /** A sentence naming what happened, without the actor. */
  summary: string
  /** Denials get a distinct treatment: they are what shows someone probing. */
  denied: boolean
  /** Present when the event names an object worth showing. */
  target: string | null
}

/**
 * describeEvent turns a stored action into something readable.
 *
 * Actions are permanent strings, so an unknown one is a real possibility: a
 * newer server writing an action this Portal predates. It is shown verbatim
 * rather than dropped or relabelled "unknown", because hiding an audit entry a
 * reader cannot interpret is worse than showing them a name they can search
 * for.
 */
export function describeEvent(event: ApiAuditEvent): AuditEventDescription {
  const target = event.target_id ? event.target_id : null
  switch (event.action) {
    case "user.login":
      return { summary: `Signed in from ${event.target_id || "an unknown platform"}`, denied: false, target: null }
    case "team.member_added":
      return {
        summary: event.detail ? `Added a member as ${event.detail}` : "Added a member",
        denied: false,
        target,
      }
    case "team.member_removed":
      return { summary: "Removed a member", denied: false, target }
    case "llm_model.created":
      return {
        summary: event.detail ? `Added the model ${event.detail}` : "Added a model",
        denied: false,
        target,
      }
    case "llm_model.enabled":
      return { summary: "Enabled a model", denied: false, target }
    case "llm_model.disabled":
      return { summary: "Disabled a model", denied: false, target }
    case "user.logout":
      return { summary: "Signed out", denied: false, target: null }
    case "user.password_set":
      return { summary: "Set a password", denied: false, target: null }
    case "auth.refresh_reuse":
      return {
        summary: "A refresh token was presented twice; the session was revoked",
        denied: true,
        target,
      }
    case "user.created":
      return { summary: "Created an account", denied: false, target }
    case "user.login_code_issued":
      return { summary: "Issued a login code", denied: false, target }
    case "user.disabled":
      return { summary: "Disabled an account", denied: false, target }
    case "user.enabled":
      return { summary: "Enabled an account", denied: false, target }
    case "user.sessions_revoked":
      return { summary: "Revoked every session of an account", denied: false, target }
    case "system.admin_granted":
      return {
        summary: event.detail
          ? `Granted ${event.detail} over the deployment`
          : "Granted deployment authority",
        denied: false,
        target,
      }
    case "system.admin_revoked":
      return {
        summary: event.detail
          ? `Revoked ${event.detail} over the deployment`
          : "Revoked deployment authority",
        denied: false,
        target,
      }
    case "access.denied":
      return {
        summary: event.target_id ? `Was refused: ${event.target_id}` : "Was refused a request",
        denied: true,
        target: null,
      }
    default:
      return { summary: event.action, denied: false, target }
  }
}

/** actorLabel names who acted, distinguishing a person from the deployment. */
export function actorLabel(event: ApiAuditEvent, currentUserId?: string): string {
  if (event.actor_type === "system") return `${event.actor_id} (system)`
  if (event.actor_type === "worker") return `${event.actor_id} (worker)`
  if (currentUserId && event.actor_id === currentUserId) return "You"
  return event.actor_id
}

export function formatEventTime(seconds: number): string {
  if (!seconds) return "—"
  return new Date(seconds * 1000).toLocaleString()
}
