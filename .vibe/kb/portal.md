# Portal

## Purpose

The **Portal** is the web UI for BuildMax, living under `portal/`. It is a separate React + Vite + TypeScript application that builds and runs independently of the Go binary. It provides a "BuildMax Portal" experience: login, workspaces, projects, and tasks. The Go backend is the `buildmax server` API; the portal calls it via `src/lib/api.ts`.

## Layout

- **portal/** — Vite project; `npm run dev` (default http://localhost:5173), `npm run build` → `dist/`
- **portal/src/** — `App.tsx`, pages (Login, WorkspaceHome, ProjectDashboard, TaskDetail, etc.), components (TopBar, AppShell, Breadcrumbs, PromptArea), contexts (AuthContext), and `lib/api.ts` for API calls
- **portal/src/data/** — Mock data and re-exports; swap for real API via a single data layer

## How It Works

- Auth: Login page submits email to `POST /api/login`; JWT is stored and sent as `Authorization: Bearer <token>` for protected routes.
- Workspaces, projects, tasks: Fetched and created via the API (see [Server](server.md)); when the backend is not used, the app can use mock data from `src/data/`.
- No agent or LLM logic runs in the portal; it is a frontend for the workspace/project/task model and future agent-mediated flows (see design/001-about-portal.md).

## Dependencies

- **Stack**: React, TypeScript, Vite. Build output is static; no Node runtime required for production.
- **Backend**: Optional; `buildmax server` with store and JWT configured. CORS is set (e.g. `http://localhost:5173`) for dev.

## Notes

- See [Server](server.md) for API endpoints and [Store](store.md) for backend persistence. Product vision: [design/001-about-portal.md](../../design/001-about-portal.md).
