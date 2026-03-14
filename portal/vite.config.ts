import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import svgr from 'vite-plugin-svgr'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  plugins: [react(), svgr()],
  root: '.',
  resolve: {
    alias: {
      // Resolve CSS subpath so Vite does not pass the bare specifier to the file system (avoids ENOENT)
      '@buildmax/gui/theme.css': path.resolve(__dirname, 'node_modules/@buildmax/gui/dist/theme.css'),
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
