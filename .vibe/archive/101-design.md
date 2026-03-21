# Design 101: Common GUI package for portal and desktop

## Goal

Introduce a shared GUI package at workspace root and align both portal and desktop on React 19, so they can import common theme components and CSS from one place without duplication.

## Shared component strategy

- **Portal and desktop have different inner logic** (data, auth, routing, backends). The gui package does **not** unify app logic; it provides **reusable presentational widgets** so we implement once and use in both.
- **What belongs in gui:** Presentational components that accept data and callbacks via props — theme, settings UI, chat history rendering, input/prompt box, and similar widgets. Each app keeps its own state and API integration; gui stays UI-only.
- **This design:** First slice is theme only. Later slices can add settings page (or pieces), chat message list, prompt input, etc., as needed.

## Modules


| Module                | Responsibility                                       | Owns                                                                             |
| --------------------- | ---------------------------------------------------- | -------------------------------------------------------------------------------- |
| **gui/**              | Shared React components and styles for BuildMax UIs. | ThemeProvider, useTheme, ThemeToggle, theme.css; package.json, Vite lib build.   |
| **portal/**           | Web app consuming the shared package.                | Remove local theme context/toggle/CSS; depend on `@buildmax/gui`; keep React 19. |
| **desktop/frontend/** | Desktop app consuming the shared package.            | Upgrade to React 19; remove local theme files; depend on `@buildmax/gui`.        |


## Structure

**Directory and files**

- `gui/` — shared package at repo root (sibling to `portal/`, `desktop/`)
  - `package.json` — name `@buildmax/gui`, exports (main, module, types), peerDependencies react/react-dom ^19.0.0, build script
  - `vite.config.ts` — Vite in library mode: build to `dist/`, emit ESM + types, copy or emit `theme.css`
  - `src/ThemeContext.tsx` — ThemeProvider, useTheme, Theme type (same behavior as current portal implementation)
  - `src/ThemeToggle.tsx` — ThemeToggle component (uses useTheme)
  - `src/index.ts` — re-export ThemeProvider, useTheme, ThemeToggle, Theme
  - `src/theme.css` — CSS variables for light/dark (`:root` / `[data-theme="light"]` / `[data-theme="dark"]`), same as current portal/desktop
  - `README.md` — short description, list of exports, how to add local dependency
  - `tsconfig.json` — for type-check and Vite (if needed)
- Build output: `gui/dist/` — ESM bundle(s), type declarations, and `theme.css` (or path to it in package exports)

**Main types and exports**

- **Theme** (gui): type `"light" | "dark"`.
- **ThemeProvider** (gui): React component; wraps app, provides theme state and setters via context; syncs `data-theme` and localStorage (`buildmax_theme`).
- **useTheme** (gui): hook returning `{ theme, setTheme, toggleTheme }`; throws if used outside ThemeProvider.
- **ThemeToggle** (gui): button that toggles theme; uses `useTheme`, renders sun/moon icons, class `theme-toggle` for consumer CSS if needed.
- **theme.css** (gui): single file; consumers import `@buildmax/gui/theme.css` or equivalent (see package exports).

## Package design

**package.json (gui)**

- `"name": "@buildmax/gui"`
- `"version": "0.0.1"` (or align with project)
- `"type": "module"`
- `"main"`, `"module"`, `"types"`: point to dist entry (e.g. `dist/index.js`, `dist/index.d.ts`). Export `theme.css` via `"exports"` (e.g. `"./theme.css": "./dist/theme.css"`).
- `"peerDependencies"`: `"react": "^19.0.0"`, `"react-dom": "^19.0.0"`
- `"scripts"`: `"build": "vite build"` (and optionally `"dev"` for development)
- `"files"`: include `dist` (and optionally `src` for source maps)

**Vite library build (gui)**

- Use Vite’s library mode: single entry `src/index.ts`, output ESM, generate `.d.ts` (e.g. via vite-plugin-dts or rollup-plugin-dts). Copy `src/theme.css` to `dist/theme.css` (e.g. via plugin or build script) and expose it in package exports so consumers can `import '@buildmax/gui/theme.css'`.

**Consumer dependency**

- Portal and desktop add: `"@buildmax/gui": "file:../gui"` (relative path from `portal/` or `desktop/frontend/` to `gui/`). Run `npm install` after adding; build the gui package first (`cd gui && npm run build`) so dist exists.

## How they work together

**Data/control flow**

1. Developer runs `cd gui && npm install && npm run build` to produce `gui/dist/`.
2. Portal and desktop list `@buildmax/gui` as a local `file:../gui` dependency and run `npm install`.
3. Portal: in entry or layout, `import { ThemeProvider } from '@buildmax/gui'` and `import '@buildmax/gui/theme.css'`; wrap app with `<ThemeProvider>`; use `<ThemeToggle />` and `useTheme()` from `@buildmax/gui`. Remove `portal/src/contexts/ThemeContext.tsx`, `portal/src/components/ThemeToggle.tsx`, and the theme part of `portal/src/css/theme.css` (or replace `index.css` theme import with package import).
4. Desktop: upgrade React to 19 in `desktop/frontend/package.json`; same import and wrap pattern; remove `ThemeContext.jsx`, `ThemeToggle.jsx`, and local `css/theme.css` theme content (or import package theme CSS in `index.css`).
5. Both apps build and run as before; theme behavior and CSS variables stay the same.

**Dependencies**

- `gui` has no dependency on portal or desktop; it only depends on React (peer).
- Portal and desktop depend on `gui` for theme UI and theme CSS.
- Portal remains React 19; desktop is upgraded to React 19 so a single React version is used across the shared package and both apps.

**Key data structures**

- Theme state: `theme: "light" | "dark"` in context; persisted in `localStorage` under `buildmax_theme`; applied to `document.documentElement` as `data-theme`.
- CSS variables: defined in `gui/src/theme.css`; consumers get them by importing the package’s theme.css.

## Changes for review

- **New**: `gui/` — root-level package: package.json, vite.config.ts, src/ThemeContext.tsx, src/ThemeToggle.tsx, src/index.ts, src/theme.css, README.md, tsconfig.json.
- **New**: `gui` build output — dist/ with ESM bundle, type declarations, and theme.css.
- **Modified**: `portal/package.json` — add `"@buildmax/gui": "file:../gui"`; remove no files (theme files removed from repo).
- **Modified**: `portal/src/App.tsx` — import ThemeProvider from `@buildmax/gui`.
- **Modified**: `portal/src/layout/Layout.tsx` (or wherever ThemeToggle is used) — import ThemeToggle from `@buildmax/gui`.
- **Modified**: `portal/src/index.css` — import theme from `@buildmax/gui/theme.css` (or keep a thin wrapper that only imports that).
- **Deleted**: `portal/src/contexts/ThemeContext.tsx`, `portal/src/components/ThemeToggle.tsx`; remove or replace theme section of `portal/src/css/theme.css` if it becomes redundant.
- **Modified**: `desktop/frontend/package.json` — set `react` and `react-dom` to ^19.0.0; add `"@buildmax/gui": "file:../../gui"` (path from desktop/frontend to repo root gui).
- **Modified**: `desktop/frontend/src/App.jsx` — import ThemeProvider, ThemeToggle from `@buildmax/gui`; import `@buildmax/gui/theme.css` in index.css or here.
- **Modified**: `desktop/frontend/src/index.css` — import theme from package instead of local css/theme.css.
- **Deleted**: `desktop/frontend/src/ThemeContext.jsx`, `desktop/frontend/src/ThemeToggle.jsx`, `desktop/frontend/src/css/theme.css`.

