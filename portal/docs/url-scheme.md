# Portal URL scheme

The portal uses **hash-based routing**: the path after `#` defines the current view. This works without server configuration (the server always serves the same `index.html`).

## Rules

- **Base**: The hash is optional. Empty or invalid hash is treated as workspace home with an empty workspace ID (the app then redirects to the first available workspace if needed).
- **First segment**: Always the **workspace ID** (opaque string, e.g. short base62 IDs).
- **Second segment** (optional): One of `project`, `task`, `artifact`, `activity`, `explore`. If absent, the route is **workspace home**.
- **IDs**: All entity IDs in the URL (workspace, project, task, artifact) are opaque strings; the app does not interpret their format.

## Route patterns

| Route       | Pattern                                      | Example (IDs are placeholders)        |
|------------|-----------------------------------------------|----------------------------------------|
| Workspace  | `#<workspaceId>`                             | `#abc123`                              |
| Project    | `#<workspaceId>/project/<projectId>`         | `#abc123/project/def456`               |
| Task       | `#<workspaceId>/task/<projectId>/<taskId>`    | `#abc123/task/def456/ghi789`           |
| Artifact   | `#<workspaceId>/artifact/<projectId>/<artifactId>` | `#abc123/artifact/def456/ghi789`  |
| Activity   | `#<workspaceId>/activity`                    | `#abc123/activity`                     |
| Explore    | `#<workspaceId>/explore`                     | `#abc123/explore`                      |

## Examples

- `https://example.com/` or `https://example.com/#` — No workspace; app redirects to default workspace.
- `https://example.com/#W1` — Workspace home for workspace `W1`.
- `https://example.com/#W1/project/P1` — Project `P1` in workspace `W1`.
- `https://example.com/#W1/task/P1/T1` — Task `T1` in project `P1`, workspace `W1`.
- `https://example.com/#W1/artifact/P1/A1` — Artifact `A1` in project `P1`, workspace `W1`.
- `https://example.com/#W1/activity` — Activity feed for workspace `W1`.
- `https://example.com/#W1/explore` — File explore for workspace `W1`.

## Implementation

- **Parse**: `parseHash(window.location.hash)` in `src/lib/router.ts` turns the hash into a typed `Route`.
- **Build**: `buildHash(route)` produces the canonical hash string.
- **Navigate**: `navigate(route)` sets `window.location.hash` to the result of `buildHash(route)`.
- Path segment names (`project`, `task`, `artifact`, `activity`, `explore`) are defined as `SEGMENT` in `src/lib/router.ts` and used in both `parseHash` and `buildHash`.
