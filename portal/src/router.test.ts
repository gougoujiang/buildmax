import { describe, expect, it } from "vitest"
import { buildHash, parseHash } from "./router"
import type { Route } from "./lib/types"

describe("hash router", () => {
  it.each([
    ["#/", { name: "home" }],
    ["#/login", { name: "login" }],
    ["#/conversation/c_123", { name: "conversation", conversationId: "c_123" }],
    ["#/conversations", { name: "conversations" }],
    ["#/explore", { name: "explore" }],
    ["#/agents", { name: "agents" }],
    ["#/account", { name: "account", section: "general" }],
    ["#/account/usage", { name: "account", section: "usage" }],
    ["#/account/webhook", { name: "account", section: "webhook" }],
    ["#/account/invitations", { name: "account", section: "invitations" }],
    ["#/space", { name: "space", section: "overview" }],
    ["#/space/members", { name: "space", section: "members" }],
    ["#/space/members/new", { name: "space", section: "memberNew" }],
    // A section reachable only by clicking a tab cannot be linked, shared, or
    // survive a reload, so every one of them needs a URL.
    ["#/space/audit", { name: "space", section: "audit" }],
    // Artifacts left space settings for their own area; the old address still
    // lands on them rather than silently falling through to Overview.
    ["#/space/artifacts", { name: "artifacts" }],
    ["#/team-settings", { name: "space", section: "overview" }],
    // Deployment administration is a separate area from space settings, and
    // its sections are linkable for the same reason the space ones are.
    ["#/admin", { name: "admin", section: "overview" }],
    ["#/admin/accounts", { name: "admin", section: "accounts" }],
    ["#/admin/teams", { name: "admin", section: "teams" }],
    ["#/account/plugins", { name: "account", section: "plugins" }],
    ["#/admin/models", { name: "admin", section: "models" }],
    ["#/admin/plugins", { name: "admin", section: "plugins" }],
    ["#/admin/audit", { name: "admin", section: "audit" }],
    ["#/workflows", { name: "workflows" }],
    ["#/workflow/w_123", { name: "workflow", workflowId: "w_123" }],
    ["#/workflow-run/wr_123", { name: "workflowRun", workflowRunId: "wr_123" }],
    ["#/issues", { name: "issues" }],
    ["#/issue/i_123", { name: "issue", issueId: "i_123" }],
    ["#/artifacts", { name: "artifacts" }],
    // No team in the path: an artifact's id is the whole address, matching the
    // API. See docs/design/unified-artifacts.md section 6.1.
    ["#/artifact/gsyt7at6cjfr33d73mta", { name: "artifact", artifactId: "gsyt7at6cjfr33d73mta" }],
  ] satisfies Array<[string, Route]>)("parses %s", (hash, route) => {
    expect(parseHash(hash)).toEqual(route)
  })

  it.each([
    [{ name: "home" }, "#/"],
    [{ name: "login" }, "#/login"],
    [{ name: "conversation", conversationId: "c_123" }, "#/conversation/c_123"],
    [{ name: "conversations" }, "#/conversations"],
    [{ name: "explore" }, "#/explore"],
    [{ name: "agents" }, "#/agents"],
    [{ name: "account", section: "general" }, "#/account"],
    [{ name: "account", section: "usage" }, "#/account/usage"],
    [{ name: "admin", section: "overview" }, "#/admin"],
    [{ name: "admin", section: "accounts" }, "#/admin/accounts"],
    [{ name: "admin", section: "teams" }, "#/admin/teams"],
    [{ name: "account", section: "plugins" }, "#/account/plugins"],
    [{ name: "admin", section: "models" }, "#/admin/models"],
    [{ name: "admin", section: "plugins" }, "#/admin/plugins"],
    [{ name: "admin", section: "audit" }, "#/admin/audit"],
    [{ name: "account", section: "webhook" }, "#/account/webhook"],
    [{ name: "account", section: "invitations" }, "#/account/invitations"],
    [{ name: "space", section: "overview" }, "#/space"],
    [{ name: "space", section: "audit" }, "#/space/audit"],
    [{ name: "space", section: "members" }, "#/space/members"],
    [{ name: "space", section: "memberNew" }, "#/space/members/new"],
    [{ name: "workflows" }, "#/workflows"],
    [{ name: "workflow", workflowId: "w_123" }, "#/workflow/w_123"],
    [{ name: "workflowRun", workflowRunId: "wr_123" }, "#/workflow-run/wr_123"],
    [{ name: "issues" }, "#/issues"],
    [{ name: "issue", issueId: "i_123" }, "#/issue/i_123"],
    [{ name: "artifacts" }, "#/artifacts"],
    [{ name: "artifact", artifactId: "gsyt7at6cjfr33d73mta" }, "#/artifact/gsyt7at6cjfr33d73mta"],
  ] satisfies Array<[Route, string]>)("builds %s", (route, hash) => {
    expect(buildHash(route)).toBe(hash)
  })

  it("falls back to home for unknown hashes", () => {
    expect(parseHash("#/unknown/path")).toEqual({ name: "home" })
  })
})
