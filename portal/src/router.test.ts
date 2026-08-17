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
    ["#/space", { name: "space", section: "overview" }],
    ["#/space/members", { name: "space", section: "members" }],
    ["#/space/members/new", { name: "space", section: "memberNew" }],
    // A section reachable only by clicking a tab cannot be linked, shared, or
    // survive a reload, so every one of them needs a URL.
    ["#/space/audit", { name: "space", section: "audit" }],
    ["#/team-settings", { name: "space", section: "overview" }],
    ["#/workflows", { name: "workflows" }],
    ["#/workflow/w_123", { name: "workflow", workflowId: "w_123" }],
    ["#/workflow-run/wr_123", { name: "workflowRun", workflowRunId: "wr_123" }],
    ["#/issues", { name: "issues" }],
    ["#/issue/i_123", { name: "issue", issueId: "i_123" }],
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
    [{ name: "account", section: "webhook" }, "#/account/webhook"],
    [{ name: "space", section: "overview" }, "#/space"],
    [{ name: "space", section: "audit" }, "#/space/audit"],
    [{ name: "space", section: "members" }, "#/space/members"],
    [{ name: "space", section: "memberNew" }, "#/space/members/new"],
    [{ name: "workflows" }, "#/workflows"],
    [{ name: "workflow", workflowId: "w_123" }, "#/workflow/w_123"],
    [{ name: "workflowRun", workflowRunId: "wr_123" }, "#/workflow-run/wr_123"],
    [{ name: "issues" }, "#/issues"],
    [{ name: "issue", issueId: "i_123" }, "#/issue/i_123"],
  ] satisfies Array<[Route, string]>)("builds %s", (route, hash) => {
    expect(buildHash(route)).toBe(hash)
  })

  it("falls back to home for unknown hashes", () => {
    expect(parseHash("#/unknown/path")).toEqual({ name: "home" })
  })
})
