import type { FormModalSelectOption } from "@buildmax/gui"

// Mirrors config.SandboxNetworkTier / config.SandboxFilesystemTier in the Go
// backend. See docs/design/agent-sandbox-policy.md.

const NETWORK_TIERS: FormModalSelectOption[] = [
  { value: "none", label: "None", description: "No network access." },
  {
    value: "registries",
    label: "Registries",
    description: "Package registries and the domains they redirect to, nothing else.",
  },
  { value: "open", label: "Open", description: "Unrestricted outbound network access." },
]

const FILESYSTEM_TIERS: FormModalSelectOption[] = [
  {
    value: "workspace",
    label: "Workspace only",
    description: "Reads and writes are confined to the run's own workspace.",
  },
  {
    value: "workspace_plus_shared_read",
    label: "Workspace + shared read",
    description: "Workspace read/write, plus read access to team-shared paths.",
  },
  {
    value: "workspace_plus_external_write",
    label: "Workspace + external write",
    description: "Workspace read/write, plus write access outside it.",
  },
]

/** An agent that declares nothing inherits the team's default, and only then
 * falls through to the strictest baseline -- so the first option here is
 * "inherit," not a hardcoded tier. */
export const AGENT_SANDBOX_NETWORK_TIER_OPTIONS: FormModalSelectOption[] = [
  {
    value: "",
    label: "Team default",
    description: "Inherit this team's default network tier (or the strictest baseline if the team sets none).",
  },
  ...NETWORK_TIERS,
]

export const AGENT_SANDBOX_FILESYSTEM_TIER_OPTIONS: FormModalSelectOption[] = [
  {
    value: "",
    label: "Team default",
    description: "Inherit this team's default filesystem tier (or the strictest baseline if the team sets none).",
  },
  ...FILESYSTEM_TIERS,
]

/** A team's own default has no further tier to inherit from -- leaving it
 * unset means the strictest baseline applies to every agent that declares
 * nothing. */
export const TEAM_SANDBOX_NETWORK_TIER_OPTIONS: FormModalSelectOption[] = [
  {
    value: "",
    label: "No default (strictest baseline)",
    description: "An agent that declares no network tier runs under the strictest baseline.",
  },
  ...NETWORK_TIERS,
]

export const TEAM_SANDBOX_FILESYSTEM_TIER_OPTIONS: FormModalSelectOption[] = [
  {
    value: "",
    label: "No default (strictest baseline)",
    description: "An agent that declares no filesystem tier runs under the strictest baseline.",
  },
  ...FILESYSTEM_TIERS,
]
