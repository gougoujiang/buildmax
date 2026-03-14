# @buildmax/gui

Shared React UI components and styles for BuildMax portal and desktop app. Implement once, use in both.

## Exports

- **Theme**: `ThemeProvider`, `useTheme`, `ThemeToggle`, and type `Theme` (`"light" | "dark"`).
- **Styles**: `theme.css` — CSS variables for light/dark (`data-theme`). Import as `@buildmax/gui/theme.css`.

## Local dependency (portal & desktop)

From `portal/` or `desktop/frontend/` add to `package.json`:

```json
"dependencies": {
  "@buildmax/gui": "file:../gui"
}
```

From `desktop/frontend/` the path is `file:../../gui`.

1. Build the package: `cd gui && npm install && npm run build`
2. In the app: `npm install`
3. Import: `import { ThemeProvider, useTheme, ThemeToggle } from '@buildmax/gui'` and `import '@buildmax/gui/theme.css'`

Consumers keep their own layout/sidebar and any `.theme-toggle` button styles; the package provides the component and theme variables.
