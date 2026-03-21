# Design 102: Gui common components (BaseModal + consolidation candidates)

## Goal

Add the base modal component and its minimal CSS to `@buildmax/gui`, document consolidation candidates for future work, and switch portal to use the shared BaseModal so both apps can reuse it and the pattern is established.

## Modules

| Module   | Responsibility | Changes |
|----------|----------------|---------|
| **gui/** | Shared React components and styles. | Add BaseModal, modal.css; extend package exports and build; add README section for BaseModal and consolidation candidates. |
| **portal/** | Web app consuming gui. | Import BaseModal and modal base CSS from gui; remove local BaseModal.tsx; trim modal.css to portal-specific rules only. |

## Structure

### gui package

**New/updated files**

- `gui/src/BaseModal.tsx` — Presentational modal: same API and behavior as current portal `BaseModal`. No dependency on portal or desktop.
- `gui/src/modal.css` — Base modal styles only (see "Modal CSS content" below). Uses theme variables from `theme.css` (e.g. `var(--color-bg-elevated)`, `var(--shadow-modal)`); consumers must have already imported `theme.css`.
- `gui/src/index.ts` — Add export for `BaseModal` and (if desired) a `BaseModalProps` type.
- `gui/package.json` — Add export entry for `"./modal.css": "./dist/modal.css"`.
- `gui/README.md` — Document BaseModal and modal CSS; add "Consolidation candidates" section.

**Build**

- Extend the existing build so `modal.css` is copied to `dist/modal.css` (same pattern as `theme.css`: e.g. `cp src/modal.css dist/modal.css` in the build script).

### Modal CSS content (gui)

Include in `gui/src/modal.css` only the rules required by the BaseModal component and the common `.modal__body` / `.modal--large` used by portal modals:

- `.modal-overlay` — fixed overlay, flex center, background, z-index, `modal-fade-in` animation.
- `@keyframes modal-fade-in`, `@keyframes modal-slide-up`.
- `.modal` — bg, border-radius, box-shadow, max-width, margin, overflow, `modal-slide-up` animation (all via theme vars).
- `.modal--large` — max-width 600px, min-height 320px; and `.modal--large .modal__body` padding/gap override.
- `.modal__header`, `.modal__title`, `.modal__close`, `.modal__close:hover`.
- `.modal__body` — padding, flex column, gap.

Do **not** include in gui: `.modal--settings`, any `.settings-modal__*`, `.modal__label`, `.modal__input`, `.modal__textarea`, `.modal__btn*`, `.modal__actions`, `.modal__hint`, `.modal__error`, etc. Those remain in portal.

### Portal

- **Remove** `portal/src/components/BaseModal.tsx`.
- **Replace** `portal/src/css/modal.css` with a file that:
  1. Imports the base: `@import '@buildmax/gui/modal.css';` (or equivalent so base styles load first).
  2. Contains only portal-specific rules: `.modal--settings`, `.settings-modal__*`, `.modal__label`, `.modal__input`, `.modal__textarea`, `.modal__optional`, `.modal__hint`, `.modal__error`, `.modal__actions`, `.modal__btn`, `.modal__btn--primary`, `.modal__btn--secondary`, and any other form/panel rules that are not part of the minimal base.
- **Update** every file that imported `BaseModal` from `./BaseModal` or `./components/BaseModal` to import from `@buildmax/gui` instead. No change to how they use the component (same props).
- **Ensure** portal still imports theme CSS before modal (e.g. in `index.css`: theme, then modal). If portal's `index.css` currently does `@import './css/modal.css'`, keep that; the updated `modal.css` will pull in gui base via `@import` and then add portal overrides.

## Method design

### BaseModal (gui)

- **Component**: `BaseModal(props: BaseModalProps): JSX.Element`
- **Props** (same as current portal):
  - `open: boolean` — whether the modal is visible.
  - `title: string` — used for heading and aria-label when header hidden.
  - `titleId: string` — id for the title element (accessibility).
  - `onClose: () => void` — called on overlay click or Escape.
  - `className?: string` — optional class(es) applied to the inner `.modal` div (e.g. `modal--large`).
  - `hideHeader?: boolean` — when true, do not render header; caller can render close/heading elsewhere.
  - `children: ReactNode` — modal content (typically a div with class `modal__body` or custom layout).
- **Behavior**: When `open` becomes true, focus first input/textarea in the modal after a tick. On Escape, call `onClose`. Overlay click calls `onClose`. Render `role="dialog"`, `aria-modal="true"`, `aria-labelledby={titleId}` (or `aria-label={title}` when header hidden). When `open` is false, return `null`.
- **Exports**: Export `BaseModal` from `gui/src/index.ts`. Optionally export `BaseModalProps` type for TypeScript consumers.

## How they work together

1. **Build**: `cd gui && npm run build` produces `dist/index.js`, `dist/index.d.ts`, `dist/theme.css`, and `dist/modal.css`.
2. **Portal**: Depends on `@buildmax/gui`. Imports `BaseModal` from `@buildmax/gui`. Portal's `modal.css` imports `@buildmax/gui/modal.css` and then defines only portal-specific modal and form styles. All existing modals (SettingsModal, NewChatFromAgentModal, ArtifactContentModal, CreateEntityModal) use `<BaseModal ...>` from gui with the same props; they may still use `className="modal--large"` or custom classes; portal’s CSS continues to define `.modal--large` if needed (or it can be moved to gui — task says minimal base includes `.modal--large` and `.modal__body`, so gui will have those). So portal’s modal.css does **not** redefine `.modal--large` or `.modal__body` unless it needs overrides (e.g. `.modal--large .modal__textarea` stays in portal).
3. **Consumers**: Portal (and later desktop) import `import '@buildmax/gui/theme.css'` and `import '@buildmax/gui/modal.css'` (or get modal.css via portal’s single modal.css import). Theme variables are required for modal.css; theme.css must be loaded first.
4. **Desktop**: No code changes in this task; when desktop adds modals later, it can import BaseModal and modal.css from gui.

## Consolidation candidates (README)

Add a short "Consolidation candidates" section to `gui/README.md` listing suggested next components to move into gui, with one-line rationale each, for example:

- **PromptArea / composer** — Single-line or multi-line input + primary action; used in portal and can be reused in desktop chat.
- **Chat message bubble** — Presentational message block (user/assistant); shared rendering for both apps.
- **Primary / secondary button** — Shared button styles and variants to keep actions consistent.
- **Icons** — Shared icon set or sprite so both apps use the same symbols.

No implementation of these in this task; the list is documentation only.

## Changes for review

- **New**: `gui/src/BaseModal.tsx` — BaseModal component (same API as current portal implementation).
- **New**: `gui/src/modal.css` — Base modal styles (overlay, .modal, .modal--large, .modal__header, .modal__title, .modal__close, .modal__body, keyframes).
- **Modified**: `gui/src/index.ts` — Export BaseModal (and optionally BaseModalProps).
- **Modified**: `gui/package.json` — Add `"./modal.css": "./dist/modal.css"` to exports; add `cp src/modal.css dist/modal.css` to build script (or equivalent).
- **Modified**: `gui/README.md` — Document BaseModal and modal CSS; add "Consolidation candidates" section.
- **Deleted**: `portal/src/components/BaseModal.tsx`.
- **Modified**: `portal/src/css/modal.css` — Start with `@import '@buildmax/gui/modal.css';`; keep only portal-specific rules (modal--settings, settings-modal__*, form helpers, modal__input, modal__btn, etc.). Remove base rules that are now in gui.
- **Modified**: Portal components that import BaseModal — Change import from `./BaseModal` or `./components/BaseModal` to `@buildmax/gui`: `SettingsModal.tsx`, `NewChatFromAgentModal.tsx`, `ArtifactContentModal.tsx`, `CreateEntityModal.tsx`.
