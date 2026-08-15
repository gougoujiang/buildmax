# @buildmax/gui

Shared React UI components and styles for BuildMax portal and desktop app. Implement once, use in both.

## Exports

- **Theme**: `ThemeProvider`, `useTheme`, `ThemeToggle`, and type `Theme` (`"light" | "dark"`).
- **Styles**: `theme.css` — CSS variables for light/dark (`data-theme`). Import as `@buildmax/gui/theme.css`.
- **BaseModal**: Presentational modal component; props: `open`, `title`, `titleId`, `onClose`, optional `className`, optional `hideHeader`, `children`. Type `BaseModalProps` is exported for TypeScript.
- **Modal styles**: `modal.css` — base modal layout (overlay, `.modal`, `.modal--large`, `.modal__header`, `.modal__title`, `.modal__close`, `.modal__body`). Uses theme variables; import `theme.css` first, then `@buildmax/gui/modal.css`.

## Local dependency (portal & desktop)

From `portal/` or `desktop/frontend/` add to `package.json`:

```json
"dependencies": {
  "@buildmax/gui": "file:../gui"
}
```

From `desktop/frontend/` the path is `file:../../gui`.

1. Use Node 22 and npm 10 (see the root `.node-version`).
2. Build the package: `cd gui && npm ci && npm run build`
3. In the app: `npm ci`
4. Import: `import { ThemeProvider, useTheme, ThemeToggle, BaseModal } from '@buildmax/gui'`, `import '@buildmax/gui/theme.css'`, and (if using modals) `import '@buildmax/gui/modal.css'`

Consumers keep their own layout/sidebar and any `.theme-toggle` button styles; the package provides the component and theme variables.

## Consolidation candidates

Future components to consider moving into this package (presentational only; each app keeps its own data and callbacks):

- **PromptArea / composer** — Single-line or multi-line input + primary action; used in portal and reusable in desktop chat.
- **Chat message bubble** — Presentational message block (user/assistant); shared rendering for both apps.
- **Primary / secondary button** — Shared button styles and variants to keep actions consistent.
- **Icons** — Shared icon set or sprite so both apps use the same symbols.
