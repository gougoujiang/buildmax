# Portal URL scheme

The portal uses **hash-based routing**: the path after `#` defines the current view. This works without server configuration (the server always serves the same `index.html`). **The user is the top-level owner**; the first segment is the user id (`profileId`).

## Rules

- **Base**: The hash is optional. Empty or invalid hash is treated as home with no profile id; the app then redirects to the signed-in user's id.
- **First segment**: The **user id** (`profileId`), used as the scope for all data (conversations, files, agents).
- **Second segment** (optional): One of `new`, `conversations`, `conversation`, `explore`, `agents`. If absent, the route is **home**.
- **IDs**: Entity IDs in the URL (for example `conversationId`) are opaque strings; the app does not interpret their format.

## Route patterns

| Route          | Pattern                              | Example (IDs are placeholders)   |
|----------------|--------------------------------------|----------------------------------|
| Home           | `#<userId>`                          | `#u_abc123`                      |
| New Conversation | `#<userId>/new`                    | `#u_abc123/new`                  |
| Conversations  | `#<userId>/conversations`            | `#u_abc123/conversations`        |
| Conversation   | `#<userId>/conversation/<conversationId>` | `#u_abc123/conversation/c_xyz` |
| Explore (Files)| `#<userId>/explore`                  | `#u_abc123/explore`              |
| Agents         | `#<userId>/agents`                   | `#u_abc123/agents`               |

## Examples

- `https://example.com/` or `https://example.com/#` — No user id; app redirects to current user's home.
- `https://example.com/#u_abc` — Home for user `u_abc`.
- `https://example.com/#u_abc/new` — New conversation for user `u_abc`.
- `https://example.com/#u_abc/conversations` — Conversations list for user `u_abc`.
- `https://example.com/#u_abc/conversation/c_xyz` — Conversation `c_xyz` for user `u_abc`.
- `https://example.com/#u_abc/explore` — File explore for user `u_abc`.
- `https://example.com/#u_abc/agents` — Agents for user `u_abc`.

## Implementation

- **Parse**: `parseHash(window.location.hash)` in `src/router.ts` turns the hash into a typed `Route`.
- **Build**: `buildHash(route)` produces the canonical hash string.
- **Navigate**: `navigate(route)` sets `window.location.hash` to the result of `buildHash(route)`.
- Path segment names (`new`, `conversations`, `conversation`, `explore`, `agents`) are defined as `SEGMENT` in `src/router.ts` and used in both `parseHash` and `buildHash`.
