# Portal URL scheme

The portal uses **hash-based routing**: the path after `#` defines the current view. This works without server configuration (the server always serves the same `index.html`). The signed-in user is resolved from auth, so the hash only describes the current view or resource.

## Rules

- **Base**: The hash is optional. Empty or invalid hash is treated as home.
- **First segment** (optional): One of `conversations`, `conversation`, `explore`, `agents`. If absent, the route is **home**.
- **IDs**: Entity IDs in the URL (for example `conversationId`) are opaque strings; the app does not interpret their format.

## Route patterns

| Route          | Pattern                              | Example (IDs are placeholders)   |
|----------------|--------------------------------------|----------------------------------|
| Home           | `#/`                                 | `#/`                             |
| Conversations  | `#/conversations`                    | `#/conversations`                |
| Conversation   | `#/conversation/<conversationId>`    | `#/conversation/c_xyz`           |
| Explore (Files)| `#/explore`                          | `#/explore`                      |
| Agents         | `#/agents`                           | `#/agents`                       |

## Examples

- `https://example.com/` or `https://example.com/#` — Home.
- `https://example.com/#/conversations` — Conversations list.
- `https://example.com/#/conversation/c_xyz` — Conversation `c_xyz`.
- `https://example.com/#/explore` — File explore.
- `https://example.com/#/agents` — Agents.

## Implementation

- **Parse**: `parseHash(window.location.hash)` in `src/router.ts` turns the hash into a typed `Route`.
- **Build**: `buildHash(route)` produces the canonical hash string.
- **Navigate**: `navigate(route)` sets `window.location.hash` to the result of `buildHash(route)`.
- Path segment names (`new`, `conversations`, `conversation`, `explore`, `agents`) are defined as `SEGMENT` in `src/router.ts` and used in both `parseHash` and `buildHash`.
