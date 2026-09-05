import type { ReactNode } from "react"
import type { FormModalGroup } from "@buildmax/gui"

// The configuration sections shared by the create dialog and the inline detail-
// page editor. The create dialog lays these out as FormModal tabs (a left
// sidebar); the detail page renders the same groups as its own sidebar. Keeping
// the id/title/description in one place stops the two surfaces from drifting.
// The plugin and secret groups' content (their editors) is injected per surface
// via buildAgentGroups, because it needs that surface's live state.
export const AGENT_GROUP_META: FormModalGroup[] = [
  { id: "basics", title: "Basics" },
  {
    id: "sandbox",
    title: "Sandbox access",
    description:
      "Restrict what this agent's runs can reach. Leave on the team default unless this agent needs something different.",
  },
  {
    id: "plugins",
    title: "Plugins",
    description:
      "Catalog plugins this agent loads for background runs. Nothing is inherited — an agent that names none loads none.",
  },
  {
    id: "secrets",
    title: "Secrets",
    description: "Grant Team Secrets to this agent's runs as environment variables.",
  },
]

/**
 * buildAgentGroups injects the live plugin and secret editors into their groups,
 * so a FormModal-driven surface (the create dialog) can render them as tab bodies.
 */
export function buildAgentGroups(opts: {
  pluginEditor: ReactNode
  secretEditor: ReactNode
}): FormModalGroup[] {
  return AGENT_GROUP_META.map((group) => {
    if (group.id === "plugins") return { ...group, content: opts.pluginEditor }
    if (group.id === "secrets") return { ...group, content: opts.secretEditor }
    return group
  })
}
