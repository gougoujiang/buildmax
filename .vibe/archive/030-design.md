# Task 030 — Design: Add a portal

## Goal

Add a `portal/` directory with a minimal React (Vite + TypeScript) app that builds and runs and shows a simple "BuildMax Portal" landing. No backend or agent features in this task.

## Modules

| Module | Purpose |
|--------|---------|
| `portal/` | Root of the frontend app; holds package.json, Vite config, and React source. |
| `portal/src/` | Entry and app component(s). |
| `portal/public/` | Static assets (if any; optional for minimal setup). |

## Structure

```
portal/
├── package.json           # Scripts: dev, build, preview; deps: react, react-dom, vite, types
├── tsconfig.json          # TypeScript for app and Vite
├── vite.config.ts         # Vite config (root: portal/, build out: dist/)
├── index.html             # Vite entry HTML (root)
├── README.md              # Install, build, dev instructions
└── src/
    ├── main.tsx           # React root (createRoot, App)
    ├── App.tsx            # Single component: heading "BuildMax Portal"
    └── index.css          # Minimal global styles (optional)
```

No routing, no state library, no API client in this task.

## Method design

- **N/A** — no Go code and no new exported APIs. Implementation is purely new files under `portal/`.

## How they work together

1. Developer runs `cd portal && npm install && npm run dev`; Vite serves the app; browser shows "BuildMax Portal".
2. `npm run build` produces `portal/dist/` for future use (e.g. static hosting or later embedding).

## Changes for review

| Change | Description |
|--------|-------------|
| **New** `portal/` | Directory and all files above (package.json, Vite + TS configs, index.html, src/main.tsx, App.tsx, index.css, README.md). |
| **Unchanged** | `cmd/`, `internal/`, `go.mod`, `go.sum`, root README, make.bat. |
