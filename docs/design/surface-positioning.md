# Agent Core / CLI / Desktop / Portal Product Positioning

## Status

- status: `current_decision`
- created_at: `2026-05-10`
- context: product surface positioning for a shared Agent core, local entry points, and enterprise Portal deployment

---

## 1. Purpose

BuildMax has one foundational Agent runtime and three user-facing surfaces:

- Agent Core
- CLI/TUI
- Desktop
- Portal

The current product decision is:

> BuildMax is an out-of-the-box, privately deployable enterprise Agent platform
> powered by one shared Agent core. CLI and Desktop expose direct local
> single-agent capability; Portal operationalizes the same capability for teams
> with collaboration, workflows, governance, and results.

This is not a choice between "local Agent" and "team workspace." Users can use
only local surfaces, deploy Portal for a company, or use both together.

---

## 2. Product Layers

### 2.1 Agent Core: Foundation

Agent Core is the root product capability.

Primary job:

- run the shared LLM/tool-calling loop
- provide file-aware execution
- assemble tools, MCP, skills, subagents, sessions, and model configuration
- support both local interactive use and remote worker task execution

All important Agent capabilities should land here first, or be shaped so they
can be shared here. CLI, Desktop, and worker task runs should not drift into
separate Agent implementations.

### 2.2 CLI: Local Agent Executor

The CLI is the fastest local execution surface.

Primary job:

- run an agent against the current local workspace
- support terminal-native workflows
- support prompt mode, sessions, scripting, and developer automation

The CLI should remain:

- direct
- composable
- local-first
- comfortable for technical users

### 2.3 Desktop: Local Personal Agent Workbench

Desktop is the local personal workbench for the same Agent Core.

Primary job:

- provide a richer local UI around the same local execution role as CLI
- make local sessions, local workspaces, output review, and result handling easier than the terminal
- connect to Portal where useful, without inheriting Portal's management scope

Desktop should feel like:

> a comfortable local place to use the Agent Core

not:

> a second place to administer team work.

### 2.4 Portal: Enterprise Operation Layer

Portal is the enterprise/team operation layer over the same Agent Core.

Primary job:

- Team
- Issue
- Workflow
- Agent definitions
- Team files
- Results and artifacts
- Governance

Portal owns the team operating model:

- manage teams, members, roles, and shared resources
- manage issues as the user-facing work object
- define, publish, and inspect workflows
- track task runs, artifacts, usage, and eventually audit/history
- provide the canonical team workspace view

---

## 3. Positioning Decision

Desktop should **not** become a local clone of Portal.

Full Portal alignment would imply Desktop needs to manage:

- issues
- workflows
- team settings
- members
- roles
- governance
- cloud execution history

That exceeds the natural scope of a local app and creates product confusion:

- users would have two places to manage the same team objects
- Desktop would become a local Portal replica
- local-first value would be diluted by cloud administration features
- implementation complexity would rise without clarifying the user promise

Desktop should instead align with CLI on the direct Agent-use axis:

- local workspace
- local agent execution
- local sessions
- local tools
- local files

But Desktop should provide a more visual and persistent experience than CLI.

Portal should align with the same Agent Core on execution capability. Its added
value is enterprise operation: organizing, reusing, governing, and observing
that capability across teams.

---

## 4. Functional Matrix

| Capability | Agent Core | CLI | Desktop | Portal |
| --- | --- | --- | --- | --- |
| Agent tool-calling execution | Strong | Strong | Strong | Strong via worker |
| Local workspace agent execution | Foundation | Strong | Strong | Not primary |
| Terminal scripting and automation | Foundation | Strong | Weak | None |
| Local session history | Foundation | Medium | Strong | Separate team conversations |
| Local file context and edits | Foundation | Strong | Strong | Team file space, not local disk |
| Rich result viewing | Foundation | Medium | Strong local | Strong team/cloud |
| Team and member management | None | None | None | Strong |
| Issue management | None | None | Lightweight inbox / launcher only | Strong |
| Workflow authoring | None | None | None | Strong |
| Workflow triggering | Execution substrate | Possible later | Optional, for published workflows | Strong |
| Task/run visibility | Runtime data | Weak | Optional assigned subset | Strong |
| Artifact browsing | Runtime data | Local/simple | Strong local plus optional cloud outputs | Strong cloud outputs |
| Offline/private local use | Supports | Strong | Strong | Deployment-dependent |
| Governance, quota, audit | Hooks/contracts | None | Read-only hints at most | Strong |

---

## 5. Desktop Scope

### 5.1 In Scope

Desktop should focus on:

- local chat sessions
- local workspace selection
- local file-aware agent execution
- session list/detail/search
- streaming replies
- local result and artifact viewing
- local settings and model selection
- local MCP/tool configuration where appropriate
- optional login to Portal for identity and sync
- optional "assigned work inbox" from Portal
- optional ability to attach local work results back to a Portal issue

### 5.2 Out Of Scope

Desktop should not own:

- full issue tracker management
- workflow authoring or lifecycle management
- team membership management
- role and permission administration
- quota administration
- audit/event administration
- cloud workspace administration
- being a full offline copy of Portal

### 5.3 Bridge Scope

Desktop may bridge to Portal in narrow ways:

- show issues assigned to me as an inbox
- open a Portal issue in the browser
- start a local session from an assigned issue
- attach or summarize local results back to an issue
- trigger a published workflow if that helps local work
- show cloud result links related to my local task

The bridge should be framed as:

> receive cloud work, do local work, send results back

not:

> administer the cloud workspace locally.

---

## 6. Product Mental Model

Recommended product explanation:

> BuildMax is a privately deployable enterprise Agent platform powered by one
> shared Agent core. CLI and Desktop expose that core locally; Portal
> operationalizes it for teams.

Slightly longer version:

> BuildMax gives companies practical Agent capability out of the box. A user can
> run the Agent locally through CLI or Desktop, while a company can deploy Portal
> to organize the same capability into teams, issues, workflows, shared files,
> results, quota, and governance.

Surface-specific version:

> CLI is the fastest local Agent interface. Desktop is the local personal Agent
> workbench. Portal is the enterprise operation layer for the same Agent core.

Desktop-specific sentence:

> Desktop is the local AI workbench for personal Agent execution. It can connect
> to Portal for identity and assigned work, but it does not manage the team
> operating system.

---

## 7. Roadmap Implications

### 7.1 Near-Term Desktop Priorities

Prefer features that strengthen local execution:

1. Better session management.
2. Workspace picker and recent workspaces.
3. Rich chat and streaming polish.
4. Local output/result viewer.
5. Local file/diff awareness.
6. Model and local tool settings.
7. Portal login as an optional connector.

### 7.2 Agent Core Priorities

Prefer features that strengthen every surface at once:

1. More reliable tool execution and context-window handling.
2. MCP/tool configuration that works locally and in worker execution.
3. Skills and subagents as shared runtime capabilities.
4. Better artifact/result production conventions.
5. Safer file editing and diff visibility.
6. Runtime observability needed by both local and Portal workflows.

### 7.3 Portal Bridge Priorities

After the local workbench feels solid, add narrow cloud bridge features:

1. Assigned issue inbox.
2. Start local session from issue.
3. Send summary/result back to issue.
4. Link local session to cloud issue.
5. Open cloud issue/workflow/result in Portal.

### 7.4 Avoided Roadmap Items

Avoid adding these to Desktop unless the product direction changes explicitly:

- full issue CRUD
- workflow builder
- team settings
- member management
- quota dashboard
- audit log UI
- duplicated Portal navigation tree

These belong in Portal.

---

## 8. Decision Summary

BuildMax should be Agent-core-first.

CLI and Desktop show the direct local Agent experience. Portal packages the same
Agent capability into an enterprise platform for teams, workflows, governance,
and results. Desktop may integrate with Portal, but the integration should be
intentionally narrow: receive work from the team system, execute locally when
appropriate, and return results.

Portal remains the source of truth for team collaboration, issue/workflow management, shared results, and governance.
