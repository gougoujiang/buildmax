import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'

// The desktop frontend is plain JSX with no type checking, so ESLint is the
// only static check it has. The rule set stays close to the recommended
// defaults on purpose.
export default [
  { ignores: ['dist', 'wailsjs'] },
  js.configs.recommended,
  reactHooks.configs.flat['recommended-latest'],
  {
    plugins: { 'react-refresh': reactRefresh },
    rules: {
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
    },
  },
  {
    files: ['**/*.{js,jsx}'],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
      parserOptions: {
        ecmaFeatures: { jsx: true },
        sourceType: 'module',
      },
    },
    rules: {
      // Uppercase identifiers are Wails-generated bindings and constants.
      'no-unused-vars': ['error', { varsIgnorePattern: '^[A-Z_]' }],
      // Reported, but not a build breaker: the effects it flags predate the
      // rule and reworking them is a behavioural change to the desktop app,
      // not a lint fix. Left visible so the count goes down over time.
      'react-hooks/set-state-in-effect': 'warn',
    },
  },
]
