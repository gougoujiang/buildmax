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
      'react-refresh/only-export-components': 'off',
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
      // Desktop synchronizes Wails state through effects. This compiler
      // eligibility rule rejects that valid bridge pattern; hook ordering and
      // exhaustive dependency checks remain enabled.
      'react-hooks/set-state-in-effect': 'off',
    },
  },
]
