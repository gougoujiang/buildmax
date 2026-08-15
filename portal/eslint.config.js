import js from '@eslint/js'
import globals from 'globals'
import tseslint from 'typescript-eslint'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'

// `npm run build` already type-checks with `tsc -b`; ESLint covers what the
// type checker cannot see — hook rules, unreachable code, unused bindings.
export default tseslint.config(
  { ignores: ['dist'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  reactHooks.configs.flat['recommended-latest'],
  {
    plugins: { 'react-refresh': reactRefresh },
    rules: {
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
    },
  },
  {
    // Static assets copied into the bundle verbatim: classic browser scripts,
    // not modules, and not type-checked by `tsc -b`.
    files: ['public/**/*.js'],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
      sourceType: 'script',
    },
  },
  {
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
    },
    rules: {
      // An argument prefixed with _ is deliberately unused, usually to keep a
      // callback's shape.
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
      // Reported, but not a build breaker. Both rules arrived with
      // eslint-plugin-react-hooks 7 and flag long-standing patterns across the
      // app; reworking them changes render behaviour, which is a code change
      // rather than a lint fix. Left visible so the count goes down over time.
      'react-hooks/refs': 'warn',
      'react-hooks/set-state-in-effect': 'warn',
    },
  },
)
