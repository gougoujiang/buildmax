# Design 083: Agent detail page (edit and delete agent)

## Goal

Enable users to edit and delete existing workspace agents from the Portal: click an agent card → edit dialog (name, description, instructions) → save via PATCH or delete via DELETE; list updates.

## Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **internal/storage/entity** | Agent persistence | AgentStore interface, Store Get/List/Create/UpdateAgent; add DeleteAgent. |
| **internal/server** | Agent HTTP API | agents.go handlers (list, create, get, patch, delete); server.go routes; openapi.json. |
| **portal/src/lib/api** | Backend API client | getAgents, createAgent, updateAgent, deleteAgent; apiAgentToAgent. |
| **portal/src/components** | Modals and shared UI | CreateEntityModal (initialValues); CreateAgentModal; EditAgentModal (edit + Delete button). |
| **portal/src/pages** | Agents list page | AgentList.tsx: edit state, card click, edit modal, onSave, onDelete. |

## Structure

**Backend**

- `internal/storage/entity/interfaces.go` — Add `UpdateAgent` and `DeleteAgent` to `AgentStore`.
- `internal/storage/entity/agent.go` — Implement `UpdateAgent` and `DeleteAgent`.
- `internal/server/agents.go` — Add `patchAgentRequest`, `patchAgentHandler`, `deleteAgentHandler`; reuse `agentToResponse`.
- `internal/server/server.go` — Register `PATCH` and `DELETE` for `/api/workspaces/{workspace_id}/agents/{agent_id}`.
- `internal/server/static/openapi.json` — Add `patch` and `delete` under `/api/workspaces/{workspace_id}/agents/{agent_id}`.
- `internal/server/agents_test.go` (new) or extend existing test file — Handler tests for PATCH and DELETE.

**Portal**

- `portal/src/lib/api/index.ts` — Add `updateAgent` and `deleteAgent`.
- `portal/src/components/CreateEntityModal.tsx` — Optional `initialValues`; when `open` and `initialValues` provided, seed form.
- `portal/src/components/EditAgentModal.tsx` (new) — Uses CreateEntityModal with agent fields, initialValues from agent, submitLabel "Save", onSubmit → onSave; add "Delete" button that calls parent `onDelete` (optionally with confirmation).
- `portal/src/pages/AgentList.tsx` — State `editingAgent: Agent | null`; card click opens edit modal; EditAgentModal onSave → updateAgent + update list; onDelete → deleteAgent + remove from list + close modal.

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|----------------|
| **AgentStore** (interface) | UpdateAgent | `(ctx, agentID, workspaceID, name, description, instructions string) (*Agent, error)` | Update agent by ID; caller ensures workspace ownership. Return updated agent or error (e.g. not found). |
| **AgentStore** (interface) | DeleteAgent | `(ctx, agentID, workspaceID string) error` | Delete agent by ID only if it exists and workspace_id matches; return nil or error (e.g. not found). |
| **Store** | UpdateAgent | Same as interface | Load by agent_id; verify workspace_id; update name, description, instructions; return updated row. |
| **Store** | DeleteAgent | Same as interface | Load by agent_id; verify workspace_id; delete row; return nil or error. |
| **Server** | patchAgentHandler | `(w http.ResponseWriter, r *http.Request)` | Auth + workspace; parse agent_id; decode body; validate name non-empty if provided; call AgentStore.UpdateAgent; return 200 + agent or 400/404. |
| **Server** | deleteAgentHandler | `(w http.ResponseWriter, r *http.Request)` | Auth + workspace; parse agent_id; call AgentStore.DeleteAgent; return 204 or 404. |
| **api** (Portal) | updateAgent | `(workspaceId, agentId, body, token) => Promise<ApiAgent>` | PATCH JSON to `/api/workspaces/{id}/agents/{id}`; return parsed ApiAgent. |
| **api** (Portal) | deleteAgent | `(workspaceId, agentId, token) => Promise<void>` | DELETE `/api/workspaces/{id}/agents/{id}`; throw on non-2xx. |

**CreateEntityModal (change)**

- Props: add optional `initialValues?: Record<string, string>`.
- Behavior: when `open` becomes true, if `initialValues` is provided, set internal `values` from it for each field key (use `initialValues[key] ?? ""`); otherwise reset to empty strings as today.

**EditAgentModal (new)**

- Props: `open`, `agent: Agent | null`, `loading`, `error`, `onClose`, `onSave: (values) => void`, `onDelete: () => void`, `deleting?: boolean`.
- When `open` and `agent` are set, render CreateEntityModal with title "Edit agent", titleId "edit-agent-title", same AGENT_FIELDS, initialValues from agent, submitLabel "Save", onSubmit → onSave.
- Add a "Delete" button (e.g. secondary/danger, below or beside Cancel/Save). On click: confirm (e.g. `window.confirm` or a small confirm state) then call `onDelete`. Disable Save/Delete while `loading` or `deleting`.

**AgentList**

- Add state: `editingAgent: Agent | null`.
- Add handler: `handleEditAgent(agent)` sets `editingAgent(agent)` (and optionally a small “edit modal open” flag if we use one modal for both create and edit; here we have separate Create vs Edit modal so “edit open” is implied by `editingAgent !== null`).
- On agent card: add `onClick={() => setEditingAgent(a)}` (and optionally `role="button"` + keyboard for a11y).
- Render EditAgentModal when `editingAgent !== null`; onClose set `editingAgent(null)`; onSave call `updateAgent(workspaceId, editingAgent.id, body, token)`, then replace that agent in `agents` state (or refetch), then set `editingAgent(null)`.

## How they work together

**Edit flow**

1. User clicks an agent card on AgentList → `editingAgent` set to that agent, EditAgentModal opens with initialValues from agent.
2. User edits name/description/instructions and clicks Save → EditAgentModal onSubmit → parent onSave(values) → AgentList calls `updateAgent(workspaceId, editingAgent.id, { name, description, instructions }, token)`.
3. API client sends PATCH to server; server patchAgentHandler uses withWorkspaceAuth, reads agent_id, decodes body, calls AgentStore.UpdateAgent, returns agent response.
4. Store UpdateAgent finds agent by ID, checks workspace_id, updates columns, returns updated Agent.
5. Portal on success: update local `agents` (replace item by id) and set `editingAgent(null)`; modal closes and list shows new data.

**Delete flow**

1. User has EditAgentModal open for an agent and clicks "Delete" → confirm (e.g. "Delete this agent?") → parent onDelete().
2. AgentList calls `deleteAgent(workspaceId, editingAgent.id, token)`.
3. API client sends DELETE to server; server deleteAgentHandler uses withWorkspaceAuth, reads agent_id, calls AgentStore.DeleteAgent, returns 204.
4. Store DeleteAgent finds agent by ID, checks workspace_id, deletes row.
5. Portal on success: remove agent from `agents` state and set `editingAgent(null)`; modal closes and list no longer shows that agent.

**Dependencies**

- Server depends on entity (AgentStore) for persistence.
- Portal pages depend on api and components; api depends on no other Portal code.
- EditAgentModal depends on CreateEntityModal and Agent type; AgentList owns edit state and updateAgent call.

**Key data structures**

- **patchAgentRequest** (server): JSON body with `name`, `description`, `instructions` (all optional; name required to be non-empty when provided, or require name always for PATCH for simplicity — task says “require name if present (non-empty)” so we accept partial body but if name is present it must be non-empty).
- **AgentResponse**: Unchanged; PATCH returns same shape as GET.

## OpenAPI (PATCH and DELETE)

Under `"/api/workspaces/{workspace_id}/agents/{agent_id}"` add:

- **patch**: summary "Update agent"; description "Updates a workspace-scoped agent. Caller must own the workspace. Requires Bearer JWT."
  - Path parameters: workspace_id, agent_id (required).
  - Request body: application/json with schema object properties name (string), description (string), instructions (string); no required array (all optional; backend validates name non-empty if provided).
  - Responses: 200 (OK, same agent schema as GET), 400 (agent_id required / invalid body / name empty), 401, 403, 404 (agent not found), 503 (agents not configured).

- **delete**: summary "Delete agent"; description "Deletes a workspace-scoped agent. Caller must own the workspace. Requires Bearer JWT."
  - Path parameters: workspace_id, agent_id (required).
  - Responses: 204 (No Content), 400 (agent_id required), 401, 403, 404 (agent not found), 503 (agents not configured).

## Tests (backend)

- **PATCH success**: With valid workspace and agent_id and body { name, description?, instructions? }, expect 200 and response body matches updated agent.
- **PATCH 404**: Wrong workspace_id or non-existent agent_id → 404.
- **PATCH 400**: Empty name in body (e.g. `{"name":""}`) → 400.
- **DELETE success**: With valid workspace and agent_id, expect 204; agent no longer returned by list/get.
- **DELETE 404**: Wrong workspace_id or non-existent agent_id → 404.

Use a test store (in-memory or DB) and existing auth helpers if present; same pattern as other handler tests in the server package.

## Changes for review

- **New**: `internal/storage/entity` — `AgentStore.UpdateAgent` and `AgentStore.DeleteAgent` in interface; `Store.UpdateAgent` and `Store.DeleteAgent` in agent.go.
- **New**: `internal/server/agents.go` — `patchAgentRequest`, `patchAgentHandler`, `deleteAgentHandler`; routes in server.go for PATCH and DELETE.
- **Modified**: `internal/server/static/openapi.json` — Add `patch` and `delete` for `/api/workspaces/{workspace_id}/agents/{agent_id}`.
- **New**: `internal/server/agents_test.go` (or similar) — Tests for PATCH (success, 400, 404) and DELETE (success, 404).
- **New**: `portal/src/lib/api/index.ts` — `updateAgent(workspaceId, agentId, body, token)` and `deleteAgent(workspaceId, agentId, token)`.
- **Modified**: `portal/src/components/CreateEntityModal.tsx` — Optional prop `initialValues`; when open and initialValues set, initialize form state from it.
- **New**: `portal/src/components/EditAgentModal.tsx` — Edit modal using CreateEntityModal with agent initialValues, onSave, and "Delete" button with onDelete (with confirmation).
- **Modified**: `portal/src/pages/AgentList.tsx` — `editingAgent` state; card onClick opens edit; EditAgentModal onSave → updateAgent + update list; onDelete → deleteAgent + remove from list + close modal.
