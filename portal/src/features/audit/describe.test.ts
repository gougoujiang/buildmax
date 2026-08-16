import { describe, expect, it } from "vitest"
import type { ApiAuditEvent } from "../../lib/api/types"
import { actorLabel, describeEvent, formatEventTime } from "./describe"

function event(partial: Partial<ApiAuditEvent>): ApiAuditEvent {
  return {
    audit_event_id: "ae_1",
    team_id: "tm_1",
    actor_type: "user",
    actor_id: "u_1",
    action: "user.login",
    created_at: 1_786_875_000,
    ...partial,
  }
}

describe("describeEvent", () => {
  it("marks a refusal apart from the actions around it", () => {
    // A denial is what shows someone probing at a boundary. Rendering it like
    // a successful action would bury the one entry an owner is looking for.
    const got = describeEvent(event({ action: "access.denied", target_id: "manage_agents" }))
    expect(got.denied).toBe(true)
    expect(got.summary).toContain("manage_agents")
  })

  it("shows an action it does not recognise verbatim", () => {
    // Action strings are permanent and a newer server may write one this
    // Portal predates. Dropping the row, or relabelling it "unknown", hides an
    // audit entry — worse than showing a name the reader can search for.
    const got = describeEvent(event({ action: "something.added.later" }))
    expect(got.summary).toBe("something.added.later")
    expect(got.denied).toBe(false)
  })

  it("uses the detail when there is one to use", () => {
    expect(describeEvent(event({ action: "team.member_added", detail: "admin" })).summary)
      .toContain("admin")
    expect(describeEvent(event({ action: "team.member_added" })).summary)
      .toBe("Added a member")
  })

  it("does not show a target for events whose target is already in the summary", () => {
    // A login's target is the platform, which the sentence already names.
    expect(describeEvent(event({ action: "user.login", target_id: "cli" })).target).toBeNull()
    expect(describeEvent(event({ action: "team.member_removed", target_id: "u_2" })).target).toBe("u_2")
  })
})

describe("actorLabel", () => {
  it("separates a person from the deployment itself", () => {
    // Model catalog changes are recorded as the system, because they run from a
    // shell on the server that no user id was verified for.
    expect(actorLabel(event({ actor_type: "system", actor_id: "buildmax-server" })))
      .toBe("buildmax-server (system)")
    expect(actorLabel(event({ actor_type: "worker", actor_id: "r_1" }))).toBe("r_1 (worker)")
  })

  it("names the reader as themselves", () => {
    expect(actorLabel(event({ actor_id: "u_1" }), "u_1")).toBe("You")
    expect(actorLabel(event({ actor_id: "u_2" }), "u_1")).toBe("u_2")
  })
})

describe("formatEventTime", () => {
  it("shows a dash rather than the epoch when there is no time", () => {
    expect(formatEventTime(0)).toBe("—")
  })
})
