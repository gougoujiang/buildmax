import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import svgr from 'vite-plugin-svgr'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  plugins: [react(), svgr()],
  root: '.',
  resolve: {
    // @buildmax/gui is a symlinked workspace package that externalises react,
    // so its bare `import "react"` resolves from its own real path — and gui
    // has react installed as a peer. Without this, the bundle ships two React
    // instances and every hook throws "Cannot read properties of null".
    dedupe: ['react', 'react-dom', 'react/jsx-runtime'],
    alias: {
      // Resolve CSS subpath so Vite does not pass the bare specifier to the file system (avoids ENOENT)
      '@buildmax/gui/theme.css': path.resolve(__dirname, 'node_modules/@buildmax/gui/dist/theme.css'),
      '@buildmax/gui/modal.css': path.resolve(__dirname, 'node_modules/@buildmax/gui/dist/modal.css'),
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  test: {
    // e2e/ holds Playwright specs, which need a browser and a running
    // deployment. Vitest would otherwise collect them by extension and fail on
    // the import. `./make e2e` runs those.
    exclude: ['node_modules/**', 'dist/**', 'e2e/**'],
  },
})
