# Design 031 — Portal landing page

## Goal

Implement the wireframe landing page from `design/001-about-portal.md` (§7) in the existing Portal app: one screen with header (app name, workspace label, Profile), prompt area (“What would you like to accomplish?” + input + Run), and Recent Activity with mock items. All data is static; no backend or API.

## Modules

| Module | Responsibility | Owns |
|--------|----------------|------|
| **portal/src** | Landing UI and layout | App.tsx, optional components, index.css |
| **portal/** (root) | Build and entry | index.html, main.tsx, package.json — unchanged except index.html title if desired |

## Structure

**Directory / files**

- `portal/src/`
  - `App.tsx` — Root component: composes header, prompt area, and recent activity; holds prompt input state and mock activity data.
  - `index.css` — Global and layout styles; extended with classes for landing sections.
- Optional (implementer may keep everything in App or extract):
  - `portal/src/Header.tsx` — Header: app name, workspace label, Profile entry (presentational; props or none).
  - `portal/src/PromptArea.tsx` — Heading, controlled input, Run button (props: value, onChange, onRun).
  - `portal/src/RecentActivity.tsx` — Section title and list of items (props: items array).

**Main types (TypeScript)**

- **ActivityItem**: `{ title: string; time: string }` — One recent-activity row (title + timestamp/relative time). Defined in App or a small `types.ts` / inline.
- **Mock data**: A constant array of 3–5 `ActivityItem` values (e.g. “Generated sales report (Today 10:42 AM)”) in App or a `mockActivity.ts`; no API.

**Component responsibilities**

- **App**: Renders full landing layout; owns `prompt` state (string) and passes it to the prompt area; owns or imports mock activity list; Run handler is no-op (e.g. `console.log(prompt)` or empty).
- **Header** (if extracted): Renders “BuildMax Portal” (or “Portal”), “Workspace: Default” (or “Workspace: Sales Team”), and “Profile” or ⚙️; no navigation or handlers.
- **PromptArea** (if extracted): Renders “What would you like to accomplish?”, `<input>` controlled by parent, and `<button>Run</button>`; `onRun` called on button click.
- **RecentActivity** (if extracted): Renders “Recent Activity” heading and a list of items; each item shows `title` and `time`.

## Method design

No Go code. React-only contracts:

| Component | Props / state | Responsibility |
|-----------|----------------|----------------|
| **App** | State: `prompt: string`. Optional state for Run feedback (not required). | Compose layout; provide mock activity array; pass prompt value/onChange and onRun (no-op) to PromptArea. |
| **Header** | None or `workspaceName?: string` | Render header row: branding, workspace label, Profile. |
| **PromptArea** | `value: string`, `onChange: (v: string) => void`, `onRun: () => void` | Render heading, input, Run button; call onRun when Run is clicked. |
| **RecentActivity** | `items: { title: string; time: string }[]` | Render section title and list; each item: title + time. |

## How they work together

**Render flow**

1. `main.tsx` renders `App` (unchanged).
2. `App` renders in order: Header (or inline header), PromptArea (with prompt state and no-op onRun), RecentActivity (with mock items).
3. User types in the input → `App` updates `prompt` state → input stays controlled. User clicks Run → onRun runs (no-op); no API call.
4. Recent Activity is purely presentational; data is a constant array defined in App or a small mock module.

**Data**

- **Mock activity**: Defined once (e.g. in App or `portal/src/mockActivity.ts`) as a constant array of 3–5 objects `{ title, time }`. Example: `{ title: "Generated sales report", time: "Today 10:42 AM" }`.
- **Prompt**: Single React state string in App; no persistence.

**Styling**

- Reuse `:root`, `body`, `#root`, `.app` from existing `index.css`. Extend with:
  - Layout: `.app` remains main container (max-width, padding); add section classes (e.g. `.landing-header`, `.prompt-area`, `.recent-activity`) for spacing and visual separation.
  - Header: flex or grid so app name is left, workspace center or left, Profile right.
  - Prompt area: heading, input (full width or constrained), button below or beside.
  - Recent Activity: list with bullet or icon; each row shows title and time.
- No new dependencies (no CSS-in-JS, no Tailwind for this task).

## Changes for review

| Change | Description |
|--------|-------------|
| **Modified** `portal/src/App.tsx` | Replace current content with landing layout: header, prompt area (with local state and no-op Run), recent activity list using mock data. Optionally extract Header, PromptArea, RecentActivity into separate components under `portal/src/`. |
| **Modified** `portal/src/index.css` | Add classes for landing sections (header, prompt area, recent activity), list styles, and spacing so layout matches wireframe. |
| **New** (optional) `portal/src/Header.tsx` | Presentational header: app name, workspace label, Profile. |
| **New** (optional) `portal/src/PromptArea.tsx` | Prompt heading, controlled input, Run button; receives value, onChange, onRun from App. |
| **New** (optional) `portal/src/RecentActivity.tsx` | “Recent Activity” section and list; receives items array. |
| **New** (optional) `portal/src/mockActivity.ts` | Export constant array of `{ title, time }` for recent activity (or inline in App). |
| **Unchanged** | `portal/main.tsx`, `portal/package.json`, `portal/index.html` (unless adding a one-line doc note), Go code, `portal/README.md` (unless brief note that landing is wireframe). |
